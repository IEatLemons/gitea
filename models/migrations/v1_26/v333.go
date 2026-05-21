// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_26

import (
	"xorm.io/xorm"
)

// AddSSHCredentialsAndMirrorSyncTask adds SSH auth fields to push/pull mirrors
// and creates mirror_sync_task for sync history.
func AddSSHCredentialsAndMirrorSyncTask(x *xorm.Engine) error {
	type PushMirror struct {
		AuthType                   string `xorm:"VARCHAR(16) NOT NULL DEFAULT 'https'"`
		SSHPrivateKeyEncrypted     string `xorm:"TEXT"`
		SSHHostKeyPolicy           string `xorm:"VARCHAR(32) NOT NULL DEFAULT 'fingerprint'"`
		SSHKnownHostFingerprint    string `xorm:"TEXT"`
	}
	if _, err := x.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, new(PushMirror)); err != nil {
		return err
	}

	type Mirror struct {
		AuthType                   string `xorm:"VARCHAR(16) NOT NULL DEFAULT 'https'"`
		SSHPrivateKeyEncrypted     string `xorm:"TEXT"`
		SSHHostKeyPolicy           string `xorm:"VARCHAR(32) NOT NULL DEFAULT 'fingerprint'"`
		SSHKnownHostFingerprint    string `xorm:"TEXT"`
		LastError                  string `xorm:"TEXT"`
	}
	if _, err := x.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, new(Mirror)); err != nil {
		return err
	}

	type MirrorSyncTask struct {
		ID            int64  `xorm:"pk autoincr"`
		UUID          string `xorm:"VARCHAR(40) UNIQUE"`
		RepoID        int64  `xorm:"INDEX"`
		MirrorType    string `xorm:"VARCHAR(8) NOT NULL"`
		PushMirrorID  int64  `xorm:"INDEX"`
		TriggerType   string `xorm:"VARCHAR(16) NOT NULL"`
		IsSucceed     bool
		Stdout        string `xorm:"LONGTEXT"`
		Stderr        string `xorm:"LONGTEXT"`
		ErrorMessage  string `xorm:"TEXT"`
		StartedUnix   int64  `xorm:"INDEX"`
		FinishedUnix  int64
	}
	_, err := x.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, new(MirrorSyncTask))
	return err
}
