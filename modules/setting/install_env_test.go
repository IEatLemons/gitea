// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func resetDatabaseForInstallEnvTest(t *testing.T) func() {
	t.Helper()
	saved := Database
	return func() {
		Database = saved
	}
}

func TestInstallDatabaseConfiguredViaEnvironment_PostgresURL(t *testing.T) {
	defer resetDatabaseForInstallEnvTest(t)()

	t.Setenv("DATABASE_URL", "postgres://user:pass@127.0.0.1:5432/gitea?sslmode=disable")
	t.Setenv("GITEA__database__DB_TYPE", "postgres")

	Database.Type = "postgres"
	Database.ConnStr = os.Getenv("DATABASE_URL")

	assert.True(t, InstallDatabaseConfiguredViaEnvironment())
}

func TestInstallDatabaseConfiguredViaEnvironment_PostgresGiteaConnStr(t *testing.T) {
	defer resetDatabaseForInstallEnvTest(t)()

	conn := "postgres://user:pass@127.0.0.1:5432/gitea?sslmode=disable"
	t.Setenv("GITEA__database__CONN_STR", conn)
	t.Setenv("GITEA__database__DB_TYPE", "postgres")

	Database.Type = "postgres"
	Database.ConnStr = conn

	assert.True(t, InstallDatabaseConfiguredViaEnvironment())
}

func TestInstallDatabaseConfiguredViaEnvironment_PostgresConnStrIniOnly(t *testing.T) {
	defer resetDatabaseForInstallEnvTest(t)()

	Database.Type = "postgres"
	Database.ConnStr = "postgres://user:pass@127.0.0.1:5432/gitea?sslmode=disable"

	assert.False(t, InstallDatabaseConfiguredViaEnvironment())
}

func TestInstallDatabaseConfiguredViaEnvironment_PostgresSplit(t *testing.T) {
	defer resetDatabaseForInstallEnvTest(t)()

	t.Setenv("GITEA__database__DB_TYPE", "postgres")
	t.Setenv("GITEA__database__HOST", "127.0.0.1:5432")
	t.Setenv("GITEA__database__USER", "gitea")
	t.Setenv("GITEA__database__NAME", "gitea")

	Database.Type = "postgres"
	Database.Host = "127.0.0.1:5432"
	Database.User = "gitea"
	Database.Name = "gitea"
	Database.SSLMode = "disable"

	assert.True(t, InstallDatabaseConfiguredViaEnvironment())
}

func TestInstallDatabaseConfiguredViaEnvironment_MySQLSplit(t *testing.T) {
	defer resetDatabaseForInstallEnvTest(t)()

	t.Setenv("GITEA__database__DB_TYPE", "mysql")
	t.Setenv("GITEA__database__HOST", "127.0.0.1:3306")
	t.Setenv("GITEA__database__USER", "root")
	t.Setenv("GITEA__database__NAME", "gitea")

	Database.Type = "mysql"
	Database.Host = "127.0.0.1:3306"
	Database.User = "root"
	Database.Name = "gitea"
	Database.SSLMode = "disable"

	assert.True(t, InstallDatabaseConfiguredViaEnvironment())
}

func TestInstallDatabaseConfiguredViaEnvironment_SplitMissingHost(t *testing.T) {
	defer resetDatabaseForInstallEnvTest(t)()

	t.Setenv("GITEA__database__DB_TYPE", "mysql")
	t.Setenv("GITEA__database__USER", "root")
	t.Setenv("GITEA__database__NAME", "gitea")

	Database.Type = "mysql"
	Database.Host = "127.0.0.1:3306"
	Database.User = "root"
	Database.Name = "gitea"
	Database.SSLMode = "disable"

	assert.False(t, InstallDatabaseConfiguredViaEnvironment())
}

func TestInstallDatabaseConfiguredViaEnvironment_SQLiteSplit(t *testing.T) {
	if !EnableSQLite3 {
		t.Skip("SQLite3 not enabled in this build")
	}
	defer resetDatabaseForInstallEnvTest(t)()

	t.Setenv("GITEA__database__DB_TYPE", "sqlite3")
	t.Setenv("GITEA__database__PATH", "/tmp/gitea-install-env-test.db")

	Database.Type = "sqlite3"
	Database.Path = "/tmp/gitea-install-env-test.db"

	assert.True(t, InstallDatabaseConfiguredViaEnvironment())
}

func TestHasGiteaEnvDatabaseKey(t *testing.T) {
	t.Setenv("GITEA__database__HOST", "127.0.0.1:3306")
	assert.True(t, hasGiteaEnvDatabaseKey("HOST"))
	assert.True(t, hasGiteaEnvDatabaseKey("host"))
	assert.False(t, hasGiteaEnvDatabaseKey("CONN_STR"))
}

func TestInstallAdminConfiguredViaEnvironment(t *testing.T) {
	t.Setenv(InstallAdminNameEnv, " admin ")
	t.Setenv(InstallAdminPasswordEnv, "password")
	t.Setenv(InstallAdminEmailEnv, " admin@example.com ")

	admin, ok := InstallAdminConfiguredViaEnvironment()

	assert.True(t, ok)
	assert.Equal(t, InstallAdminEnvironment{
		Name:     "admin",
		Password: "password",
		Email:    "admin@example.com",
	}, admin)
}

func TestInstallAdminConfiguredViaEnvironment_Incomplete(t *testing.T) {
	t.Setenv(InstallAdminNameEnv, "admin")
	t.Setenv(InstallAdminEmailEnv, "admin@example.com")

	_, ok := InstallAdminConfiguredViaEnvironment()

	assert.False(t, ok)
}

func TestInstallAutoConfiguredViaEnvironment(t *testing.T) {
	defer resetDatabaseForInstallEnvTest(t)()

	conn := "postgres://user:pass@127.0.0.1:5432/gitea?sslmode=disable"
	t.Setenv("GITEA__database__CONN_STR", conn)
	t.Setenv("GITEA__database__DB_TYPE", "postgres")
	t.Setenv(InstallAdminNameEnv, "admin")
	t.Setenv(InstallAdminPasswordEnv, "password")
	t.Setenv(InstallAdminEmailEnv, "admin@example.com")

	Database.Type = "postgres"
	Database.ConnStr = conn

	assert.True(t, InstallAutoConfiguredViaEnvironment())
}
