// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package mirror

import (
	"errors"

	"code.gitea.io/gitea/modules/git"
)

func extractPushGitOutput(err error) (stdout, stderr string) {
	if err == nil {
		return "", ""
	}
	var ood *git.ErrPushOutOfDate
	if errors.As(err, &ood) {
		return ood.StdOut, ood.StdErr
	}
	var rej *git.ErrPushRejected
	if errors.As(err, &rej) {
		return rej.StdOut, rej.StdErr
	}
	var mo *git.ErrMoreThanOne
	if errors.As(err, &mo) {
		return mo.StdOut, mo.StdErr
	}
	return "", err.Error()
}
