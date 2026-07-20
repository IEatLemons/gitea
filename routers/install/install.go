// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package install

import (
	std_context "context"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"code.gitea.io/gitea/models/db"
	db_install "code.gitea.io/gitea/models/db/install"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/auth/password/hash"
	"code.gitea.io/gitea/modules/generate"
	"code.gitea.io/gitea/modules/graceful"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/optional"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/user"
	"code.gitea.io/gitea/modules/web"
	"code.gitea.io/gitea/modules/web/middleware"
	"code.gitea.io/gitea/routers/common"
	auth_service "code.gitea.io/gitea/services/auth"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/forms"
	"code.gitea.io/gitea/services/versioned_migration"
)

const (
	// tplInstall template for installation page
	tplInstall     templates.TplName = "install"
	tplPostInstall templates.TplName = "post-install"
)

// getSupportedDbTypeNames returns a slice for supported database types and names. The slice is used to keep the order
func getSupportedDbTypeNames() (dbTypeNames []map[string]string) {
	for _, t := range setting.SupportedDatabaseTypes {
		dbTypeNames = append(dbTypeNames, map[string]string{"type": t, "name": setting.DatabaseTypeNames[t]})
	}
	return dbTypeNames
}

func installContexter() func(next http.Handler) http.Handler {
	return context.ContexterInstallPage(map[string]any{
		"DbTypeNames":            getSupportedDbTypeNames(),
		"EnvConfigKeys":          setting.CollectEnvConfigKeys(),
		"CustomConfFile":         setting.CustomConf,
		"PasswordHashAlgorithms": hash.RecommendedHashAlgorithms,
	})
}

func prepareInstallForm() forms.InstallForm {
	form := forms.InstallForm{}

	// Database settings
	form.DbType = setting.Database.Type.String()
	form.DbHost = setting.Database.Host
	form.DbUser = setting.Database.User
	form.DbPasswd = setting.Database.Passwd
	form.DbName = setting.Database.Name
	form.DbPath = setting.Database.Path
	form.DbSchema = setting.Database.Schema
	form.SSLMode = setting.Database.SSLMode

	// Application general settings
	form.AppName = setting.AppName
	form.RepoRootPath = setting.RepoRootPath
	form.LFSRootPath = setting.LFS.Storage.Path

	// Note(unknown): it's hard for Windows users change a running user,
	// 	so just use current one if config says default.
	if setting.IsWindows && setting.RunUser == "git" {
		form.RunUser = user.CurrentUsername()
	} else {
		form.RunUser = setting.RunUser
	}

	form.Domain = setting.Domain
	form.SSHPort = setting.SSH.Port
	form.HTTPPort = setting.HTTPPort
	form.AppURL = setting.AppURL
	form.LogRootPath = setting.Log.RootPath

	// E-mail service settings
	if setting.MailService != nil {
		form.SMTPAddr = setting.MailService.SMTPAddr
		form.SMTPPort = setting.MailService.SMTPPort
		form.SMTPFrom = setting.MailService.From
		form.SMTPUser = setting.MailService.User
		form.SMTPPasswd = setting.MailService.Passwd
	}
	form.RegisterConfirm = setting.Service.RegisterEmailConfirm
	form.MailNotify = setting.Service.EnableNotifyMail

	form.EnableOpenIDSignIn = setting.Service.EnableOpenIDSignIn
	form.EnableOpenIDSignUp = setting.Service.EnableOpenIDSignUp
	form.DisableRegistration = setting.Service.DisableRegistration
	form.AllowOnlyExternalRegistration = setting.Service.AllowOnlyExternalRegistration
	form.EnableCaptcha = setting.Service.EnableCaptcha
	form.RequireSignInView = setting.Service.RequireSignInViewStrict
	form.DefaultKeepEmailPrivate = setting.Service.DefaultKeepEmailPrivate
	form.DefaultAllowCreateOrganization = setting.Service.DefaultAllowCreateOrganization
	form.DefaultEnableTimetracking = setting.Service.DefaultEnableTimetracking
	form.NoReplyAddress = setting.Service.NoReplyAddress
	form.PasswordAlgorithm = hash.ConfigHashAlgorithm(setting.PasswordHashAlgo)

	return form
}

// Install render installation page
func Install(ctx *context.Context) {
	if setting.InstallLock {
		InstallDone(ctx)
		return
	}

	ctx.Data["SkipDatabaseForm"] = setting.InstallDatabaseConfiguredViaEnvironment()
	ctx.Data["DatabaseTypeForInstall"] = setting.Database.Type.String()

	form := prepareInstallForm()

	curDBType := form.DbType
	if !slices.Contains(setting.SupportedDatabaseTypes, curDBType) {
		curDBType = "mysql"
	}
	ctx.Data["CurDbType"] = curDBType

	middleware.AssignForm(form, ctx.Data)
	ctx.HTML(http.StatusOK, tplInstall)
}

func checkDatabase(ctx *context.Context, form *forms.InstallForm) bool {
	var err error

	if (setting.Database.Type == "sqlite3") &&
		len(setting.Database.Path) == 0 {
		ctx.Data["Err_DbPath"] = true
		ctx.RenderWithErrDeprecated(ctx.Tr("install.err_empty_db_path"), tplInstall, form)
		return false
	}

	// Check if the user is trying to re-install in an installed database
	db.UnsetDefaultEngine()
	defer db.UnsetDefaultEngine()

	if err = db.InitEngine(ctx); err != nil {
		if strings.Contains(err.Error(), `Unknown database type: sqlite3`) {
			ctx.Data["Err_DbType"] = true
			ctx.RenderWithErrDeprecated(ctx.Tr("install.sqlite3_not_available", "https://docs.gitea.com/installation/install-from-binary"), tplInstall, form)
		} else {
			ctx.Data["Err_DbSetting"] = true
			ctx.RenderWithErrDeprecated(ctx.Tr("install.invalid_db_setting", err), tplInstall, form)
		}
		return false
	}

	err = db_install.CheckDatabaseConnection(ctx)
	if err != nil {
		ctx.Data["Err_DbSetting"] = true
		ctx.RenderWithErrDeprecated(ctx.Tr("install.invalid_db_setting", err), tplInstall, form)
		return false
	}

	hasPostInstallationUser, err := db_install.HasPostInstallationUsers(ctx)
	if err != nil {
		ctx.Data["Err_DbSetting"] = true
		ctx.RenderWithErrDeprecated(ctx.Tr("install.invalid_db_table", "user", err), tplInstall, form)
		return false
	}
	dbMigrationVersion, err := db_install.GetMigrationVersion(ctx)
	if err != nil {
		ctx.Data["Err_DbSetting"] = true
		ctx.RenderWithErrDeprecated(ctx.Tr("install.invalid_db_table", "version", err), tplInstall, form)
		return false
	}

	if hasPostInstallationUser && dbMigrationVersion > 0 {
		log.Error("The database is likely to have been used by Gitea before, database migration version=%d", dbMigrationVersion)
		confirmed := form.ReinstallConfirmFirst && form.ReinstallConfirmSecond && form.ReinstallConfirmThird
		if !confirmed {
			ctx.Data["Err_DbInstalledBefore"] = true
			ctx.RenderWithErrDeprecated(ctx.Tr("install.reinstall_error"), tplInstall, form)
			return false
		}

		log.Info("User confirmed re-installation of Gitea into a pre-existing database")
	}

	if hasPostInstallationUser || dbMigrationVersion > 0 {
		log.Info("Gitea will be installed in a database with: hasPostInstallationUser=%v, dbMigrationVersion=%v", hasPostInstallationUser, dbMigrationVersion)
	}

	return true
}

func checkDatabaseForAutoInstall(ctx std_context.Context) (bool, error) {
	if setting.Database.Type == "sqlite3" && len(setting.Database.Path) == 0 {
		return false, fmt.Errorf("empty sqlite database path")
	}

	db.UnsetDefaultEngine()
	defer db.UnsetDefaultEngine()

	if err := db.InitEngine(ctx); err != nil {
		return false, fmt.Errorf("init database engine: %w", err)
	}
	if err := db_install.CheckDatabaseConnection(ctx); err != nil {
		return false, fmt.Errorf("check database connection: %w", err)
	}

	dbMigrationVersion, err := db_install.GetMigrationVersion(ctx)
	if err != nil {
		return false, fmt.Errorf("check version table: %w", err)
	}
	hasUsers, err := hasIndividualUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("check user table: %w", err)
	}
	if hasUsers {
		log.Warn("Database already has users; skipping unattended install")
		return false, nil
	}
	if dbMigrationVersion > 0 {
		log.Info("Gitea will be installed in a database with: hasIndividualUsers=%v, dbMigrationVersion=%d", hasUsers, dbMigrationVersion)
	}

	return true, nil
}

func hasIndividualUsers(ctx std_context.Context) (bool, error) {
	x := db.GetEngine(ctx)
	exist, err := x.IsTableExist("user")
	if err != nil {
		return false, err
	}
	if !exist {
		return false, nil
	}
	count, err := x.Table("user").Where("type = ?", user_model.UserTypeIndividual).Count(new(user_model.User))
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func applyInstallAdminEnvironment(form *forms.InstallForm) bool {
	admin, ok := setting.InstallAdminConfiguredViaEnvironment()
	if !ok {
		return false
	}
	form.AdminName = admin.Name
	form.AdminPasswd = admin.Password
	form.AdminConfirmPasswd = admin.Password
	form.AdminEmail = admin.Email
	return true
}

func validateInitialAdmin(form *forms.InstallForm) error {
	if len(form.AdminName) == 0 {
		return fmt.Errorf("initial admin name is empty")
	}
	if err := user_model.IsUsableUsername(form.AdminName); err != nil {
		return fmt.Errorf("initial admin name is invalid: %w", err)
	}
	if len(form.AdminEmail) == 0 {
		return fmt.Errorf("initial admin email is empty")
	}
	if _, err := mail.ParseAddress(form.AdminEmail); err != nil {
		return fmt.Errorf("initial admin email is invalid: %w", err)
	}
	if strings.TrimSpace(form.AdminPasswd) == "" {
		return fmt.Errorf("initial admin password is empty")
	}
	if form.AdminPasswd != form.AdminConfirmPasswd {
		return fmt.Errorf("initial admin password confirmation does not match")
	}
	return nil
}

func prepareInstallPaths(form *forms.InstallForm) error {
	if err := setting.PrepareAppDataPath(); err != nil {
		return fmt.Errorf("prepare app data path: %w", err)
	}

	form.RepoRootPath = strings.ReplaceAll(form.RepoRootPath, "\\", "/")
	if err := os.MkdirAll(form.RepoRootPath, os.ModePerm); err != nil {
		return fmt.Errorf("create repository root path: %w", err)
	}

	if form.LFSRootPath != "" {
		form.LFSRootPath = strings.ReplaceAll(form.LFSRootPath, "\\", "/")
		if err := os.MkdirAll(form.LFSRootPath, os.ModePerm); err != nil {
			return fmt.Errorf("create lfs root path: %w", err)
		}
	}

	form.LogRootPath = strings.ReplaceAll(form.LogRootPath, "\\", "/")
	if err := os.MkdirAll(form.LogRootPath, os.ModePerm); err != nil {
		return fmt.Errorf("create log root path: %w", err)
	}

	currentUser, match := setting.IsRunUserMatchCurrentUser(form.RunUser)
	if !match {
		return fmt.Errorf("run user %q does not match current user %q", form.RunUser, currentUser)
	}
	return nil
}

func saveInstallConfig(form *forms.InstallForm) error {
	cfg, err := setting.NewConfigProviderFromFile(setting.CustomConf)
	if err != nil {
		return fmt.Errorf("load custom config %q: %w", setting.CustomConf, err)
	}

	cfg.Section("").Key("APP_NAME").SetValue(form.AppName)
	cfg.Section("").Key("RUN_USER").SetValue(form.RunUser)
	cfg.Section("").Key("WORK_PATH").SetValue(setting.AppWorkPath)
	cfg.Section("").Key("RUN_MODE").SetValue("prod")

	cfg.Section("database").Key("DB_TYPE").SetValue(setting.Database.Type.String())
	cfg.Section("database").Key("HOST").SetValue(setting.Database.Host)
	cfg.Section("database").Key("NAME").SetValue(setting.Database.Name)
	cfg.Section("database").Key("USER").SetValue(setting.Database.User)
	cfg.Section("database").Key("PASSWD").SetValue(setting.Database.Passwd)
	cfg.Section("database").Key("SCHEMA").SetValue(setting.Database.Schema)
	cfg.Section("database").Key("SSL_MODE").SetValue(setting.Database.SSLMode)
	cfg.Section("database").Key("PATH").SetValue(setting.Database.Path)
	cfg.Section("database").Key("LOG_SQL").SetValue("false")
	if setting.Database.ConnStr != "" {
		cfg.Section("database").Key("CONN_STR").SetValue(setting.Database.ConnStr)
	}

	cfg.Section("repository").Key("ROOT").SetValue(form.RepoRootPath)
	cfg.Section("server").Key("SSH_DOMAIN").SetValue(form.Domain)
	cfg.Section("server").Key("DOMAIN").SetValue(form.Domain)
	cfg.Section("server").Key("HTTP_PORT").SetValue(form.HTTPPort)
	cfg.Section("server").Key("ROOT_URL").SetValue(form.AppURL)
	cfg.Section("server").Key("APP_DATA_PATH").SetValue(setting.AppDataPath)

	if form.SSHPort == 0 {
		cfg.Section("server").Key("DISABLE_SSH").SetValue("true")
	} else {
		cfg.Section("server").Key("DISABLE_SSH").SetValue("false")
		cfg.Section("server").Key("SSH_PORT").SetValue(strconv.Itoa(form.SSHPort))
	}

	if form.LFSRootPath != "" {
		cfg.Section("server").Key("LFS_START_SERVER").SetValue("true")
		cfg.Section("lfs").Key("PATH").SetValue(form.LFSRootPath)

		if !cfg.Section("server").HasKey("LFS_JWT_SECRET_URI") {
			_, lfsJwtSecret := generate.NewJwtSecretWithBase64()
			cfg.Section("server").Key("LFS_JWT_SECRET").SetValue(lfsJwtSecret)
		}
	} else {
		cfg.Section("server").Key("LFS_START_SERVER").SetValue("false")
	}

	if len(strings.TrimSpace(form.SMTPAddr)) > 0 {
		if _, err := mail.ParseAddress(form.SMTPFrom); err != nil {
			return fmt.Errorf("smtp from address is invalid: %w", err)
		}

		cfg.Section("mailer").Key("ENABLED").SetValue("true")
		cfg.Section("mailer").Key("SMTP_ADDR").SetValue(form.SMTPAddr)
		cfg.Section("mailer").Key("SMTP_PORT").SetValue(form.SMTPPort)
		cfg.Section("mailer").Key("FROM").SetValue(form.SMTPFrom)
		cfg.Section("mailer").Key("USER").SetValue(form.SMTPUser)
		cfg.Section("mailer").Key("PASSWD").SetValue(form.SMTPPasswd)
	} else {
		cfg.Section("mailer").Key("ENABLED").SetValue("false")
	}
	cfg.Section("service").Key("REGISTER_EMAIL_CONFIRM").SetValue(strconv.FormatBool(form.RegisterConfirm))
	cfg.Section("service").Key("ENABLE_NOTIFY_MAIL").SetValue(strconv.FormatBool(form.MailNotify))

	cfg.Section("openid").Key("ENABLE_OPENID_SIGNIN").SetValue(strconv.FormatBool(form.EnableOpenIDSignIn))
	cfg.Section("openid").Key("ENABLE_OPENID_SIGNUP").SetValue(strconv.FormatBool(form.EnableOpenIDSignUp))
	cfg.Section("service").Key("DISABLE_REGISTRATION").SetValue(strconv.FormatBool(form.DisableRegistration))
	cfg.Section("service").Key("ALLOW_ONLY_EXTERNAL_REGISTRATION").SetValue(strconv.FormatBool(form.AllowOnlyExternalRegistration))
	cfg.Section("service").Key("ENABLE_CAPTCHA").SetValue(strconv.FormatBool(form.EnableCaptcha))
	cfg.Section("service").Key("REQUIRE_SIGNIN_VIEW").SetValue(strconv.FormatBool(form.RequireSignInView))
	cfg.Section("service").Key("DEFAULT_KEEP_EMAIL_PRIVATE").SetValue(strconv.FormatBool(form.DefaultKeepEmailPrivate))
	cfg.Section("service").Key("DEFAULT_ALLOW_CREATE_ORGANIZATION").SetValue(strconv.FormatBool(form.DefaultAllowCreateOrganization))
	cfg.Section("service").Key("DEFAULT_ENABLE_TIMETRACKING").SetValue(strconv.FormatBool(form.DefaultEnableTimetracking))
	cfg.Section("service").Key("NO_REPLY_ADDRESS").SetValue(form.NoReplyAddress)
	cfg.Section("cron.update_checker").Key("ENABLED").SetValue(strconv.FormatBool(form.EnableUpdateChecker))

	cfg.Section("session").Key("PROVIDER").SetValue("file")

	cfg.Section("log").Key("MODE").MustString("console")
	cfg.Section("log").Key("LEVEL").SetValue(setting.Log.Level.String())
	cfg.Section("log").Key("ROOT_PATH").SetValue(form.LogRootPath)

	cfg.Section("repository.pull-request").Key("DEFAULT_MERGE_STYLE").SetValue("merge")

	cfg.Section("repository.signing").Key("DEFAULT_TRUST_MODEL").SetValue("committer")

	cfg.Section("security").Key("INSTALL_LOCK").SetValue("true")

	if setting.InternalToken == "" {
		internalToken, err := generate.NewInternalToken()
		if err != nil {
			return fmt.Errorf("generate internal token: %w", err)
		}
		cfg.Section("security").Key("INTERNAL_TOKEN").SetValue(internalToken)
	}

	if !cfg.Section("oauth2").HasKey("JWT_SECRET") && !cfg.Section("oauth2").HasKey("JWT_SECRET_URI") {
		_, jwtSecretBase64 := generate.NewJwtSecretWithBase64()
		cfg.Section("oauth2").Key("JWT_SECRET").SetValue(jwtSecretBase64)
	}

	if setting.SecretKey == "" {
		secretKey, err := generate.NewSecretKey()
		if err != nil {
			return fmt.Errorf("generate secret key: %w", err)
		}
		cfg.Section("security").Key("SECRET_KEY").SetValue(secretKey)
	}

	if len(form.PasswordAlgorithm) > 0 {
		var algorithm *hash.PasswordHashAlgorithm
		setting.PasswordHashAlgo, algorithm = hash.SetDefaultPasswordHashAlgorithm(form.PasswordAlgorithm)
		if algorithm == nil {
			return fmt.Errorf("invalid password hash algorithm")
		}
		cfg.Section("security").Key("PASSWORD_HASH_ALGO").SetValue(form.PasswordAlgorithm)
	}

	log.Info("Save settings to custom config file %s", setting.CustomConf)

	if err := os.MkdirAll(filepath.Dir(setting.CustomConf), os.ModePerm); err != nil {
		return fmt.Errorf("create custom config directory: %w", err)
	}

	setting.EnvironmentToConfig(cfg, os.Environ())
	cfg.Section("security").Key("INSTALL_LOCK").SetValue("true")

	if err := cfg.SaveTo(setting.CustomConf); err != nil {
		return fmt.Errorf("save custom config: %w", err)
	}
	return nil
}

func forceInstallLock() error {
	cfg, err := setting.NewConfigProviderFromFile(setting.CustomConf)
	if err != nil {
		return fmt.Errorf("load custom config %q: %w", setting.CustomConf, err)
	}
	setting.EnvironmentToConfig(cfg, os.Environ())
	cfg.Section("security").Key("INSTALL_LOCK").SetValue("true")
	if err := os.MkdirAll(filepath.Dir(setting.CustomConf), os.ModePerm); err != nil {
		return fmt.Errorf("create custom config directory: %w", err)
	}
	if err := cfg.SaveTo(setting.CustomConf); err != nil {
		return fmt.Errorf("save install lock: %w", err)
	}
	if setting.CfgProvider != nil {
		setting.CfgProvider.Section("security").Key("INSTALL_LOCK").SetValue("true")
	}
	setting.InstallLock = true
	return nil
}

func createInitialAdmin(ctx std_context.Context, form *forms.InstallForm) (*user_model.User, error) {
	u := &user_model.User{
		Name:    form.AdminName,
		Email:   form.AdminEmail,
		Passwd:  form.AdminPasswd,
		IsAdmin: true,
	}
	overwriteDefault := &user_model.CreateUserOverwriteOptions{
		IsRestricted: optional.Some(false),
		IsActive:     optional.Some(true),
	}

	if err := user_model.CreateUser(ctx, u, &user_model.Meta{}, overwriteDefault); err != nil {
		if !user_model.IsErrUserAlreadyExist(err) {
			return nil, fmt.Errorf("create initial admin: %w", err)
		}
		log.Info("Admin account already exists")
		u, err = user_model.GetUserByName(ctx, u.Name)
		if err != nil {
			return nil, fmt.Errorf("load existing initial admin: %w", err)
		}
	}
	return u, nil
}

func AutoInstall(ctx std_context.Context) error {
	if setting.InstallLock {
		return nil
	}
	if !setting.InstallAutoConfiguredViaEnvironment() {
		return fmt.Errorf("unattended install requires database environment and %s, %s, %s", setting.InstallAdminNameEnv, setting.InstallAdminPasswordEnv, setting.InstallAdminEmailEnv)
	}

	form := prepareInstallForm()
	applyInstallAdminEnvironment(&form)

	if form.AppURL != "" && form.AppURL[len(form.AppURL)-1] != '/' {
		form.AppURL += "/"
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("test git: %w", err)
	}

	setting.Database.LogSQL = !setting.IsProd

	shouldInstall, err := checkDatabaseForAutoInstall(ctx)
	if err != nil {
		return err
	}
	if !shouldInstall {
		return forceInstallLock()
	}
	if err := prepareInstallPaths(&form); err != nil {
		return err
	}
	if err := validateInitialAdmin(&form); err != nil {
		return err
	}
	if err := db.InitEngineWithMigration(ctx, versioned_migration.Migrate); err != nil {
		db.UnsetDefaultEngine()
		return fmt.Errorf("run database migrations: %w", err)
	}
	if err := saveInstallConfig(&form); err != nil {
		return err
	}

	db.UnsetDefaultEngine()

	setting.InitCfgProvider(setting.CustomConf)
	setting.LoadCommonSettings()
	setting.MustInstalled()
	setting.LoadDBSetting()
	if err := common.InitDBEngine(ctx); err != nil {
		return fmt.Errorf("initialize installed database engine: %w", err)
	}
	if _, err := createInitialAdmin(ctx, &form); err != nil {
		return err
	}

	setting.ClearEnvConfigKeys()
	log.Info("First-time unattended install finished!")
	return nil
}

func EnsureInitialAdmin(ctx std_context.Context) error {
	form := forms.InstallForm{}
	if !applyInstallAdminEnvironment(&form) {
		return nil
	}

	hasUsers, err := hasIndividualUsers(ctx)
	if err != nil {
		return fmt.Errorf("check user table before initial admin bootstrap: %w", err)
	}
	if hasUsers {
		return nil
	}
	if err := validateInitialAdmin(&form); err != nil {
		return err
	}

	if _, err := createInitialAdmin(ctx, &form); err != nil {
		return err
	}
	log.Info("Initial admin account created from environment")
	return nil
}

// SubmitInstall response for submit install items
func SubmitInstall(ctx *context.Context) {
	if setting.InstallLock {
		InstallDone(ctx)
		return
	}

	var err error

	form := *web.GetForm(ctx).(*forms.InstallForm)

	ctx.Data["SkipDatabaseForm"] = setting.InstallDatabaseConfiguredViaEnvironment()
	ctx.Data["DatabaseTypeForInstall"] = setting.Database.Type.String()

	// fix form values
	if form.AppURL != "" && form.AppURL[len(form.AppURL)-1] != '/' {
		form.AppURL += "/"
	}

	ctx.Data["CurDbType"] = form.DbType

	if ctx.HasError() {
		ctx.Data["Err_SMTP"] = ctx.Data["Err_SMTPUser"] != nil
		ctx.Data["Err_Admin"] = ctx.Data["Err_AdminName"] != nil || ctx.Data["Err_AdminPasswd"] != nil || ctx.Data["Err_AdminEmail"] != nil
		ctx.HTML(http.StatusOK, tplInstall)
		return
	}

	if _, err = exec.LookPath("git"); err != nil {
		ctx.RenderWithErrDeprecated(ctx.Tr("install.test_git_failed", err), tplInstall, &form)
		return
	}

	// ---- Basic checks are passed, now test configuration.

	// Test database setting.
	if !setting.InstallDatabaseConfiguredViaEnvironment() {
		setting.Database.Type = setting.DatabaseType(form.DbType)
		setting.Database.Host = form.DbHost
		setting.Database.User = form.DbUser
		setting.Database.Passwd = form.DbPasswd
		setting.Database.Name = form.DbName
		setting.Database.Schema = form.DbSchema
		setting.Database.SSLMode = form.SSLMode
		setting.Database.Path = form.DbPath
	}
	setting.Database.LogSQL = !setting.IsProd

	if !checkDatabase(ctx, &form) {
		return
	}

	// Prepare AppDataPath, it is very important for Gitea
	if err = setting.PrepareAppDataPath(); err != nil {
		ctx.RenderWithErrDeprecated(ctx.Tr("install.invalid_app_data_path", err), tplInstall, &form)
		return
	}

	// Test repository root path.
	form.RepoRootPath = strings.ReplaceAll(form.RepoRootPath, "\\", "/")
	if err = os.MkdirAll(form.RepoRootPath, os.ModePerm); err != nil {
		ctx.Data["Err_RepoRootPath"] = true
		ctx.RenderWithErrDeprecated(ctx.Tr("install.invalid_repo_path", err), tplInstall, &form)
		return
	}

	// Test LFS root path if not empty, empty meaning disable LFS
	if form.LFSRootPath != "" {
		form.LFSRootPath = strings.ReplaceAll(form.LFSRootPath, "\\", "/")
		if err := os.MkdirAll(form.LFSRootPath, os.ModePerm); err != nil {
			ctx.Data["Err_LFSRootPath"] = true
			ctx.RenderWithErrDeprecated(ctx.Tr("install.invalid_lfs_path", err), tplInstall, &form)
			return
		}
	}

	// Test log root path.
	form.LogRootPath = strings.ReplaceAll(form.LogRootPath, "\\", "/")
	if err = os.MkdirAll(form.LogRootPath, os.ModePerm); err != nil {
		ctx.Data["Err_LogRootPath"] = true
		ctx.RenderWithErrDeprecated(ctx.Tr("install.invalid_log_root_path", err), tplInstall, &form)
		return
	}

	currentUser, match := setting.IsRunUserMatchCurrentUser(form.RunUser)
	if !match {
		ctx.Data["Err_RunUser"] = true
		ctx.RenderWithErrDeprecated(ctx.Tr("install.run_user_not_match", form.RunUser, currentUser), tplInstall, &form)
		return
	}

	// Check logic loophole between disable self-registration and no admin account.
	if form.DisableRegistration && len(form.AdminName) == 0 {
		ctx.Data["Err_Services"] = true
		ctx.Data["Err_Admin"] = true
		ctx.RenderWithErrDeprecated(ctx.Tr("install.no_admin_and_disable_registration"), tplInstall, form)
		return
	}

	// Check admin user creation
	if len(form.AdminName) > 0 {
		// Ensure AdminName is valid
		if err := user_model.IsUsableUsername(form.AdminName); err != nil {
			ctx.Data["Err_Admin"] = true
			ctx.Data["Err_AdminName"] = true
			if db.IsErrNameReserved(err) {
				ctx.RenderWithErrDeprecated(ctx.Tr("install.err_admin_name_is_reserved"), tplInstall, form)
				return
			} else if db.IsErrNamePatternNotAllowed(err) {
				ctx.RenderWithErrDeprecated(ctx.Tr("install.err_admin_name_pattern_not_allowed"), tplInstall, form)
				return
			}
			ctx.RenderWithErrDeprecated(ctx.Tr("install.err_admin_name_is_invalid"), tplInstall, form)
			return
		}
		// Check Admin email
		if len(form.AdminEmail) == 0 {
			ctx.Data["Err_Admin"] = true
			ctx.Data["Err_AdminEmail"] = true
			ctx.RenderWithErrDeprecated(ctx.Tr("install.err_empty_admin_email"), tplInstall, form)
			return
		}
		// Check admin password.
		if len(form.AdminPasswd) == 0 {
			ctx.Data["Err_Admin"] = true
			ctx.Data["Err_AdminPasswd"] = true
			ctx.RenderWithErrDeprecated(ctx.Tr("install.err_empty_admin_password"), tplInstall, form)
			return
		}
		if form.AdminPasswd != form.AdminConfirmPasswd {
			ctx.Data["Err_Admin"] = true
			ctx.Data["Err_AdminPasswd"] = true
			ctx.RenderWithErrDeprecated(ctx.Tr("form.password_not_match"), tplInstall, form)
			return
		}
	}

	// Init the engine with migration
	if err = db.InitEngineWithMigration(ctx, versioned_migration.Migrate); err != nil {
		db.UnsetDefaultEngine()
		ctx.Data["Err_DbSetting"] = true
		ctx.RenderWithErrDeprecated(ctx.Tr("install.invalid_db_setting", err), tplInstall, &form)
		return
	}

	// Save settings.
	cfg, err := setting.NewConfigProviderFromFile(setting.CustomConf)
	if err != nil {
		log.Error("Failed to load custom conf '%s': %v", setting.CustomConf, err)
	}

	cfg.Section("").Key("APP_NAME").SetValue(form.AppName)
	cfg.Section("").Key("RUN_USER").SetValue(form.RunUser)
	cfg.Section("").Key("WORK_PATH").SetValue(setting.AppWorkPath)
	cfg.Section("").Key("RUN_MODE").SetValue("prod")

	cfg.Section("database").Key("DB_TYPE").SetValue(setting.Database.Type.String())
	cfg.Section("database").Key("HOST").SetValue(setting.Database.Host)
	cfg.Section("database").Key("NAME").SetValue(setting.Database.Name)
	cfg.Section("database").Key("USER").SetValue(setting.Database.User)
	cfg.Section("database").Key("PASSWD").SetValue(setting.Database.Passwd)
	cfg.Section("database").Key("SCHEMA").SetValue(setting.Database.Schema)
	cfg.Section("database").Key("SSL_MODE").SetValue(setting.Database.SSLMode)
	cfg.Section("database").Key("PATH").SetValue(setting.Database.Path)
	cfg.Section("database").Key("LOG_SQL").SetValue("false") // LOG_SQL is rarely helpful
	if setting.Database.ConnStr != "" {
		cfg.Section("database").Key("CONN_STR").SetValue(setting.Database.ConnStr)
	}

	cfg.Section("repository").Key("ROOT").SetValue(form.RepoRootPath)
	cfg.Section("server").Key("SSH_DOMAIN").SetValue(form.Domain)
	cfg.Section("server").Key("DOMAIN").SetValue(form.Domain)
	cfg.Section("server").Key("HTTP_PORT").SetValue(form.HTTPPort)
	cfg.Section("server").Key("ROOT_URL").SetValue(form.AppURL)
	cfg.Section("server").Key("APP_DATA_PATH").SetValue(setting.AppDataPath)

	if form.SSHPort == 0 {
		cfg.Section("server").Key("DISABLE_SSH").SetValue("true")
	} else {
		cfg.Section("server").Key("DISABLE_SSH").SetValue("false")
		cfg.Section("server").Key("SSH_PORT").SetValue(strconv.Itoa(form.SSHPort))
	}

	if form.LFSRootPath != "" {
		cfg.Section("server").Key("LFS_START_SERVER").SetValue("true")
		cfg.Section("lfs").Key("PATH").SetValue(form.LFSRootPath)

		if !cfg.Section("server").HasKey("LFS_JWT_SECRET_URI") {
			_, lfsJwtSecret := generate.NewJwtSecretWithBase64()
			cfg.Section("server").Key("LFS_JWT_SECRET").SetValue(lfsJwtSecret)
		}
	} else {
		cfg.Section("server").Key("LFS_START_SERVER").SetValue("false")
	}

	if len(strings.TrimSpace(form.SMTPAddr)) > 0 {
		if _, err := mail.ParseAddress(form.SMTPFrom); err != nil {
			ctx.RenderWithErrDeprecated(ctx.Tr("install.smtp_from_invalid"), tplInstall, &form)
			return
		}

		cfg.Section("mailer").Key("ENABLED").SetValue("true")
		cfg.Section("mailer").Key("SMTP_ADDR").SetValue(form.SMTPAddr)
		cfg.Section("mailer").Key("SMTP_PORT").SetValue(form.SMTPPort)
		cfg.Section("mailer").Key("FROM").SetValue(form.SMTPFrom)
		cfg.Section("mailer").Key("USER").SetValue(form.SMTPUser)
		cfg.Section("mailer").Key("PASSWD").SetValue(form.SMTPPasswd)
	} else {
		cfg.Section("mailer").Key("ENABLED").SetValue("false")
	}
	cfg.Section("service").Key("REGISTER_EMAIL_CONFIRM").SetValue(strconv.FormatBool(form.RegisterConfirm))
	cfg.Section("service").Key("ENABLE_NOTIFY_MAIL").SetValue(strconv.FormatBool(form.MailNotify))

	cfg.Section("openid").Key("ENABLE_OPENID_SIGNIN").SetValue(strconv.FormatBool(form.EnableOpenIDSignIn))
	cfg.Section("openid").Key("ENABLE_OPENID_SIGNUP").SetValue(strconv.FormatBool(form.EnableOpenIDSignUp))
	cfg.Section("service").Key("DISABLE_REGISTRATION").SetValue(strconv.FormatBool(form.DisableRegistration))
	cfg.Section("service").Key("ALLOW_ONLY_EXTERNAL_REGISTRATION").SetValue(strconv.FormatBool(form.AllowOnlyExternalRegistration))
	cfg.Section("service").Key("ENABLE_CAPTCHA").SetValue(strconv.FormatBool(form.EnableCaptcha))
	cfg.Section("service").Key("REQUIRE_SIGNIN_VIEW").SetValue(strconv.FormatBool(form.RequireSignInView))
	cfg.Section("service").Key("DEFAULT_KEEP_EMAIL_PRIVATE").SetValue(strconv.FormatBool(form.DefaultKeepEmailPrivate))
	cfg.Section("service").Key("DEFAULT_ALLOW_CREATE_ORGANIZATION").SetValue(strconv.FormatBool(form.DefaultAllowCreateOrganization))
	cfg.Section("service").Key("DEFAULT_ENABLE_TIMETRACKING").SetValue(strconv.FormatBool(form.DefaultEnableTimetracking))
	cfg.Section("service").Key("NO_REPLY_ADDRESS").SetValue(form.NoReplyAddress)
	cfg.Section("cron.update_checker").Key("ENABLED").SetValue(strconv.FormatBool(form.EnableUpdateChecker))

	cfg.Section("session").Key("PROVIDER").SetValue("file")

	cfg.Section("log").Key("MODE").MustString("console")
	cfg.Section("log").Key("LEVEL").SetValue(setting.Log.Level.String())
	cfg.Section("log").Key("ROOT_PATH").SetValue(form.LogRootPath)

	cfg.Section("repository.pull-request").Key("DEFAULT_MERGE_STYLE").SetValue("merge")

	cfg.Section("repository.signing").Key("DEFAULT_TRUST_MODEL").SetValue("committer")

	cfg.Section("security").Key("INSTALL_LOCK").SetValue("true")

	// the internal token could be read from INTERNAL_TOKEN or INTERNAL_TOKEN_URI (the file is guaranteed to be non-empty)
	// if there is no InternalToken, generate one and save to security.INTERNAL_TOKEN
	if setting.InternalToken == "" {
		var internalToken string
		if internalToken, err = generate.NewInternalToken(); err != nil {
			ctx.RenderWithErrDeprecated(ctx.Tr("install.internal_token_failed", err), tplInstall, &form)
			return
		}
		cfg.Section("security").Key("INTERNAL_TOKEN").SetValue(internalToken)
	}

	// FIXME: at the moment, no matter oauth2 is enabled or not, it must generate a "oauth2 JWT_SECRET"
	// see the "loadOAuth2From" in "setting/oauth2.go"
	if !cfg.Section("oauth2").HasKey("JWT_SECRET") && !cfg.Section("oauth2").HasKey("JWT_SECRET_URI") {
		_, jwtSecretBase64 := generate.NewJwtSecretWithBase64()
		cfg.Section("oauth2").Key("JWT_SECRET").SetValue(jwtSecretBase64)
	}

	// if there is already a SECRET_KEY, we should not overwrite it, otherwise the encrypted data will not be able to be decrypted
	if setting.SecretKey == "" {
		var secretKey string
		if secretKey, err = generate.NewSecretKey(); err != nil {
			ctx.RenderWithErrDeprecated(ctx.Tr("install.secret_key_failed", err), tplInstall, &form)
			return
		}
		cfg.Section("security").Key("SECRET_KEY").SetValue(secretKey)
	}

	if len(form.PasswordAlgorithm) > 0 {
		var algorithm *hash.PasswordHashAlgorithm
		setting.PasswordHashAlgo, algorithm = hash.SetDefaultPasswordHashAlgorithm(form.PasswordAlgorithm)
		if algorithm == nil {
			ctx.RenderWithErrDeprecated(ctx.Tr("install.invalid_password_algorithm"), tplInstall, &form)
			return
		}
		cfg.Section("security").Key("PASSWORD_HASH_ALGO").SetValue(form.PasswordAlgorithm)
	}

	log.Info("Save settings to custom config file %s", setting.CustomConf)

	err = os.MkdirAll(filepath.Dir(setting.CustomConf), os.ModePerm)
	if err != nil {
		ctx.RenderWithErrDeprecated(ctx.Tr("install.save_config_failed", err), tplInstall, &form)
		return
	}

	setting.EnvironmentToConfig(cfg, os.Environ())
	cfg.Section("security").Key("INSTALL_LOCK").SetValue("true")

	if err = cfg.SaveTo(setting.CustomConf); err != nil {
		ctx.RenderWithErrDeprecated(ctx.Tr("install.save_config_failed", err), tplInstall, &form)
		return
	}

	// unset default engine before reload database setting
	db.UnsetDefaultEngine()

	// ---- All checks are passed

	// Reload settings (and re-initialize database connection)
	setting.InitCfgProvider(setting.CustomConf)
	setting.LoadCommonSettings()
	setting.MustInstalled()
	setting.LoadDBSetting()
	if err := common.InitDBEngine(ctx); err != nil {
		log.Fatal("ORM engine initialization failed: %v", err)
	}

	// Create admin account
	if len(form.AdminName) > 0 {
		u := &user_model.User{
			Name:    form.AdminName,
			Email:   form.AdminEmail,
			Passwd:  form.AdminPasswd,
			IsAdmin: true,
		}
		overwriteDefault := &user_model.CreateUserOverwriteOptions{
			IsRestricted: optional.Some(false),
			IsActive:     optional.Some(true),
		}

		if err = user_model.CreateUser(ctx, u, &user_model.Meta{}, overwriteDefault); err != nil {
			if !user_model.IsErrUserAlreadyExist(err) {
				setting.InstallLock = false
				ctx.Data["Err_AdminName"] = true
				ctx.Data["Err_AdminEmail"] = true
				ctx.RenderWithErrDeprecated(ctx.Tr("install.invalid_admin_setting", err), tplInstall, &form)
				return
			}
			log.Info("Admin account already exist")
			u, _ = user_model.GetUserByName(ctx, u.Name)
		}

		nt, token, err := auth_service.CreateAuthTokenForUserID(ctx, u.ID)
		if err != nil {
			ctx.ServerError("CreateAuthTokenForUserID", err)
			return
		}

		ctx.SetSiteCookie(setting.CookieRememberName, nt.ID+":"+token, setting.LogInRememberDays*timeutil.Day)

		// Auto-login for admin
		if err = ctx.Session.Set("uid", u.ID); err != nil {
			ctx.RenderWithErrDeprecated(ctx.Tr("install.save_config_failed", err), tplInstall, &form)
			return
		}
		if err = ctx.Session.Set("uname", u.Name); err != nil {
			ctx.RenderWithErrDeprecated(ctx.Tr("install.save_config_failed", err), tplInstall, &form)
			return
		}

		if err = ctx.Session.Release(); err != nil {
			ctx.RenderWithErrDeprecated(ctx.Tr("install.save_config_failed", err), tplInstall, &form)
			return
		}
	}

	setting.ClearEnvConfigKeys()
	log.Info("First-time run install finished!")
	InstallDone(ctx)

	go func() {
		// Sleep for a while to make sure the user's browser has loaded the post-install page and its assets (images, css, js)
		// What if this duration is not long enough? That's impossible -- if the user can't load the simple page in time, how could they install or use Gitea in the future ....
		time.Sleep(3 * time.Second)

		// Now get the http.Server from this request and shut it down
		// NB: This is not our hammerable graceful shutdown this is http.Server.Shutdown
		srv := ctx.Value(http.ServerContextKey).(*http.Server)
		if err := srv.Shutdown(graceful.GetManager().HammerContext()); err != nil {
			log.Error("Unable to shutdown the install server! Error: %v", err)
		}

		// After the HTTP server for "install" shuts down, the `runWeb()` will continue to run the "normal" server
	}()
}

// InstallDone shows the "post-install" page, makes it easier to develop the page.
// The name is not called as "PostInstall" to avoid misinterpretation as a handler for "POST /install"
func InstallDone(ctx *context.Context) { //nolint:revive // export stutter
	hasUsers, _ := user_model.HasUsers(ctx)
	ctx.Data["IsAccountCreated"] = hasUsers.HasAnyUser
	ctx.HTML(http.StatusOK, tplPostInstall)
}
