// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_26

import (
	"xorm.io/xorm"
)

// AddDeployStampToPushMirror adds optional deploy-stamp fields for push mirrors.
func AddDeployStampToPushMirror(x *xorm.Engine) error {
	type PushMirror struct {
		DeployStampEnabled       bool   `xorm:"NOT NULL DEFAULT false"`
		DeployStampBranches      string `xorm:"TEXT"`
		DeployStampAuthorName    string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
		DeployStampAuthorEmail   string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
		DeployStampCommitMessage string `xorm:"VARCHAR(512) NOT NULL DEFAULT 'chore: deploy stamp'"`
	}
	_, err := x.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, new(PushMirror))
	return err
}
