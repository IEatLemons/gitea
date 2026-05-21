// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package mirror

import (
	"context"
	"strings"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/log"
	repo_module "code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/setting"
)

// DefaultDeployStampCommitMessage is used when DeployStampCommitMessage is empty in settings.
const DefaultDeployStampCommitMessage = "chore: deploy stamp"

// MaybeDeployStampOnPush adds an empty commit on top of a branch when a push mirror
// has "deploy stamp" enabled, so downstream systems see a trusted commit author.
// External CD (e.g. Vercel, Railway) may filter by commit author; deploy platforms may
// also validate the GitHub account used to push — use mirror credentials that meet that policy.
func MaybeDeployStampOnPush(ctx context.Context, repo *repo_model.Repository, opts *repo_module.PushUpdateOptions) error {
	if opts == nil || !opts.RefFullName.IsBranch() || opts.IsDelRef() {
		return nil
	}
	if !setting.Mirror.Enabled {
		return nil
	}

	branch := opts.RefFullName.BranchName()
	pushMirrors, err := repo_model.GetPushMirrorsSyncedOnCommit(ctx, repo.ID)
	if err != nil {
		return err
	}

	stampMirror := selectDeployStampMirror(pushMirrors, branch)
	if stampMirror == nil {
		return nil
	}

	gitRepo, err := gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		return err
	}
	defer gitRepo.Close()

	headID, err := gitRepo.GetBranchCommitID(branch)
	if err != nil {
		return err
	}
	if headID != opts.NewCommitID {
		log.Trace("DeployStamp: skip %s/%s head=%s != pushed %s (concurrent update)", repo.FullName(), branch, headID, opts.NewCommitID)
		return nil
	}

	headCommit, err := gitRepo.GetCommit(headID)
	if err != nil {
		return err
	}
	if strings.EqualFold(headCommit.Author.Email, stampMirror.DeployStampAuthorEmail) {
		return nil
	}

	msg := NormalizeDeployStampCommitMessage(stampMirror.DeployStampCommitMessage)

	author := &git.Signature{
		Name:  stampMirror.DeployStampAuthorName,
		Email: stampMirror.DeployStampAuthorEmail,
	}
	committer := author

	tree, err := gitRepo.GetTree(headCommit.ID.String() + "^{tree}")
	if err != nil {
		return err
	}

	newID, err := gitRepo.CommitTree(author, committer, tree, git.CommitTreeOpts{
		Parents:   []string{headCommit.ID.String()},
		Message:   msg,
		NoGPGSign: true,
	})
	if err != nil {
		return err
	}

	refName := opts.RefFullName.String()
	if err := gitrepo.UpdateRef(ctx, repo, refName, newID.String()); err != nil {
		return err
	}

	if _, err := repo_module.SyncRepoBranches(ctx, repo.ID, 0); err != nil {
		log.Error("DeployStamp: SyncRepoBranches[%s]: %v", repo.FullName(), err)
	}
	if err := repo_module.UpdateRepoSize(ctx, repo); err != nil {
		log.Error("DeployStamp: UpdateRepoSize[%s]: %v", repo.FullName(), err)
	}
	return nil
}

func selectDeployStampMirror(mirrors []*repo_model.PushMirror, branch string) *repo_model.PushMirror {
	for _, m := range mirrors {
		if m == nil || !m.DeployStampEnabled {
			continue
		}
		if strings.TrimSpace(m.DeployStampAuthorEmail) == "" {
			log.Trace("DeployStamp: mirror %d has empty author email, skip", m.ID)
			continue
		}
		if !branchMatchesDeployStampList(branch, m.DeployStampBranches) {
			continue
		}
		return m
	}
	return nil
}

func branchMatchesDeployStampList(branch, branchList string) bool {
	for _, b := range parseDeployStampBranchList(branchList) {
		if b == branch {
			return true
		}
	}
	return false
}

func parseDeployStampBranchList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// NormalizeDeployStampCommitMessage returns a non-empty commit message for deploy stamp commits.
func NormalizeDeployStampCommitMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return DefaultDeployStampCommitMessage
	}
	return msg
}

// ApplyDeployStampFromForm applies deploy stamp settings from API or web forms.
func ApplyDeployStampFromForm(m *repo_model.PushMirror, enabled bool, branches, authorName, authorEmail, commitMessage string) {
	m.DeployStampEnabled = enabled
	m.DeployStampBranches = strings.TrimSpace(branches)
	m.DeployStampAuthorName = strings.TrimSpace(authorName)
	m.DeployStampAuthorEmail = strings.TrimSpace(authorEmail)
	msg := strings.TrimSpace(commitMessage)
	m.DeployStampCommitMessage = NormalizeDeployStampCommitMessage(msg)
}
