// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package deployment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/secret"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/builder"
)

// Provider identifies a supported deployment platform.
type Provider string

const (
	ProviderRailway Provider = "railway"
	ProviderVercel  Provider = "vercel"
)

// IsValid reports whether the provider is supported.
func (p Provider) IsValid() bool {
	return p == ProviderRailway || p == ProviderVercel
}

// Status is the normalized lifecycle state of a remote deployment.
type Status string

const (
	StatusQueued   Status = "queued"
	StatusRunning  Status = "running"
	StatusSuccess  Status = "success"
	StatusFailure  Status = "failure"
	StatusCanceled Status = "canceled"
	StatusUnknown  Status = "unknown"
)

// Connection stores an administrator-configured deployment platform connection.
type Connection struct {
	ID             int64    `xorm:"pk autoincr"`
	Provider       Provider `xorm:"VARCHAR(16) INDEX UNIQUE(provider_name) NOT NULL"`
	Name           string   `xorm:"VARCHAR(255) UNIQUE(provider_name) NOT NULL"`
	TokenEncrypted string   `xorm:"TEXT NOT NULL"`
	ScopeID        string   `xorm:"VARCHAR(255)"`
	IsActive       bool     `xorm:"INDEX NOT NULL DEFAULT true"`
	LastSyncStatus Status   `xorm:"VARCHAR(16) NOT NULL DEFAULT 'unknown'"`
	LastSyncError  string   `xorm:"TEXT"`
	LastSyncUnix   timeutil.TimeStamp
	CreatedUnix    timeutil.TimeStamp `xorm:"created"`
	UpdatedUnix    timeutil.TimeStamp `xorm:"updated"`
}

func (*Connection) TableName() string {
	return "deployment_connection"
}

// SetToken encrypts and stores a clear-text platform token.
func (c *Connection) SetToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("deployment platform token is empty")
	}
	encrypted, err := secret.EncryptSecret(setting.SecretKey, token)
	if err != nil {
		return err
	}
	c.TokenEncrypted = encrypted
	return nil
}

// Token decrypts the platform token for outbound API use.
func (c *Connection) Token() (string, error) {
	return secret.DecryptSecret(setting.SecretKey, c.TokenEncrypted)
}

// Binding maps one platform environment or service to a Gitea repository.
type Binding struct {
	ID              int64  `xorm:"pk autoincr"`
	ConnectionID    int64  `xorm:"INDEX UNIQUE(remote_target) NOT NULL"`
	RepoID          int64  `xorm:"INDEX NOT NULL"`
	ProjectID       string `xorm:"VARCHAR(255) UNIQUE(remote_target) NOT NULL"`
	ProjectName     string `xorm:"VARCHAR(255) NOT NULL"`
	ServiceID       string `xorm:"VARCHAR(255) UNIQUE(remote_target)"`
	ServiceName     string `xorm:"VARCHAR(255)"`
	EnvironmentID   string `xorm:"VARCHAR(255) UNIQUE(remote_target) NOT NULL"`
	EnvironmentName string `xorm:"VARCHAR(255) NOT NULL"`
	BranchFilter    string `xorm:"VARCHAR(255) UNIQUE(remote_target)"`
	DisplayName     string `xorm:"VARCHAR(255)"`
	IsActive        bool   `xorm:"INDEX NOT NULL DEFAULT true"`
	RemoteValid     bool   `xorm:"NOT NULL DEFAULT true"`
	LastSyncError   string `xorm:"TEXT"`
	LastSyncUnix    timeutil.TimeStamp
	CreatedUnix     timeutil.TimeStamp `xorm:"created"`
	UpdatedUnix     timeutil.TimeStamp `xorm:"updated"`
	Connection      *Connection        `xorm:"-"`
	Snapshot        *Snapshot          `xorm:"-"`
}

func (*Binding) TableName() string {
	return "deployment_binding"
}

// Snapshot stores the latest successful read from a deployment platform.
type Snapshot struct {
	ID                     int64  `xorm:"pk autoincr"`
	BindingID              int64  `xorm:"INDEX UNIQUE NOT NULL"`
	ActiveDeploymentID     string `xorm:"VARCHAR(255)"`
	ActiveCommitSHA        string `xorm:"VARCHAR(64)"`
	ActiveStatus           Status `xorm:"VARCHAR(16) NOT NULL DEFAULT 'unknown'"`
	ActiveURL              string `xorm:"TEXT"`
	ActiveCreatedUnix      timeutil.TimeStamp
	LatestDeploymentID     string `xorm:"VARCHAR(255)"`
	LatestCommitSHA        string `xorm:"VARCHAR(64)"`
	LatestStatus           Status `xorm:"VARCHAR(16) NOT NULL DEFAULT 'unknown'"`
	LatestURL              string `xorm:"TEXT"`
	LatestCreatedUnix      timeutil.TimeStamp
	LastSuccessfulSyncUnix timeutil.TimeStamp
	CreatedUnix            timeutil.TimeStamp `xorm:"created"`
	UpdatedUnix            timeutil.TimeStamp `xorm:"updated"`
}

func (*Snapshot) TableName() string {
	return "deployment_snapshot"
}

// IsStale reports whether the snapshot is older than two synchronization intervals.
func (s *Snapshot) IsStale() bool {
	return s == nil || s.LastSuccessfulSyncUnix.IsZero() || time.Since(s.LastSuccessfulSyncUnix.AsTime()) > 10*time.Minute
}

// ShortSHA returns a compact commit identifier for display.
func ShortSHA(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}

func init() {
	db.RegisterModel(new(Connection))
	db.RegisterModel(new(Binding))
	db.RegisterModel(new(Snapshot))
}

// ListConnections returns all configured platform connections.
func ListConnections(ctx context.Context) ([]*Connection, error) {
	connections := make([]*Connection, 0)
	return connections, db.GetEngine(ctx).OrderBy("provider, name").Find(&connections)
}

// GetConnectionByID returns one platform connection.
func GetConnectionByID(ctx context.Context, id int64) (*Connection, error) {
	connection, has, err := db.Get[Connection](ctx, builder.Eq{"id": id})
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, fmt.Errorf("deployment connection %d does not exist", id)
	}
	return connection, nil
}

// InsertConnection creates a platform connection.
func InsertConnection(ctx context.Context, connection *Connection) error {
	_, err := db.GetEngine(ctx).Insert(connection)
	return err
}

// UpdateConnection updates editable connection fields and optionally its token.
func UpdateConnection(ctx context.Context, connection *Connection, updateToken bool) error {
	cols := []string{"name", "scope_id", "is_active"}
	if updateToken {
		cols = append(cols, "token_encrypted")
	}
	_, err := db.GetEngine(ctx).ID(connection.ID).Cols(cols...).Update(connection)
	return err
}

// UpdateConnectionSyncResult persists the latest connection-wide synchronization result.
func UpdateConnectionSyncResult(ctx context.Context, connection *Connection) error {
	_, err := db.GetEngine(ctx).ID(connection.ID).Cols("last_sync_status", "last_sync_error", "last_sync_unix").Update(connection)
	return err
}

// DeleteConnection deletes a connection and all of its local bindings and snapshots.
func DeleteConnection(ctx context.Context, id int64) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		bindingIDs, err := db.FindIDs(ctx, "deployment_binding", "id", builder.Eq{"connection_id": id})
		if err != nil {
			return err
		}
		if len(bindingIDs) > 0 {
			if _, err := db.GetEngine(ctx).In("binding_id", bindingIDs).Delete(new(Snapshot)); err != nil {
				return err
			}
		}
		if _, err := db.DeleteByBean(ctx, &Binding{ConnectionID: id}); err != nil {
			return err
		}
		_, err = db.DeleteByID[Connection](ctx, id)
		return err
	})
}

// ListBindings returns all repository bindings.
func ListBindings(ctx context.Context) ([]*Binding, error) {
	bindings := make([]*Binding, 0)
	err := db.GetEngine(ctx).OrderBy("repo_id, display_name, id").Find(&bindings)
	return bindings, err
}

// ListBindingsByConnection returns bindings belonging to a connection.
func ListBindingsByConnection(ctx context.Context, connectionID int64, activeOnly bool) ([]*Binding, error) {
	bindings := make([]*Binding, 0)
	sess := db.GetEngine(ctx).Where("connection_id = ?", connectionID)
	if activeOnly {
		sess = sess.And("is_active = ?", true)
	}
	err := sess.OrderBy("id").Find(&bindings)
	return bindings, err
}

// ListBindingsByRepo returns active bindings with their connection and cached snapshot.
func ListBindingsByRepo(ctx context.Context, repoID int64) ([]*Binding, error) {
	bindings := make([]*Binding, 0)
	err := db.GetEngine(ctx).Where("repo_id = ? AND is_active = ?", repoID, true).OrderBy("display_name, id").Find(&bindings)
	if err != nil || len(bindings) == 0 {
		return bindings, err
	}
	connectionIDs := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		connectionIDs = append(connectionIDs, binding.ConnectionID)
	}
	connections := make([]*Connection, 0)
	if err := db.GetEngine(ctx).In("id", connectionIDs).Find(&connections); err != nil {
		return nil, err
	}
	connectionMap := make(map[int64]*Connection, len(connections))
	for _, connection := range connections {
		connectionMap[connection.ID] = connection
	}
	snapshots := make([]*Snapshot, 0)
	bindingIDs := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		bindingIDs = append(bindingIDs, binding.ID)
	}
	if err := db.GetEngine(ctx).In("binding_id", bindingIDs).Find(&snapshots); err != nil {
		return nil, err
	}
	snapshotMap := make(map[int64]*Snapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotMap[snapshot.BindingID] = snapshot
	}
	for _, binding := range bindings {
		binding.Connection = connectionMap[binding.ConnectionID]
		binding.Snapshot = snapshotMap[binding.ID]
	}
	return bindings, nil
}

// InsertBinding creates a repository-to-platform binding.
func InsertBinding(ctx context.Context, binding *Binding) error {
	_, err := db.GetEngine(ctx).Insert(binding)
	return err
}

// DeleteBinding deletes a binding and its cached snapshot.
func DeleteBinding(ctx context.Context, id int64) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		if _, err := db.DeleteByBean(ctx, &Snapshot{BindingID: id}); err != nil {
			return err
		}
		_, err := db.DeleteByID[Binding](ctx, id)
		return err
	})
}

// DeleteBindingsByRepoID deletes all deployment data belonging to a repository.
func DeleteBindingsByRepoID(ctx context.Context, repoID int64) error {
	bindingIDs, err := db.FindIDs(ctx, "deployment_binding", "id", builder.Eq{"repo_id": repoID})
	if err != nil || len(bindingIDs) == 0 {
		return err
	}
	if _, err := db.GetEngine(ctx).In("binding_id", bindingIDs).Delete(new(Snapshot)); err != nil {
		return err
	}
	_, err = db.GetEngine(ctx).In("id", bindingIDs).Delete(new(Binding))
	return err
}

// UpdateBindingSyncResult persists the latest binding synchronization result.
func UpdateBindingSyncResult(ctx context.Context, binding *Binding) error {
	_, err := db.GetEngine(ctx).ID(binding.ID).Cols("remote_valid", "last_sync_error", "last_sync_unix").Update(binding)
	return err
}

// UpsertSnapshot replaces the cached status for a binding.
func UpsertSnapshot(ctx context.Context, snapshot *Snapshot) error {
	existing, has, err := db.Get[Snapshot](ctx, builder.Eq{"binding_id": snapshot.BindingID})
	if err != nil {
		return err
	}
	if !has {
		_, err = db.GetEngine(ctx).Insert(snapshot)
		return err
	}
	snapshot.ID = existing.ID
	_, err = db.GetEngine(ctx).ID(existing.ID).AllCols().Omit("id", "created_unix").Update(snapshot)
	return err
}
