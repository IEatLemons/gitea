// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/services/migrations"
	mirror_service "code.gitea.io/gitea/services/mirror"
	repo_service "code.gitea.io/gitea/services/repository"
	wiki_service "code.gitea.io/gitea/services/wiki"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMirrorPush(t *testing.T) {
	onGiteaRun(t, testMirrorPush)
}

func TestMirrorPushWikiDefaultBranchMismatch(t *testing.T) {
	onGiteaRun(t, testMirrorPushWikiDefaultBranchMismatch)
}

func testMirrorPush(t *testing.T, u *url.URL) {
	setting.Migrations.AllowLocalNetworks = true
	assert.NoError(t, migrations.Init())

	_ = db.TruncateBeans(t.Context(), &repo_model.PushMirror{})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	srcRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	mirrorRepo, err := repo_service.CreateRepositoryDirectly(t.Context(), user, user, repo_service.CreateRepoOptions{
		Name: "test-push-mirror",
	}, true)
	assert.NoError(t, err)

	session := loginUser(t, user.Name)

	pushMirrorURL := fmt.Sprintf("%s%s/%s", u.String(), url.PathEscape(user.Name), url.PathEscape(mirrorRepo.Name))
	testCreatePushMirror(t, session, user.Name, srcRepo.Name, pushMirrorURL, user.LowerName, userPassword, "0")

	mirrors, _, err := repo_model.GetPushMirrorsByRepoID(t.Context(), srcRepo.ID, db.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, mirrors, 1)

	ok := mirror_service.SyncPushMirror(t.Context(), mirrors[0].ID, "")
	assert.True(t, ok)

	srcGitRepo, err := gitrepo.OpenRepository(t.Context(), srcRepo)
	assert.NoError(t, err)
	defer srcGitRepo.Close()

	srcCommit, err := srcGitRepo.GetBranchCommit("master")
	assert.NoError(t, err)

	mirrorGitRepo, err := gitrepo.OpenRepository(t.Context(), mirrorRepo)
	assert.NoError(t, err)
	defer mirrorGitRepo.Close()

	mirrorCommit, err := mirrorGitRepo.GetBranchCommit("master")
	assert.NoError(t, err)

	assert.Equal(t, srcCommit.ID, mirrorCommit.ID)

	// Cleanup
	assert.True(t, doRemovePushMirror(t, session, user.Name, srcRepo.Name, mirrors[0].ID))
	mirrors, _, err = repo_model.GetPushMirrorsByRepoID(t.Context(), srcRepo.ID, db.ListOptions{})
	assert.NoError(t, err)
	assert.Empty(t, mirrors)
}

func testMirrorPushWikiDefaultBranchMismatch(t *testing.T, u *url.URL) {
	setting.Migrations.AllowLocalNetworks = true
	assert.NoError(t, migrations.Init())

	_ = db.TruncateBeans(t.Context(), &repo_model.PushMirror{})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	srcRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	mirrorRepo, err := repo_service.CreateRepositoryDirectly(t.Context(), user, user, repo_service.CreateRepoOptions{
		Name: "test-push-mirror-wiki",
	}, true)
	assert.NoError(t, err)

	assert.NoError(t, wiki_service.AddWikiPage(t.Context(), user, mirrorRepo, wiki_service.WebPath("Home"), "Mirror wiki content", "init wiki"))

	mirrorRepo.DefaultBranch = "mirror-head"
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), mirrorRepo, "default_branch"))

	wikiCommitID, err := gitrepo.GetBranchCommitID(t.Context(), mirrorRepo.WikiStorageRepo(), mirrorRepo.DefaultWikiBranch)
	assert.NoError(t, err)
	assert.NoError(t, gitrepo.CreateBranch(t.Context(), mirrorRepo.WikiStorageRepo(), "mirror-head", wikiCommitID))

	session := loginUser(t, user.Name)

	pushMirrorURL := fmt.Sprintf("%s%s/%s", u.String(), url.PathEscape(user.Name), url.PathEscape(mirrorRepo.Name))
	testCreatePushMirror(t, session, user.Name, srcRepo.Name, pushMirrorURL, user.LowerName, userPassword, "0")

	mirrors, _, err := repo_model.GetPushMirrorsByRepoID(t.Context(), srcRepo.ID, db.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, mirrors, 1)

	ok := mirror_service.SyncPushMirror(t.Context(), mirrors[0].ID, "")
	assert.True(t, ok)
}

func testCreatePushMirror(t *testing.T, session *TestSession, owner, repo, address, username, password, interval string) {
	req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", url.PathEscape(owner), url.PathEscape(repo)), map[string]string{
		"action":               "push-mirror-add",
		"push_mirror_address":  address,
		"push_mirror_username": username,
		"push_mirror_password": password,
		"push_mirror_interval": interval,
	})
	session.MakeRequest(t, req, http.StatusSeeOther)

	flashMsg := session.GetCookieFlashMessage()
	assert.NotEmpty(t, flashMsg.SuccessMsg)
}

func doRemovePushMirror(t *testing.T, session *TestSession, owner, repo string, pushMirrorID int64) bool {
	req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", url.PathEscape(owner), url.PathEscape(repo)), map[string]string{
		"action":         "push-mirror-remove",
		"push_mirror_id": strconv.FormatInt(pushMirrorID, 10),
	})
	resp := session.MakeRequest(t, req, NoExpectedStatus)
	flashMsg := session.GetCookieFlashMessage()
	return resp.Code == http.StatusSeeOther && assert.NotEmpty(t, flashMsg.SuccessMsg)
}

func doUpdatePushMirror(t *testing.T, session *TestSession, owner, repo string, pushMirrorID int64, interval string) bool {
	req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", owner, repo), map[string]string{
		"action":                 "push-mirror-update",
		"push_mirror_id":         strconv.FormatInt(pushMirrorID, 10),
		"push_mirror_interval":   interval,
		"push_mirror_defer_sync": "true",
	})
	resp := session.MakeRequest(t, req, NoExpectedStatus)
	return resp.Code == http.StatusSeeOther
}

func TestRepoSettingPushMirrorUpdate(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	setting.Migrations.AllowLocalNetworks = true
	assert.NoError(t, migrations.Init())

	session := loginUser(t, "user2")
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	testCreatePushMirror(t, session, "user2", "repo2", "https://127.0.0.1/user1/repo1.git", "", "", "24h")

	pushMirrors, cnt, err := repo_model.GetPushMirrorsByRepoID(t.Context(), repo2.ID, db.ListOptions{})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)
	assert.Equal(t, 24*time.Hour, pushMirrors[0].Interval)
	repo2PushMirrorID := pushMirrors[0].ID

	// update repo2 push mirror
	assert.True(t, doUpdatePushMirror(t, session, "user2", "repo2", repo2PushMirrorID, "10m0s"))
	pushMirror := unittest.AssertExistsAndLoadBean(t, &repo_model.PushMirror{ID: repo2PushMirrorID})
	assert.Equal(t, 10*time.Minute, pushMirror.Interval)

	req := NewRequestWithValues(t, "POST", "/user2/repo2/settings", map[string]string{
		"action":                                  "push-mirror-update",
		"push_mirror_id":                          strconv.FormatInt(repo2PushMirrorID, 10),
		"push_mirror_interval":                    "10m0s",
		"push_mirror_sync_on_commit":              "on",
		"push_mirror_mirror_branches":             "master",
		"push_mirror_deploy_stamp_enabled":        "on",
		"push_mirror_deploy_stamp_branches":       "master",
		"push_mirror_deploy_stamp_author_name":    "Deploy Bot",
		"push_mirror_deploy_stamp_author_email":   "deploy@example.com",
		"push_mirror_deploy_stamp_commit_message": "chore: deploy stamp",
		"push_mirror_record_file_enabled":         "on",
		"push_mirror_record_file_branches":        "master",
		"push_mirror_record_file_path":            ".mirror-record",
		"push_mirror_record_file_template":        "{{.Repo}} {{.Branch}}",
		"push_mirror_record_file_author_name":     "Mirror Bot",
		"push_mirror_record_file_author_email":    "mirror@example.com",
		"push_mirror_record_file_commit_message":  "chore: update mirror record",
		"push_mirror_defer_sync":                  "true",
	})
	session.MakeRequest(t, req, http.StatusSeeOther)
	pushMirror = unittest.AssertExistsAndLoadBean(t, &repo_model.PushMirror{ID: repo2PushMirrorID})
	assert.True(t, pushMirror.SyncOnCommit)
	assert.Equal(t, "master", pushMirror.MirrorBranches)
	assert.True(t, pushMirror.DeployStampEnabled)
	assert.Equal(t, "master", pushMirror.DeployStampBranches)
	assert.Equal(t, "Deploy Bot", pushMirror.DeployStampAuthorName)
	assert.Equal(t, "deploy@example.com", pushMirror.DeployStampAuthorEmail)
	assert.Equal(t, "chore: deploy stamp", pushMirror.DeployStampCommitMessage)
	assert.True(t, pushMirror.RecordFileEnabled)
	assert.Equal(t, "master", pushMirror.RecordFileBranches)
	assert.Equal(t, ".mirror-record", pushMirror.RecordFilePath)
	assert.Equal(t, "{{.Repo}} {{.Branch}}", pushMirror.RecordFileTemplate)
	assert.Equal(t, "Mirror Bot", pushMirror.RecordFileAuthorName)
	assert.Equal(t, "mirror@example.com", pushMirror.RecordFileAuthorEmail)
	assert.Equal(t, "chore: update mirror record", pushMirror.RecordFileCommitMessage)

	// avoid updating repo2 push mirror from repo1
	assert.False(t, doUpdatePushMirror(t, session, "user2", "repo1", repo2PushMirrorID, "20m0s"))
	pushMirror = unittest.AssertExistsAndLoadBean(t, &repo_model.PushMirror{ID: repo2PushMirrorID})
	assert.Equal(t, 10*time.Minute, pushMirror.Interval) // not changed

	// avoid deleting repo2 push mirror from repo1
	assert.False(t, doRemovePushMirror(t, session, "user2", "repo1", repo2PushMirrorID))
	unittest.AssertExistsAndLoadBean(t, &repo_model.PushMirror{ID: repo2PushMirrorID})

	// delete repo2 push mirror
	assert.True(t, doRemovePushMirror(t, session, "user2", "repo2", repo2PushMirrorID))
	unittest.AssertNotExistsBean(t, &repo_model.PushMirror{ID: repo2PushMirrorID})
}

func TestRepoSettingPushMirrorSyncLogs(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	setting.Migrations.AllowLocalNetworks = true
	assert.NoError(t, migrations.Init())

	assert.NoError(t, db.TruncateBeans(t.Context(), &repo_model.PushMirror{}, &repo_model.MirrorSyncTask{}))

	session := loginUser(t, "user2")
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	testCreatePushMirror(t, session, "user2", "repo2", "https://127.0.0.1/user1/repo1.git", "", "", "24h")

	pushMirrors, cnt, err := repo_model.GetPushMirrorsByRepoID(t.Context(), repo2.ID, db.ListOptions{})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cnt)

	task := &repo_model.MirrorSyncTask{
		RepoID:       repo2.ID,
		MirrorType:   repo_model.MirrorSyncTypePush,
		PushMirrorID: pushMirrors[0].ID,
		TriggerType:  repo_model.MirrorSyncTriggerManual,
	}
	assert.NoError(t, repo_model.InsertMirrorSyncTask(t.Context(), task))
	task.IsSucceed = true
	task.Stdout = "mirror stdout line"
	task.Stderr = "mirror stderr line"
	task.FinishedUnix = timeutil.TimeStampNow()
	assert.NoError(t, repo_model.UpdateMirrorSyncTaskCompleted(t.Context(), task))

	req := NewRequest(t, "GET", "/user2/repo2/settings/mirror")
	resp := session.MakeRequest(t, req, http.StatusOK)

	body := resp.Body.String()
	assert.Contains(t, body, "Push logs")
	assert.Contains(t, body, "Manual")
	assert.Contains(t, body, "Success")
	assert.Contains(t, body, "mirror stdout line")
	assert.Contains(t, body, "mirror stderr line")
	assert.Contains(t, body, task.UUID)
}

func TestRepoSettingPushMirrorDiffPreview(t *testing.T) {
	onGiteaRun(t, testRepoSettingPushMirrorDiffPreview)
}

func testRepoSettingPushMirrorDiffPreview(t *testing.T, u *url.URL) {
	setting.Migrations.AllowLocalNetworks = true
	assert.NoError(t, migrations.Init())

	_ = db.TruncateBeans(t.Context(), &repo_model.PushMirror{}, &repo_model.MirrorSyncTask{})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	srcRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	mirrorRepo, err := repo_service.CreateRepositoryDirectly(t.Context(), user, user, repo_service.CreateRepoOptions{
		Name: "test-push-mirror-diff-preview",
	}, true)
	assert.NoError(t, err)

	session := loginUser(t, user.Name)
	pushMirrorURL := fmt.Sprintf("%s%s/%s", u.String(), url.PathEscape(user.Name), url.PathEscape(mirrorRepo.Name))
	req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", url.PathEscape(user.Name), url.PathEscape(srcRepo.Name)), map[string]string{
		"action":                      "push-mirror-add",
		"push_mirror_address":         pushMirrorURL,
		"push_mirror_username":        user.LowerName,
		"push_mirror_password":        userPassword,
		"push_mirror_interval":        "0",
		"push_mirror_mirror_branches": "master",
	})
	session.MakeRequest(t, req, http.StatusSeeOther)

	mirrors, _, err := repo_model.GetPushMirrorsByRepoID(t.Context(), srcRepo.ID, db.ListOptions{})
	assert.NoError(t, err)
	require.Len(t, mirrors, 1)

	assert.True(t, mirror_service.SyncPushMirror(t.Context(), mirrors[0].ID, ""))
	createEmptyCommitOnBranch(t, srcRepo, "master", "push mirror diff preview")

	req = NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings/mirror", url.PathEscape(user.Name), url.PathEscape(srcRepo.Name)), map[string]string{
		"action":         "push-mirror-check-diff",
		"push_mirror_id": strconv.FormatInt(mirrors[0].ID, 10),
	})
	resp := session.MakeRequest(t, req, http.StatusOK)
	body := resp.Body.String()

	assert.Contains(t, body, "Push mirror differences")
	assert.Contains(t, body, "Branch: master")
	assert.Contains(t, body, "Local ahead: 1")
	assert.Contains(t, body, "Remote ahead: 0")
	assert.Contains(t, body, "Commits to push")
	assert.Contains(t, body, "push mirror diff preview")
}

func createEmptyCommitOnBranch(t *testing.T, repo *repo_model.Repository, branch, message string) {
	t.Helper()

	repoPath := filepath.Join(setting.RepoRootPath, filepath.FromSlash(repo.RelativePath()))
	parent, _, err := gitcmd.NewCommand("rev-parse").AddDynamicArguments(git.BranchPrefix + branch).WithDir(repoPath).RunStdString(t.Context())
	require.NoError(t, err)
	parent = strings.TrimSpace(parent)
	tree, _, err := gitcmd.NewCommand("rev-parse").AddDynamicArguments(git.BranchPrefix + branch + "^{tree}").WithDir(repoPath).RunStdString(t.Context())
	require.NoError(t, err)
	tree = strings.TrimSpace(tree)

	env := []string{
		"GIT_AUTHOR_NAME=Mirror Test",
		"GIT_AUTHOR_EMAIL=mirror-test@example.com",
		"GIT_COMMITTER_NAME=Mirror Test",
		"GIT_COMMITTER_EMAIL=mirror-test@example.com",
	}
	commitID, _, err := gitcmd.NewCommand("commit-tree").
		AddDynamicArguments(tree).
		AddArguments("-p").
		AddDynamicArguments(parent).
		AddArguments("-m").
		AddDynamicArguments(message).
		WithEnv(env).
		WithDir(repoPath).
		RunStdString(t.Context())
	require.NoError(t, err)

	_, _, err = gitcmd.NewCommand("update-ref").
		AddDynamicArguments(git.BranchPrefix+branch, strings.TrimSpace(commitID)).
		WithDir(repoPath).
		RunStdString(t.Context())
	require.NoError(t, err)
}

func TestPushMirrorBranchRestriction(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	onGiteaRun(t, testPushMirrorBranchRestriction)
}

func testPushMirrorBranchRestriction(t *testing.T, u *url.URL) {
	setting.Migrations.AllowLocalNetworks = true
	assert.NoError(t, migrations.Init())

	_ = db.TruncateBeans(t.Context(), &repo_model.PushMirror{})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	srcRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	mirrorRepo, err := repo_service.CreateRepositoryDirectly(t.Context(), user, user, repo_service.CreateRepoOptions{
		Name: "test-push-mirror-branches",
	}, true)
	assert.NoError(t, err)

	session := loginUser(t, user.Name)
	pushMirrorURL := fmt.Sprintf("%s%s/%s", u.String(), url.PathEscape(user.Name), url.PathEscape(mirrorRepo.Name))
	req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", url.PathEscape(user.Name), url.PathEscape(srcRepo.Name)), map[string]string{
		"action":                      "push-mirror-add",
		"push_mirror_address":         pushMirrorURL,
		"push_mirror_username":        user.LowerName,
		"push_mirror_password":        userPassword,
		"push_mirror_interval":        "0",
		"push_mirror_mirror_branches": "master",
	})
	session.MakeRequest(t, req, http.StatusSeeOther)
	flashMsg := session.GetCookieFlashMessage()
	assert.NotEmpty(t, flashMsg.SuccessMsg)

	mirrors, _, err := repo_model.GetPushMirrorsByRepoID(t.Context(), srcRepo.ID, db.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, mirrors, 1)
	pm := mirrors[0]

	repoPath := filepath.Join(setting.RepoRootPath, filepath.FromSlash(srcRepo.RelativePath()))
	stdout, _, runErr := gitcmd.NewCommand("config", "--get-all").AddDynamicArguments("remote." + pm.RemoteName + ".push").WithDir(repoPath).RunStdString(t.Context())
	assert.NoError(t, runErr)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.Len(t, lines, 1)
	assert.Contains(t, lines[0], "refs/heads/master")
	assert.NotContains(t, lines[0], "refs/tags")

	assert.True(t, doRemovePushMirror(t, session, user.Name, srcRepo.Name, pm.ID))
}
