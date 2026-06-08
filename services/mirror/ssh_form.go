// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package mirror

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"strings"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/git/sshauth"
	"code.gitea.io/gitea/modules/secret"
	"code.gitea.io/gitea/modules/setting"

	"golang.org/x/crypto/ssh"
)

const (
	// PushMirrorSSHKeyModeKeep keeps the existing encrypted SSH private key.
	PushMirrorSSHKeyModeKeep = "keep"
	// PushMirrorSSHKeyModeGenerate creates a new private key on the server.
	PushMirrorSSHKeyModeGenerate = "generate"
	// PushMirrorSSHKeyModeManual uses the private key pasted into the form.
	PushMirrorSSHKeyModeManual = "manual"
)

// NormalizeMirrorAuthType returns repo_model.MirrorAuthHTTPS or MirrorAuthSSH.
func NormalizeMirrorAuthType(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), repo_model.MirrorAuthSSH) {
		return repo_model.MirrorAuthSSH
	}
	return repo_model.MirrorAuthHTTPS
}

// NormalizeSSHHostKeyPolicy returns fingerprint or accept_new.
func NormalizeSSHHostKeyPolicy(s string) string {
	s = strings.TrimSpace(s)
	if s == repo_model.MirrorSSHHostKeyAcceptNew {
		return repo_model.MirrorSSHHostKeyAcceptNew
	}
	return repo_model.MirrorSSHHostKeyFingerprint
}

// EncryptSSHPrivateKeyOrEmpty encrypts PEM for DB storage; blank input returns "", nil.
func EncryptSSHPrivateKeyOrEmpty(pem string) (string, error) {
	pem = strings.TrimSpace(pem)
	if pem == "" {
		return "", nil
	}
	return secret.EncryptSecret(setting.SecretKey, pem)
}

// GenerateSSHKeyPair creates an OpenSSH private key and the matching authorized_keys public key.
func GenerateSSHKeyPair() (privateKeyPEM, publicKey string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return "", "", err
	}
	sshPubKey, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(pem.EncodeToMemory(block))), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPubKey))), nil
}

// DeriveSSHPublicKey returns the authorized_keys public key for an OpenSSH private key.
func DeriveSSHPublicKey(privateKeyPEM string) (string, error) {
	signer, err := ssh.ParsePrivateKey([]byte(strings.TrimSpace(privateKeyPEM)))
	if err != nil {
		return "", fmt.Errorf("SSH mirror: invalid private key (passphrase-protected keys are not supported): %w", err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))), nil
}

// ValidateSSHMirrorFields checks policy, known_hosts line, and that a key exists (new or stored).
func ValidateSSHMirrorFields(policy, knownHostsLine, newKeyPEM, existingKeyEnc string) error {
	policy = NormalizeSSHHostKeyPolicy(policy)
	if policy == repo_model.MirrorSSHHostKeyFingerprint {
		if err := sshauth.ValidateKnownHostsLine(knownHostsLine); err != nil {
			return fmt.Errorf("%w", err)
		}
	}
	if strings.TrimSpace(newKeyPEM) == "" && strings.TrimSpace(existingKeyEnc) == "" {
		return fmt.Errorf("SSH private key is required")
	}
	if strings.TrimSpace(newKeyPEM) != "" {
		if _, err := DeriveSSHPublicKey(newKeyPEM); err != nil {
			return err
		}
	}
	return nil
}
