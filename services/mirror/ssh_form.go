// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package mirror

import (
	"fmt"
	"strings"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/git/sshauth"
	"code.gitea.io/gitea/modules/secret"
	"code.gitea.io/gitea/modules/setting"
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
	return nil
}
