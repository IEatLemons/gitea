// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_26

import (
	"xorm.io/xorm"
)

// AddMirrorBranchesToPushMirror adds optional comma-separated branch whitelist for push mirrors.
func AddMirrorBranchesToPushMirror(x *xorm.Engine) error {
	type PushMirror struct {
		MirrorBranches string `xorm:"TEXT"`
	}
	_, err := x.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, new(PushMirror))
	return err
}
