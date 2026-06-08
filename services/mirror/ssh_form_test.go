// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package mirror

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestGenerateSSHKeyPair(t *testing.T) {
	privateKey, publicKey, err := GenerateSSHKeyPair()
	require.NoError(t, err)
	require.NotEmpty(t, privateKey)
	require.NotEmpty(t, publicKey)

	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	require.NoError(t, err)
	assert.Equal(t, publicKey, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))))
}

func TestDeriveSSHPublicKey(t *testing.T) {
	privateKey, publicKey, err := GenerateSSHKeyPair()
	require.NoError(t, err)

	derived, err := DeriveSSHPublicKey(privateKey)
	require.NoError(t, err)
	assert.Equal(t, publicKey, derived)
}
