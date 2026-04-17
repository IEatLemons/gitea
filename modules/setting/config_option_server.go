// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import "code.gitea.io/gitea/modules/setting/config"

// Dynamic config keys for [ServerStruct] (system_setting + admin UI).
const (
	ServerRootURLDynKey   = "server.root_url"
	ServerSSHDomainDynKey = "server.ssh_domain"
	ServerSSHPortDynKey   = "server.ssh_port"
)

// ServerStruct holds admin-editable overrides for public clone URLs (see also [NormalizeRootURL]).
type ServerStruct struct {
	RootURL   *config.Option[string]
	SSHDomain *config.Option[string]
	SSHPort   *config.Option[int]
}
