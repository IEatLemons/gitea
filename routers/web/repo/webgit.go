// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"strings"

	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context"
	files_service "code.gitea.io/gitea/services/repository/files"
)

func WebGitOperationCommonData(ctx *context.Context) {
	// TODO: more places like "wiki page" and "merging a pull request or creating an auto merge merging task"
	emails, err := user_model.GetActivatedEmailAddresses(ctx, ctx.Doer.ID)
	if err != nil {
		log.Error("WebGitOperationCommonData: GetActivatedEmailAddresses: %v", err)
	}
	if ctx.Doer.KeepEmailPrivate {
		emails = append([]string{ctx.Doer.GetPlaceholderEmail()}, emails...)
	}
	ctx.Data["CommitCandidateEmails"] = emails
	ctx.Data["CommitDefaultEmail"] = ctx.Doer.GetEmail()
}

func WebGitOperationGetCommitChosenEmailIdentity(ctx *context.Context, email string) (_ *files_service.IdentityOptions, valid bool) {
	if ctx.Data["CommitCandidateEmails"] == nil {
		setting.PanicInDevOrTesting("no CommitCandidateEmails in context data")
	}
	emails, _ := ctx.Data["CommitCandidateEmails"].([]string)
	if email == "" {
		return nil, true
	}
	if util.SliceContainsString(emails, email, true) {
		return &files_service.IdentityOptions{GitUserEmail: email}, true
	}
	return nil, false
}

// WebGitOperationGetCommitIdentity returns the Git identity selected for a web commit.
func WebGitOperationGetCommitIdentity(ctx *context.Context, email string) (_ *files_service.IdentityOptions, valid bool) {
	identity, valid := WebGitOperationGetCommitChosenEmailIdentity(ctx, email)
	if !valid {
		return nil, false
	}

	repo := ctx.Repo.Repository
	if repo == nil || (strings.TrimSpace(repo.MergeCommitterName) == "" && strings.TrimSpace(repo.MergeCommitterEmail) == "") {
		return identity, true
	}

	if identity == nil {
		identity = &files_service.IdentityOptions{}
	}
	if name := strings.TrimSpace(repo.MergeCommitterName); name != "" {
		identity.GitUserName = name
	}
	if email := strings.TrimSpace(repo.MergeCommitterEmail); email != "" {
		identity.GitUserEmail = email
	}
	return identity, true
}
