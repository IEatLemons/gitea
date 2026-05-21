// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package mirror

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMirrorBranches(t *testing.T) {
	out, err := ParseMirrorBranches("")
	require.NoError(t, err)
	assert.Nil(t, out)

	out, err = ParseMirrorBranches("  ")
	require.NoError(t, err)
	assert.Nil(t, out)

	out, err = ParseMirrorBranches("develop, main")
	require.NoError(t, err)
	assert.Equal(t, []string{"develop", "main"}, out)

	out, err = ParseMirrorBranches("develop, develop, main")
	require.NoError(t, err)
	assert.Equal(t, []string{"develop", "main"}, out)

	_, err = ParseMirrorBranches("bad..name")
	require.Error(t, err)
}
