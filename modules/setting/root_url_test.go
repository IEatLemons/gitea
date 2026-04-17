// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeRootURL(t *testing.T) {
	out, err := NormalizeRootURL("https://example.com:443/foo/")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/foo/", out)

	out, err = NormalizeRootURL("http://example.com:80/")
	require.NoError(t, err)
	assert.Equal(t, "http://example.com/", out)

	_, err = NormalizeRootURL("ftp://example.com/")
	assert.Error(t, err)
}
