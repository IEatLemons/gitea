// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package deployment

import (
	"testing"

	"code.gitea.io/gitea/modules/setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionTokenEncryption(t *testing.T) {
	oldSecretKey := setting.SecretKey
	setting.SecretKey = "deployment-test-secret"
	t.Cleanup(func() { setting.SecretKey = oldSecretKey })

	connection := &Connection{}
	require.NoError(t, connection.SetToken("token-value"))
	assert.NotEqual(t, "token-value", connection.TokenEncrypted)
	token, err := connection.Token()
	require.NoError(t, err)
	assert.Equal(t, "token-value", token)
}

func TestShortSHA(t *testing.T) {
	assert.Equal(t, "1234567890", ShortSHA("1234567890abcdef"))
	assert.Equal(t, "short", ShortSHA("short"))
}
