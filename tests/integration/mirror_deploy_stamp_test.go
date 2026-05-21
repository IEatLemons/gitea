// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/url"
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/services/migrations"
	mirror_service "code.gitea.io/gitea/services/mirror"
	repo_service "code.gitea.io/gitea/services/repository"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushMirrorDeployStamp(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	onGiteaRun(t, testPushMirrorDeployStamp)
}

func testPushMirrorDeployStamp(t *testing.T, u *url.URL) {
	setting.Migrations.AllowLocalNetworks = true
	require.NoError(t, migrations.Init())

	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	srcRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	mirrorRepo, err := repo_service.CreateRepositoryDirectly(t.Context(), user, user, repo_service.CreateRepoOptions{
		Name: "deploy-stamp-mirror-dest",
	}, true)
	require.NoError(t, err)

	session := loginUser(t, user.Name)
	pushMirrorURL := fmt.Sprintf("%s%s/%s", u.String(), url.PathEscape(user.Name), url.PathEscape(mirrorRepo.Name))
	testCreatePushMirror(t, session, user.Name, srcRepo.Name, pushMirrorURL, user.LowerName, userPassword, "0")

	mirrors, _, err := repo_model.GetPushMirrorsByRepoID(t.Context(), srcRepo.ID, db.ListOptions{})
	require.NoError(t, err)
	require.Len(t, mirrors, 1)
	pm := mirrors[0]

	pm.DeployStampEnabled = true
	pm.DeployStampBranches = "master"
	pm.DeployStampAuthorName = "Trusted Bot"
	pm.DeployStampAuthorEmail = "trusted@example.com"
	pm.DeployStampCommitMessage = "chore: deploy stamp"
	require.NoError(t, repo_model.UpdatePushMirror(t.Context(), pm))

	gitRepo, err := gitrepo.OpenRepository(t.Context(), srcRepo)
	require.NoError(t, err)
	defer gitRepo.Close()

	oldHead, err := gitRepo.GetBranchCommitID("master")
	require.NoError(t, err)
	_, err = gitRepo.GetCommit(oldHead)
	require.NoError(t, err)

	opts := &repository.PushUpdateOptions{
		RefFullName: git.RefNameFromBranch("master"),
		OldCommitID: "",
		NewCommitID: oldHead,
	}
	require.NoError(t, mirror_service.MaybeDeployStampOnPush(t.Context(), srcRepo, opts))

	newHead, err := gitRepo.GetBranchCommitID("master")
	require.NoError(t, err)
	require.NotEqual(t, oldHead, newHead)
	newCommit, err := gitRepo.GetCommit(newHead)
	require.NoError(t, err)
	assert.Equal(t, "Trusted Bot", newCommit.Author.Name)
	assert.Equal(t, "trusted@example.com", newCommit.Author.Email)
	parent0, err := newCommit.ParentID(0)
	require.NoError(t, err)
	assert.Equal(t, oldHead, parent0.String())

	optsAfter := &repository.PushUpdateOptions{
		RefFullName: git.RefNameFromBranch("master"),
		OldCommitID: oldHead,
		NewCommitID: newHead,
	}
	require.NoError(t, mirror_service.MaybeDeployStampOnPush(t.Context(), srcRepo, optsAfter))

	stableHead, err := gitRepo.GetBranchCommitID("master")
	require.NoError(t, err)
	assert.Equal(t, newHead, stableHead)

	assert.True(t, doRemovePushMirror(t, session, user.Name, srcRepo.Name, pm.ID))
}
