// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

// Mirror authentication for pull/push mirrors: HTTPS (username/token) or SSH (deploy key).
const (
	MirrorAuthHTTPS = "https"
	MirrorAuthSSH   = "ssh"
)

// SSH host verification policy for mirror outbound git over SSH.
const (
	MirrorSSHHostKeyFingerprint = "fingerprint"
	MirrorSSHHostKeyAcceptNew   = "accept_new"
)

// MirrorSyncTrigger describes what triggered a mirror sync task.
const (
	MirrorSyncTriggerScheduled = "scheduled"
	MirrorSyncTriggerCommit    = "commit"
	MirrorSyncTriggerManual    = "manual"
)

// MirrorSyncType distinguishes pull vs push mirror tasks.
const (
	MirrorSyncTypePull = "pull"
	MirrorSyncTypePush = "push"
)
