// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"strings"

	"code.gitea.io/gitea/models/db"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/timeutil"
)

// RepoUserCommitIdentity stores a user's web commit identity for a repository.
type RepoUserCommitIdentity struct {
	ID          int64  `xorm:"pk autoincr"`
	RepoID      int64  `xorm:"UNIQUE(repo_user) INDEX NOT NULL"`
	UserID      int64  `xorm:"UNIQUE(repo_user) INDEX NOT NULL"`
	CommitName  string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
	CommitEmail string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`

	CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
	UpdatedUnix timeutil.TimeStamp `xorm:"INDEX updated"`
}

func init() {
	db.RegisterModel(new(RepoUserCommitIdentity))
}

// GetUserCommitIdentity returns the user's web commit identity for a repository.
func GetUserCommitIdentity(ctx context.Context, repoID, userID int64) (*RepoUserCommitIdentity, bool, error) {
	identity := &RepoUserCommitIdentity{RepoID: repoID, UserID: userID}
	has, err := db.GetEngine(ctx).Get(identity)
	if err != nil {
		return nil, false, err
	}
	return identity, has, nil
}

// SetUserCommitIdentity sets the user's web commit identity for a repository.
func SetUserCommitIdentity(ctx context.Context, repoID, userID int64, name, email string) error {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)

	identity, has, err := GetUserCommitIdentity(ctx, repoID, userID)
	if err != nil {
		return err
	}
	if name == "" && email == "" {
		if !has {
			return nil
		}
		_, err := db.GetEngine(ctx).ID(identity.ID).Delete(new(RepoUserCommitIdentity))
		return err
	}

	if has {
		identity.CommitName = name
		identity.CommitEmail = email
		_, err := db.GetEngine(ctx).ID(identity.ID).Cols("commit_name", "commit_email").Update(identity)
		return err
	}

	return db.Insert(ctx, &RepoUserCommitIdentity{
		RepoID:      repoID,
		UserID:      userID,
		CommitName:  name,
		CommitEmail: email,
	})
}

// NewUserCommitterSig returns the Git identity used for this user's repository web commits.
func NewUserCommitterSig(ctx context.Context, repo *Repository, doer *user_model.User) (*git.Signature, error) {
	sig := doer.NewGitSig()
	if repo == nil {
		return sig, nil
	}

	identity, has, err := GetUserCommitIdentity(ctx, repo.ID, doer.ID)
	if err != nil {
		return nil, err
	}
	if !has {
		return sig, nil
	}
	if name := strings.TrimSpace(identity.CommitName); name != "" {
		sig.Name = name
	}
	if email := strings.TrimSpace(identity.CommitEmail); email != "" {
		sig.Email = email
	}
	return sig, nil
}
