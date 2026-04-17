// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizeRootURL parses and normalizes a ROOT_URL-style string (scheme, default ports, single trailing slash).
func NormalizeRootURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("scheme must be http or https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	if (u.Scheme == string(HTTP) && u.Port() == "80") || (u.Scheme == string(HTTPS) && u.Port() == "443") {
		u.Host = u.Hostname()
	}
	return strings.TrimRight(u.String(), "/") + "/", nil
}
