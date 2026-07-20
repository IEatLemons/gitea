// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"os"
	"strings"
)

const (
	InstallAdminNameEnv     = "GITEA_INSTALL_ADMIN_NAME"
	InstallAdminPasswordEnv = "GITEA_INSTALL_ADMIN_PASSWORD"
	InstallAdminEmailEnv    = "GITEA_INSTALL_ADMIN_EMAIL"
)

type InstallAdminEnvironment struct {
	Name     string
	Password string
	Email    string
}

// hasGiteaEnvDatabaseKey reports whether the process environment defines
// GITEA__database__<KEY> (or __FILE variant), matching [database] keys case-insensitively.
func hasGiteaEnvDatabaseKey(wantKey string) bool {
	wantKey = strings.ToLower(wantKey)
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, EnvConfigKeyPrefixGitea) {
			continue
		}
		envKey, _, ok := strings.Cut(env, "=")
		if !ok {
			continue
		}
		ok, section, key, _ := decodeEnvironmentKey(EnvConfigKeyPrefixGitea, EnvConfigKeySuffixFile, envKey)
		if !ok || section != "database" {
			continue
		}
		if strings.ToLower(key) == wantKey {
			return true
		}
	}
	return false
}

// InstallDatabaseConfiguredViaEnvironment reports whether the database is fully
// specified via environment variables so the install wizard can omit the DB form.
// Callers should only use this when the instance is not install-locked.
func InstallDatabaseConfiguredViaEnvironment() bool {
	if _, err := DBConnStr(); err != nil {
		return false
	}

	// PostgreSQL URL-style: CONN_STR or DATABASE_URL (merged into Database.ConnStr in loadDBSetting).
	if Database.Type.IsPostgreSQL() && Database.ConnStr != "" {
		if u, ok := os.LookupEnv("DATABASE_URL"); ok && strings.TrimSpace(u) != "" {
			return true
		}
		if hasGiteaEnvDatabaseKey("CONN_STR") {
			return true
		}
	}

	// Split keys must include DB_TYPE from GITEA env (distinguishes from app.ini-only defaults).
	if !hasGiteaEnvDatabaseKey("DB_TYPE") {
		return false
	}

	switch Database.Type {
	case "postgres", "mysql", "mssql":
		if !hasGiteaEnvDatabaseKey("HOST") || !hasGiteaEnvDatabaseKey("USER") || !hasGiteaEnvDatabaseKey("NAME") {
			return false
		}
	case "sqlite3":
		if !hasGiteaEnvDatabaseKey("PATH") {
			return false
		}
	default:
		return false
	}

	return true
}

func InstallAdminConfiguredViaEnvironment() (InstallAdminEnvironment, bool) {
	admin := InstallAdminEnvironment{
		Name:     strings.TrimSpace(os.Getenv(InstallAdminNameEnv)),
		Password: os.Getenv(InstallAdminPasswordEnv),
		Email:    strings.TrimSpace(os.Getenv(InstallAdminEmailEnv)),
	}
	return admin, admin.Name != "" && strings.TrimSpace(admin.Password) != "" && admin.Email != ""
}

func InstallAutoConfiguredViaEnvironment() bool {
	_, hasAdmin := InstallAdminConfiguredViaEnvironment()
	return hasAdmin && InstallDatabaseConfiguredViaEnvironment()
}
