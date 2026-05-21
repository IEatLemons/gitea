// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package mirror

import (
	"fmt"
	"strings"

	"code.gitea.io/gitea/modules/git"
)

// ParseMirrorBranches splits a comma-separated branch list and validates each short name.
// Empty or whitespace-only input yields (nil, nil). Duplicate names are deduplicated in order.
func ParseMirrorBranches(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		b := strings.TrimSpace(p)
		if b == "" {
			continue
		}
		if !git.IsValidRefPattern(b) {
			return nil, fmt.Errorf("invalid branch name: %q", b)
		}
		if _, ok := seen[b]; ok {
			continue
		}
		seen[b] = struct{}{}
		out = append(out, b)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// JoinMirrorBranches formats a validated list for storage in the database.
func JoinMirrorBranches(branches []string) string {
	return strings.Join(branches, ", ")
}
