// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_26

import (
	"xorm.io/xorm"
)

// AddRecordFileToPushMirror adds optional record-file commit settings for push mirrors.
func AddRecordFileToPushMirror(x *xorm.Engine) error {
	type PushMirror struct {
		RecordFileEnabled       bool   `xorm:"NOT NULL DEFAULT false"`
		RecordFileBranches      string `xorm:"TEXT"`
		RecordFilePath          string `xorm:"VARCHAR(1024) NOT NULL DEFAULT ''"`
		RecordFileTemplate      string `xorm:"LONGTEXT"`
		RecordFileAuthorName    string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
		RecordFileAuthorEmail   string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
		RecordFileCommitMessage string `xorm:"VARCHAR(512) NOT NULL DEFAULT 'chore: update mirror record'"`
	}
	_, err := x.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, new(PushMirror))
	return err
}
