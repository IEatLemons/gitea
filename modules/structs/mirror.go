// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package structs

import "time"

// CreatePushMirrorOption represents need information to create a push mirror of a repository.
type CreatePushMirrorOption struct {
	// The remote repository URL to push to
	RemoteAddress string `json:"remote_address"`
	// The username for authentication with the remote repository
	RemoteUsername string `json:"remote_username"`
	// The password for authentication with the remote repository
	RemotePassword string `json:"remote_password"`
	// The sync interval for automatic updates
	Interval string `json:"interval"`
	// Whether to sync on every commit
	SyncOnCommit bool `json:"sync_on_commit"`
	// add trusted-identity empty commit before mirror push on listed branches
	DeployStampEnabled bool `json:"deploy_stamp_enabled"`
	// comma-separated branch names
	DeployStampBranches      string `json:"deploy_stamp_branches"`
	DeployStampAuthorName    string `json:"deploy_stamp_author_name"`
	DeployStampAuthorEmail   string `json:"deploy_stamp_author_email"`
	DeployStampCommitMessage string `json:"deploy_stamp_commit_message"`
	// Comma-separated branch names; empty mirrors all branches and tags.
	MirrorBranches string `json:"mirror_branches"`
	// https or ssh (default https)
	AuthType string `json:"auth_type"`
	// PEM private key for ssh auth (not returned in API responses)
	SSHPrivateKey string `json:"ssh_private_key"`
	// fingerprint (default) or accept_new
	SSHHostKeyPolicy string `json:"ssh_host_key_policy"`
	// Single known_hosts line, required when ssh_host_key_policy is fingerprint
	SSHKnownHostFingerprint string `json:"ssh_known_hosts_line"`
}

// EditPushMirrorOption options for updating a push mirror
type EditPushMirrorOption struct {
	Interval                 *string `json:"interval"`
	SyncOnCommit             *bool   `json:"sync_on_commit"`
	DeployStampEnabled       *bool   `json:"deploy_stamp_enabled"`
	DeployStampBranches      *string `json:"deploy_stamp_branches"`
	DeployStampAuthorName    *string `json:"deploy_stamp_author_name"`
	DeployStampAuthorEmail   *string `json:"deploy_stamp_author_email"`
	DeployStampCommitMessage *string `json:"deploy_stamp_commit_message"`
	MirrorBranches           *string `json:"mirror_branches"`
}

// PushMirror represents information of a push mirror
// swagger:model
type PushMirror struct {
	// The name of the source repository
	RepoName string `json:"repo_name"`
	// The name of the remote in the git configuration
	RemoteName string `json:"remote_name"`
	// The remote repository URL being mirrored to
	RemoteAddress string `json:"remote_address"`
	// swagger:strfmt date-time
	CreatedUnix time.Time `json:"created"`
	// swagger:strfmt date-time
	LastUpdateUnix *time.Time `json:"last_update"`
	// The last error message encountered during sync
	LastError string `json:"last_error"`
	// The sync interval for automatic updates
	Interval string `json:"interval"`
	// Whether to sync on every commit
	SyncOnCommit             bool   `json:"sync_on_commit"`
	DeployStampEnabled       bool   `json:"deploy_stamp_enabled"`
	DeployStampBranches      string `json:"deploy_stamp_branches"`
	DeployStampAuthorName    string `json:"deploy_stamp_author_name"`
	DeployStampAuthorEmail   string `json:"deploy_stamp_author_email"`
	DeployStampCommitMessage string `json:"deploy_stamp_commit_message"`
	MirrorBranches           string `json:"mirror_branches"`
	AuthType                 string `json:"auth_type"`
	SSHHostKeyPolicy         string `json:"ssh_host_key_policy"`
}

// MirrorSyncTask is one push/pull mirror sync attempt.
type MirrorSyncTask struct {
	UUID         string    `json:"uuid"`
	MirrorType   string    `json:"mirror_type"`
	TriggerType  string    `json:"trigger_type"`
	IsSucceed    bool      `json:"is_succeed"`
	Stdout       string    `json:"stdout"`
	Stderr       string    `json:"stderr"`
	ErrorMessage string    `json:"error_message"`
	StartedUnix  time.Time `json:"started"`
	FinishedUnix time.Time `json:"finished"`
}
