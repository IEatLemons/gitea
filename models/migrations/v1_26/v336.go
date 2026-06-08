// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_26

import (
	"xorm.io/xorm"
)

// AddMergeCommitterToRepository adds optional repository-level merge commit identity settings.
func AddMergeCommitterToRepository(x *xorm.Engine) error {
	type Repository struct {
		MergeCommitterName  string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
		MergeCommitterEmail string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
	}
	_, err := x.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, new(Repository))
	return err
}
