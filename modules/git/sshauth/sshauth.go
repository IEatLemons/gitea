// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package sshauth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	giturl "code.gitea.io/gitea/modules/git/url"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/util"

	"golang.org/x/crypto/ssh"
)

// Config holds everything needed to run git over SSH with a deploy key.
type Config struct {
	PrivateKeyPEM        string
	HostKeyPolicy        string
	KnownHostFingerprint string
	RemoteURL            string
}

// ValidateKnownHostsLine checks a single OpenSSH known_hosts line.
func ValidateKnownHostsLine(line string) error {
	_, _, _, _, _, err := ssh.ParseKnownHosts([]byte(strings.TrimSpace(line)))
	return err
}

// AppendSSHEnv sets GIT_SSH_COMMAND for outbound git and returns cleanup to remove temp files.
func AppendSSHEnv(baseEnvs []string, cfg Config) (envs []string, cleanup func(), err error) {
	cleanup = func() {}
	if cfg.PrivateKeyPEM == "" {
		return nil, cleanup, fmt.Errorf("SSH mirror: private key is required")
	}
	if _, err := ssh.ParsePrivateKey([]byte(strings.TrimSpace(cfg.PrivateKeyPEM))); err != nil {
		return nil, cleanup, fmt.Errorf("SSH mirror: invalid private key (passphrase-protected keys are not supported): %w", err)
	}

	u, err := giturl.ParseGitURL(cfg.RemoteURL)
	if err != nil {
		return nil, cleanup, fmt.Errorf("SSH mirror: invalid remote URL: %w", err)
	}
	if u.Host == "" {
		return nil, cleanup, fmt.Errorf("SSH mirror: could not parse host from URL")
	}

	tmpDir, cleanupDir, err := setting.AppDataTempDir("mirror-ssh").MkdirTempRandom("key")
	if err != nil {
		return nil, cleanup, fmt.Errorf("SSH mirror: temp dir: %w", err)
	}
	cleanup = cleanupDir

	keyPath := filepath.Join(tmpDir, "id")
	if err := os.WriteFile(keyPath, []byte(strings.TrimSpace(cfg.PrivateKeyPEM)), 0o600); err != nil {
		cleanup()
		cleanup = func() {}
		return nil, cleanup, fmt.Errorf("SSH mirror: write key: %w", err)
	}

	var sshArgs []string
	switch cfg.HostKeyPolicy {
	case "", "fingerprint":
		line := strings.TrimSpace(cfg.KnownHostFingerprint)
		if line == "" {
			cleanup()
			cleanup = func() {}
			return nil, cleanup, fmt.Errorf("SSH mirror: known host fingerprint line is required when policy is fingerprint")
		}
		if err := ValidateKnownHostsLine(line); err != nil {
			cleanup()
			cleanup = func() {}
			return nil, cleanup, fmt.Errorf("SSH mirror: invalid known_hosts line: %w", err)
		}
		khPath := filepath.Join(tmpDir, "known_hosts")
		if err := os.WriteFile(khPath, []byte(line+"\n"), 0o600); err != nil {
			cleanup()
			cleanup = func() {}
			return nil, cleanup, fmt.Errorf("SSH mirror: write known_hosts: %w", err)
		}
		sshArgs = []string{
			"-o", "IdentitiesOnly=yes",
			"-o", fmt.Sprintf("UserKnownHostsFile=%s", khPath),
			"-o", "StrictHostKeyChecking=yes",
			"-i", keyPath,
		}
	case "accept_new":
		sshArgs = []string{
			"-o", "IdentitiesOnly=yes",
			"-o", "StrictHostKeyChecking=accept-new",
			"-i", keyPath,
		}
	default:
		cleanup()
		cleanup = func() {}
		return nil, cleanup, fmt.Errorf("SSH mirror: unknown host key policy %q", cfg.HostKeyPolicy)
	}

	quoted := util.ShellEscape("ssh")
	for _, a := range sshArgs {
		quoted += " " + util.ShellEscape(a)
	}
	envs = append(append([]string{}, baseEnvs...), "GIT_SSH_COMMAND="+quoted)
	return envs, cleanup, nil
}
