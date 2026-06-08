// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package convert

import (
	"context"

	repo_model "code.gitea.io/gitea/models/repo"
	api "code.gitea.io/gitea/modules/structs"
)

// ToPushMirror convert from repo_model.PushMirror and remoteAddress to api.TopicResponse
func ToPushMirror(ctx context.Context, pm *repo_model.PushMirror) (*api.PushMirror, error) {
	repo := pm.GetRepository(ctx)
	return &api.PushMirror{
		RepoName:                 repo.Name,
		RemoteName:               pm.RemoteName,
		RemoteAddress:            pm.RemoteAddress,
		CreatedUnix:              pm.CreatedUnix.AsTime(),
		LastUpdateUnix:           pm.LastUpdateUnix.AsTimePtr(),
		LastError:                pm.LastError,
		Interval:                 pm.Interval.String(),
		SyncOnCommit:             pm.SyncOnCommit,
		DeployStampEnabled:       pm.DeployStampEnabled,
		DeployStampBranches:      pm.DeployStampBranches,
		DeployStampAuthorName:    pm.DeployStampAuthorName,
		DeployStampAuthorEmail:   pm.DeployStampAuthorEmail,
		DeployStampCommitMessage: pm.DeployStampCommitMessage,
		RecordFileEnabled:        pm.RecordFileEnabled,
		RecordFileBranches:       pm.RecordFileBranches,
		RecordFilePath:           pm.RecordFilePath,
		RecordFileTemplate:       pm.RecordFileTemplate,
		RecordFileAuthorName:     pm.RecordFileAuthorName,
		RecordFileAuthorEmail:    pm.RecordFileAuthorEmail,
		RecordFileCommitMessage:  pm.RecordFileCommitMessage,
		MirrorBranches:           pm.MirrorBranches,
		AuthType:                 pm.AuthType,
		SSHHostKeyPolicy:         pm.SSHHostKeyPolicy,
	}, nil
}
