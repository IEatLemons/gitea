// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package mirror

import (
	"fmt"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/git/sshauth"
	giturl "code.gitea.io/gitea/modules/git/url"
	"code.gitea.io/gitea/modules/proxy"
	"code.gitea.io/gitea/modules/secret"
	"code.gitea.io/gitea/modules/setting"
)

// BuildPushMirrorGitEnvs builds git process env (HTTP proxy + optional SSH) for a push mirror.
func BuildPushMirrorGitEnvs(remoteURL *giturl.GitURL, pm *repo_model.PushMirror) ([]string, func(), error) {
	base := proxy.EnvWithProxy(remoteURL.URL)
	if pm.AuthType != repo_model.MirrorAuthSSH {
		return base, func() {}, nil
	}
	if pm.SSHPrivateKeyEncrypted == "" {
		return nil, func() {}, fmt.Errorf("SSH mirror: private key is missing")
	}
	key, err := secret.DecryptSecret(setting.SecretKey, pm.SSHPrivateKeyEncrypted)
	if err != nil {
		return nil, func() {}, err
	}
	policy := pm.SSHHostKeyPolicy
	if policy == "" {
		policy = repo_model.MirrorSSHHostKeyFingerprint
	}
	return sshauth.AppendSSHEnv(base, sshauth.Config{
		PrivateKeyPEM:        key,
		HostKeyPolicy:        policy,
		KnownHostFingerprint: pm.SSHKnownHostFingerprint,
		RemoteURL:            remoteURL.String(),
	})
}

// BuildPullMirrorGitEnvs builds git process env for a pull mirror.
func BuildPullMirrorGitEnvs(remoteURL *giturl.GitURL, m *repo_model.Mirror) ([]string, func(), error) {
	base := proxy.EnvWithProxy(remoteURL.URL)
	if m.AuthType != repo_model.MirrorAuthSSH {
		return base, func() {}, nil
	}
	if m.SSHPrivateKeyEncrypted == "" {
		return nil, func() {}, fmt.Errorf("SSH mirror: private key is missing")
	}
	key, err := secret.DecryptSecret(setting.SecretKey, m.SSHPrivateKeyEncrypted)
	if err != nil {
		return nil, func() {}, err
	}
	policy := m.SSHHostKeyPolicy
	if policy == "" {
		policy = repo_model.MirrorSSHHostKeyFingerprint
	}
	return sshauth.AppendSSHEnv(base, sshauth.Config{
		PrivateKeyPEM:        key,
		HostKeyPolicy:        policy,
		KnownHostFingerprint: m.SSHKnownHostFingerprint,
		RemoteURL:            remoteURL.String(),
	})
}
