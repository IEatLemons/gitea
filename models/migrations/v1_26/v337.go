// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_26

import (
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/xorm"
)

// AddUserCommitIdentity adds per-user repository web commit identity settings.
func AddUserCommitIdentity(x *xorm.Engine) error {
	type RepoUserCommitIdentity struct {
		ID          int64  `xorm:"pk autoincr"`
		RepoID      int64  `xorm:"UNIQUE(repo_user) INDEX NOT NULL"`
		UserID      int64  `xorm:"UNIQUE(repo_user) INDEX NOT NULL"`
		CommitName  string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
		CommitEmail string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`

		CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
		UpdatedUnix timeutil.TimeStamp `xorm:"INDEX updated"`
	}
	_, err := x.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, new(RepoUserCommitIdentity))
	return err
}
