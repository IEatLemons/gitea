// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_26

import (
	"xorm.io/xorm"
)

// AddSSHPublicKeyToPushMirror stores the public key for generated SSH push mirror keys.
func AddSSHPublicKeyToPushMirror(x *xorm.Engine) error {
	type PushMirror struct {
		SSHPublicKey string `xorm:"TEXT"`
	}
	_, err := x.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, new(PushMirror))
	return err
}
