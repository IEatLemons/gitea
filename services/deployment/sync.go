// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package deployment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	deployment_model "code.gitea.io/gitea/models/deployment"
	"code.gitea.io/gitea/modules/graceful"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/timeutil"
)

var (
	syncingMu          sync.Mutex
	syncingConnections = map[int64]struct{}{}
)

// ErrAlreadySyncing is returned when a connection already has a queued or running synchronization.
var ErrAlreadySyncing = errors.New("deployment connection is already syncing")

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 1024 {
		return message[:1024]
	}
	return message
}

func beginSync(connectionID int64) bool {
	syncingMu.Lock()
	defer syncingMu.Unlock()
	if _, exists := syncingConnections[connectionID]; exists {
		return false
	}
	syncingConnections[connectionID] = struct{}{}
	return true
}

func endSync(connectionID int64) {
	syncingMu.Lock()
	delete(syncingConnections, connectionID)
	syncingMu.Unlock()
}

// Discover lists bindable remote targets using a stored connection.
func Discover(ctx context.Context, connection *deployment_model.Connection) ([]RemoteTarget, error) {
	client, err := NewClient(connection)
	if err != nil {
		return nil, err
	}
	return client.Discover(ctx)
}

// TestConnection verifies that a platform credential can read its configured scope.
func TestConnection(ctx context.Context, connection *deployment_model.Connection) error {
	client, err := NewClient(connection)
	if err != nil {
		return err
	}
	return client.Test(ctx)
}

// SyncConnection synchronizes all active bindings for one connection.
func SyncConnection(ctx context.Context, connectionID int64) error {
	if !beginSync(connectionID) {
		return ErrAlreadySyncing
	}
	defer endSync(connectionID)
	return syncConnection(ctx, connectionID)
}

func syncConnection(ctx context.Context, connectionID int64) error {
	connection, err := deployment_model.GetConnectionByID(ctx, connectionID)
	if err != nil {
		return err
	}
	if !connection.IsActive {
		return nil
	}
	client, err := NewClient(connection)
	if err != nil {
		return err
	}
	bindings, err := deployment_model.ListBindingsByConnection(ctx, connectionID, true)
	if err != nil {
		return err
	}
	now := timeutil.TimeStampNow()
	failures := make([]string, 0)
	for _, binding := range bindings {
		binding.LastSyncUnix = now
		summary, fetchErr := client.Fetch(ctx, binding)
		if fetchErr != nil {
			binding.LastSyncError = truncateError(fetchErr)
			if strings.Contains(strings.ToLower(fetchErr.Error()), "not found") || strings.Contains(fetchErr.Error(), "HTTP 404") {
				binding.RemoteValid = false
			}
			if updateErr := deployment_model.UpdateBindingSyncResult(ctx, binding); updateErr != nil {
				log.Error("Unable to update deployment binding sync error: %v", updateErr)
			}
			failures = append(failures, fmt.Sprintf("%s: %v", binding.DisplayName, fetchErr))
			continue
		}
		binding.RemoteValid = true
		binding.LastSyncError = ""
		if err := deployment_model.UpdateBindingSyncResult(ctx, binding); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", binding.DisplayName, err))
			continue
		}
		snapshot := &deployment_model.Snapshot{
			BindingID:              binding.ID,
			ActiveStatus:           deployment_model.StatusUnknown,
			LatestStatus:           deployment_model.StatusUnknown,
			LastSuccessfulSyncUnix: now,
		}
		if summary.Active != nil {
			snapshot.ActiveDeploymentID = summary.Active.ID
			snapshot.ActiveCommitSHA = summary.Active.CommitSHA
			snapshot.ActiveStatus = summary.Active.Status
			snapshot.ActiveURL = summary.Active.URL
			snapshot.ActiveCreatedUnix = timeutil.TimeStamp(summary.Active.Created.Unix())
		}
		if summary.Latest != nil {
			snapshot.LatestDeploymentID = summary.Latest.ID
			snapshot.LatestCommitSHA = summary.Latest.CommitSHA
			snapshot.LatestStatus = summary.Latest.Status
			snapshot.LatestURL = summary.Latest.URL
			snapshot.LatestCreatedUnix = timeutil.TimeStamp(summary.Latest.Created.Unix())
		}
		if err := deployment_model.UpsertSnapshot(ctx, snapshot); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", binding.DisplayName, err))
		}
	}
	connection.LastSyncUnix = now
	connection.LastSyncError = strings.Join(failures, "; ")
	connection.LastSyncStatus = deployment_model.StatusSuccess
	if len(failures) > 0 {
		connection.LastSyncStatus = deployment_model.StatusFailure
		connection.LastSyncError = truncateError(errors.New(connection.LastSyncError))
	}
	if err := deployment_model.UpdateConnectionSyncResult(ctx, connection); err != nil {
		return err
	}
	if len(failures) > 0 {
		return errors.New("one or more deployment bindings failed to sync")
	}
	return nil
}

// EnqueueConnection starts a deduplicated asynchronous synchronization.
func EnqueueConnection(connectionID int64) error {
	if !beginSync(connectionID) {
		return ErrAlreadySyncing
	}
	go graceful.GetManager().RunWithShutdownContext(func(ctx context.Context) {
		defer endSync(connectionID)
		if err := syncConnection(ctx, connectionID); err != nil {
			log.Error("Unable to sync deployment connection %d: %v", connectionID, err)
		}
	})
	return nil
}

// SyncAll synchronizes every active deployment platform connection.
func SyncAll(ctx context.Context) error {
	connections, err := deployment_model.ListConnections(ctx)
	if err != nil {
		return err
	}
	failures := 0
	for _, connection := range connections {
		if !connection.IsActive {
			continue
		}
		if err := SyncConnection(ctx, connection.ID); err != nil && !errors.Is(err, ErrAlreadySyncing) {
			log.Error("Unable to sync deployment connection %d: %v", connection.ID, err)
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d deployment platform connection(s) failed to sync", failures)
	}
	return nil
}
