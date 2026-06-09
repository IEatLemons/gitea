// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package mirror

import (
	"context"
	"fmt"
	"strings"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/gitdiff"
)

const (
	defaultPushMirrorDiffCommitLimit = 20
	defaultPushMirrorDiffFileLimit   = 50
)

// PushMirrorDiffPreview is a read-only summary of local branches compared with
// their configured push mirror remote branches.
type PushMirrorDiffPreview struct {
	PushMirrorID      int64
	Branches          []*PushMirrorBranchDiffPreview
	DefaultBranchOnly bool
	Error             string
}

type PushMirrorBranchDiffPreview struct {
	Branch         string
	LocalCommitID  string
	RemoteCommitID string
	Ahead          int
	Behind         int
	NumFiles       int
	TotalAdditions int
	TotalDeletions int
	Commits        []*PushMirrorDiffCommit
	Files          []*PushMirrorDiffFile
	CommitsMore    int
	FilesMore      int
	Error          string
}

type PushMirrorDiffCommit struct {
	ID      string
	Summary string
	Author  string
}

type PushMirrorDiffFile struct {
	Path    string
	OldPath string
	Status  string
}

// GetPushMirrorDiffPreview fetches the configured mirror branches into
// temporary refs and compares them with the local branches.
func GetPushMirrorDiffPreview(ctx context.Context, m *repo_model.PushMirror) *PushMirrorDiffPreview {
	preview := &PushMirrorDiffPreview{PushMirrorID: m.ID}
	if m.Repo == nil {
		m.GetRepository(ctx)
	}
	if m.Repo == nil {
		preview.Error = "repository not found"
		return preview
	}

	branches, err := ParseMirrorBranches(m.MirrorBranches)
	if err != nil {
		preview.Error = err.Error()
		return preview
	}
	if len(branches) == 0 {
		if strings.TrimSpace(m.Repo.DefaultBranch) == "" {
			preview.Error = "default branch is empty"
			return preview
		}
		branches = []string{m.Repo.DefaultBranch}
		preview.DefaultBranchOnly = true
	}

	remoteURL, err := gitrepo.GitRemoteGetURL(ctx, m.Repo, m.RemoteName)
	if err != nil {
		preview.Error = sanitizePushMirrorDiffError(err)
		return preview
	}
	envs, envCleanup, err := BuildPushMirrorGitEnvs(remoteURL, m)
	if err != nil {
		preview.Error = sanitizePushMirrorDiffError(err)
		return preview
	}
	defer envCleanup()

	gitRepo, err := gitrepo.OpenRepository(ctx, m.Repo)
	if err != nil {
		preview.Error = sanitizePushMirrorDiffError(err)
		return preview
	}
	defer gitRepo.Close()

	for _, branch := range branches {
		preview.Branches = append(preview.Branches, getPushMirrorBranchDiffPreview(ctx, m, gitRepo, envs, branch))
	}
	return preview
}

func getPushMirrorBranchDiffPreview(ctx context.Context, m *repo_model.PushMirror, gitRepo *git.Repository, envs []string, branch string) *PushMirrorBranchDiffPreview {
	result := &PushMirrorBranchDiffPreview{Branch: branch}
	localRef := git.BranchPrefix + branch
	localCommitID, err := gitRepo.GetRefCommitID(localRef)
	if err != nil {
		if git.IsErrNotExist(err) {
			result.Error = fmt.Sprintf("Local branch %q does not exist.", branch)
			return result
		}
		result.Error = sanitizePushMirrorDiffError(err)
		return result
	}
	result.LocalCommitID = localCommitID

	tempRef := fmt.Sprintf("refs/push-mirror-preview/%d/%d/%s", m.ID, time.Now().UnixNano(), branch)
	defer deletePushMirrorPreviewRef(ctx, m.Repo, tempRef)

	if err := fetchPushMirrorBranch(ctx, m, envs, branch, tempRef); err != nil {
		result.Error = sanitizePushMirrorDiffError(err)
		return result
	}

	remoteCommitID, err := gitRepo.GetRefCommitID(tempRef)
	if err != nil {
		result.Error = sanitizePushMirrorDiffError(err)
		return result
	}
	result.RemoteCommitID = remoteCommitID

	diverge, err := gitrepo.GetDivergingCommits(ctx, m.Repo, tempRef, localRef)
	if err != nil {
		result.Error = sanitizePushMirrorDiffError(err)
		return result
	}
	result.Ahead = diverge.Ahead
	result.Behind = diverge.Behind

	if err := fillPushMirrorDiffCommits(ctx, m.Repo, gitRepo, result, tempRef, localRef); err != nil {
		result.Error = sanitizePushMirrorDiffError(err)
		return result
	}
	if err := fillPushMirrorDiffFiles(ctx, m.Repo, gitRepo, result, tempRef, localRef); err != nil {
		result.Error = sanitizePushMirrorDiffError(err)
		return result
	}
	return result
}

func fetchPushMirrorBranch(ctx context.Context, m *repo_model.PushMirror, envs []string, branch, tempRef string) error {
	refspec := fmt.Sprintf("+refs/heads/%s:%s", branch, tempRef)
	cmd := gitcmd.NewCommand("fetch", "--no-tags").
		AddDynamicArguments(m.RemoteName, refspec).
		WithEnv(envs).
		WithTimeout(time.Duration(setting.Git.Timeout.Mirror) * time.Second)
	_, _, err := gitrepo.RunCmdString(ctx, m.Repo, cmd)
	if err != nil {
		if isRemoteBranchMissing(err) {
			return fmt.Errorf("Remote branch %q does not exist.", branch)
		}
		return err
	}
	return nil
}

func fillPushMirrorDiffCommits(ctx context.Context, repo *repo_model.Repository, gitRepo *git.Repository, result *PushMirrorBranchDiffPreview, remoteRef, localRef string) error {
	if result.Ahead == 0 {
		return nil
	}

	commitIDs, err := gitrepo.GetCommitIDsBetweenReverse(ctx, repo, remoteRef, localRef, "", defaultPushMirrorDiffCommitLimit)
	if err != nil {
		return err
	}
	for _, commitID := range commitIDs {
		commit, err := gitRepo.GetCommit(commitID)
		if err != nil {
			return err
		}
		author := ""
		if commit.Author != nil {
			author = commit.Author.Name
		}
		result.Commits = append(result.Commits, &PushMirrorDiffCommit{
			ID:      commit.ID.String(),
			Summary: commit.Summary(),
			Author:  author,
		})
	}
	if result.Ahead > len(result.Commits) {
		result.CommitsMore = result.Ahead - len(result.Commits)
	}
	return nil
}

func fillPushMirrorDiffFiles(ctx context.Context, repo *repo_model.Repository, gitRepo *git.Repository, result *PushMirrorBranchDiffPreview, remoteRef, localRef string) error {
	numFiles, additions, deletions, err := gitrepo.GetDiffShortStatByCmdArgs(ctx, repo, nil, remoteRef, localRef)
	if err != nil {
		return err
	}
	result.NumFiles = numFiles
	result.TotalAdditions = additions
	result.TotalDeletions = deletions

	diffTree, err := gitdiff.GetDiffTree(ctx, gitRepo, false, remoteRef, localRef)
	if err != nil {
		return err
	}
	limit := len(diffTree.Files)
	if limit > defaultPushMirrorDiffFileLimit {
		limit = defaultPushMirrorDiffFileLimit
		result.FilesMore = len(diffTree.Files) - defaultPushMirrorDiffFileLimit
	}
	for _, file := range diffTree.Files[:limit] {
		result.Files = append(result.Files, &PushMirrorDiffFile{
			Path:    file.HeadPath,
			OldPath: file.BasePath,
			Status:  file.Status,
		})
	}
	return nil
}

func deletePushMirrorPreviewRef(ctx context.Context, repo *repo_model.Repository, ref string) {
	_ = gitrepo.RunCmd(ctx, repo, gitcmd.NewCommand("update-ref", "-d").AddDynamicArguments(ref))
}

func isRemoteBranchMissing(err error) bool {
	stderr, ok := gitcmd.ErrorAsStderr(err)
	return ok && strings.Contains(stderr, "couldn't find remote ref")
}

func sanitizePushMirrorDiffError(err error) string {
	if err == nil {
		return ""
	}
	return stripExitStatus.ReplaceAllLiteralString(util.SanitizeCredentialURLs(err.Error()), "")
}
