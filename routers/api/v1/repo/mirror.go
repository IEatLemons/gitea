// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	"code.gitea.io/gitea/modules/git"
	giturl "code.gitea.io/gitea/modules/git/url"
	"code.gitea.io/gitea/modules/setting"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/modules/web"
	"code.gitea.io/gitea/routers/api/v1/utils"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/convert"
	"code.gitea.io/gitea/services/migrations"
	mirror_service "code.gitea.io/gitea/services/mirror"
)

// MirrorSync adds a mirrored repository to the sync queue
func MirrorSync(ctx *context.APIContext) {
	// swagger:operation POST /repos/{owner}/{repo}/mirror-sync repository repoMirrorSync
	// ---
	// summary: Sync a mirrored repository
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo to sync
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo to sync
	//   type: string
	//   required: true
	// responses:
	//   "200":
	//     "$ref": "#/responses/empty"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	repo := ctx.Repo.Repository

	if !ctx.Repo.CanWrite(unit.TypeCode) {
		ctx.APIError(http.StatusForbidden, "Must have write access")
	}

	if !setting.Mirror.Enabled {
		ctx.APIError(http.StatusBadRequest, "Mirror feature is disabled")
		return
	}

	if _, err := repo_model.GetMirrorByRepoID(ctx, repo.ID); err != nil {
		if errors.Is(err, repo_model.ErrMirrorNotExist) {
			ctx.APIError(http.StatusBadRequest, "Repository is not a mirror")
			return
		}
		ctx.APIErrorInternal(err)
		return
	}

	mirror_service.AddPullMirrorToQueue(repo.ID, repo_model.MirrorSyncTriggerManual)

	ctx.Status(http.StatusOK)
}

// PushMirrorSync adds all push mirrored repositories to the sync queue
func PushMirrorSync(ctx *context.APIContext) {
	// swagger:operation POST /repos/{owner}/{repo}/push_mirrors-sync repository repoPushMirrorSync
	// ---
	// summary: Sync all push mirrored repository
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo to sync
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo to sync
	//   type: string
	//   required: true
	// responses:
	//   "200":
	//     "$ref": "#/responses/empty"
	//   "400":
	//     "$ref": "#/responses/error"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	if !setting.Mirror.Enabled {
		ctx.APIError(http.StatusBadRequest, "Mirror feature is disabled")
		return
	}
	// Get All push mirrors of a specific repo
	pushMirrors, _, err := repo_model.GetPushMirrorsByRepoID(ctx, ctx.Repo.Repository.ID, db.ListOptions{})
	if err != nil {
		ctx.APIError(http.StatusNotFound, err)
		return
	}
	for _, mirror := range pushMirrors {
		ok := mirror_service.SyncPushMirror(ctx, mirror.ID, repo_model.MirrorSyncTriggerManual)
		if !ok {
			ctx.APIErrorInternal(errors.New("error occurred when syncing push mirror " + mirror.RemoteName))
			return
		}
	}

	ctx.Status(http.StatusOK)
}

// ListPushMirrors get list of push mirrors of a repository
func ListPushMirrors(ctx *context.APIContext) {
	// swagger:operation GET /repos/{owner}/{repo}/push_mirrors repository repoListPushMirrors
	// ---
	// summary: Get all push mirrors of the repository
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// - name: page
	//   in: query
	//   description: page number of results to return (1-based)
	//   type: integer
	// - name: limit
	//   in: query
	//   description: page size of results
	//   type: integer
	// responses:
	//   "200":
	//     "$ref": "#/responses/PushMirrorList"
	//   "400":
	//     "$ref": "#/responses/error"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	if !setting.Mirror.Enabled {
		ctx.APIError(http.StatusBadRequest, "Mirror feature is disabled")
		return
	}

	repo := ctx.Repo.Repository
	// Get all push mirrors for the specified repository.
	pushMirrors, count, err := repo_model.GetPushMirrorsByRepoID(ctx, repo.ID, utils.GetListOptions(ctx))
	if err != nil {
		ctx.APIError(http.StatusNotFound, err)
		return
	}

	responsePushMirrors := make([]*api.PushMirror, 0, len(pushMirrors))
	for _, mirror := range pushMirrors {
		m, err := convert.ToPushMirror(ctx, mirror)
		if err == nil {
			responsePushMirrors = append(responsePushMirrors, m)
		}
	}
	ctx.SetLinkHeader(int64(len(responsePushMirrors)), utils.GetListOptions(ctx).PageSize)
	ctx.SetTotalCountHeader(count)
	ctx.JSON(http.StatusOK, responsePushMirrors)
}

// GetPushMirrorByName get push mirror of a repository by name
func GetPushMirrorByName(ctx *context.APIContext) {
	// swagger:operation GET /repos/{owner}/{repo}/push_mirrors/{name} repository repoGetPushMirrorByRemoteName
	// ---
	// summary: Get push mirror of the repository by remoteName
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// - name: name
	//   in: path
	//   description: remote name of push mirror
	//   type: string
	//   required: true
	// responses:
	//   "200":
	//     "$ref": "#/responses/PushMirror"
	//   "400":
	//     "$ref": "#/responses/error"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	if !setting.Mirror.Enabled {
		ctx.APIError(http.StatusBadRequest, "Mirror feature is disabled")
		return
	}

	mirrorName := ctx.PathParam("name")
	// Get push mirror of a specific repo by remoteName
	pushMirror, exist, err := db.Get[repo_model.PushMirror](ctx, repo_model.PushMirrorOptions{
		RepoID:     ctx.Repo.Repository.ID,
		RemoteName: mirrorName,
	}.ToConds())
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	} else if !exist {
		ctx.APIError(http.StatusNotFound, nil)
		return
	}

	m, err := convert.ToPushMirror(ctx, pushMirror)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	ctx.JSON(http.StatusOK, m)
}

// AddPushMirror adds a push mirror to a repository
func AddPushMirror(ctx *context.APIContext) {
	// swagger:operation POST /repos/{owner}/{repo}/push_mirrors repository repoAddPushMirror
	// ---
	// summary: add a push mirror to the repository
	// consumes:
	// - application/json
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// - name: body
	//   in: body
	//   schema:
	//     "$ref": "#/definitions/CreatePushMirrorOption"
	// responses:
	//   "200":
	//     "$ref": "#/responses/PushMirror"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "400":
	//     "$ref": "#/responses/error"
	//   "404":
	//     "$ref": "#/responses/notFound"

	if !setting.Mirror.Enabled {
		ctx.APIError(http.StatusBadRequest, "Mirror feature is disabled")
		return
	}

	pushMirror := web.GetForm(ctx).(*api.CreatePushMirrorOption)
	CreatePushMirror(ctx, pushMirror)
}

// DeletePushMirrorByRemoteName deletes a push mirror from a repository by remoteName
func DeletePushMirrorByRemoteName(ctx *context.APIContext) {
	// swagger:operation DELETE /repos/{owner}/{repo}/push_mirrors/{name} repository repoDeletePushMirror
	// ---
	// summary: deletes a push mirror from a repository by remoteName
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// - name: name
	//   in: path
	//   description: remote name of the pushMirror
	//   type: string
	//   required: true
	// responses:
	//   "204":
	//     "$ref": "#/responses/empty"
	//   "404":
	//     "$ref": "#/responses/notFound"
	//   "400":
	//     "$ref": "#/responses/error"

	if !setting.Mirror.Enabled {
		ctx.APIError(http.StatusBadRequest, "Mirror feature is disabled")
		return
	}

	remoteName := ctx.PathParam("name")
	m, exist, err := db.Get[repo_model.PushMirror](ctx, repo_model.PushMirrorOptions{
		RepoID:     ctx.Repo.Repository.ID,
		RemoteName: remoteName,
	}.ToConds())
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	} else if !exist {
		ctx.APIError(http.StatusNotFound, nil)
		return
	}
	if err := mirror_service.RemovePushMirrorRemote(ctx, m); err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	if err := repo_model.DeletePushMirrors(ctx, repo_model.PushMirrorOptions{RepoID: ctx.Repo.Repository.ID, RemoteName: remoteName}); err != nil {
		ctx.APIError(http.StatusNotFound, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func CreatePushMirror(ctx *context.APIContext, mirrorOption *api.CreatePushMirrorOption) {
	repo := ctx.Repo.Repository

	interval, err := time.ParseDuration(mirrorOption.Interval)
	if err != nil || (interval != 0 && interval < setting.Mirror.MinInterval) {
		ctx.APIError(http.StatusBadRequest, err)
		return
	}

	authType := mirror_service.NormalizeMirrorAuthType(mirrorOption.AuthType)
	policy := mirror_service.NormalizeSSHHostKeyPolicy(mirrorOption.SSHHostKeyPolicy)

	var address string
	if authType == repo_model.MirrorAuthSSH {
		address = strings.TrimSpace(mirrorOption.RemoteAddress)
		if err := migrations.IsMigrateURLAllowed(address, ctx.ContextUser); err != nil {
			HandleRemoteAddressError(ctx, err)
			return
		}
		if err := mirror_service.ValidateSSHMirrorFields(policy, mirrorOption.SSHKnownHostFingerprint, mirrorOption.SSHPrivateKey, ""); err != nil {
			ctx.APIError(http.StatusBadRequest, err)
			return
		}
	} else {
		address, err = git.ParseRemoteAddr(mirrorOption.RemoteAddress, mirrorOption.RemoteUsername, mirrorOption.RemotePassword)
		if err == nil {
			err = migrations.IsMigrateURLAllowed(address, ctx.ContextUser)
		}
		if err != nil {
			HandleRemoteAddressError(ctx, err)
			return
		}
	}

	remoteSuffix, err := util.CryptoRandomString(10)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}

	remoteAddress, err := giturl.StripCredentialsForStorage(mirrorOption.RemoteAddress)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}

	var encKey string
	if authType == repo_model.MirrorAuthSSH {
		encKey, err = mirror_service.EncryptSSHPrivateKeyOrEmpty(mirrorOption.SSHPrivateKey)
		if err != nil {
			ctx.APIErrorInternal(err)
			return
		}
	}

	mirrorBranchList, err := mirror_service.ParseMirrorBranches(mirrorOption.MirrorBranches)
	if err != nil {
		ctx.APIError(http.StatusBadRequest, err.Error())
		return
	}

	pushMirror := &repo_model.PushMirror{
		RepoID:                  repo.ID,
		Repo:                    repo,
		RemoteName:              "remote_mirror_" + remoteSuffix,
		Interval:                interval,
		SyncOnCommit:            mirrorOption.SyncOnCommit,
		RemoteAddress:           remoteAddress,
		MirrorBranches:          mirror_service.JoinMirrorBranches(mirrorBranchList),
		AuthType:                authType,
		SSHPrivateKeyEncrypted:  encKey,
		SSHHostKeyPolicy:        policy,
		SSHKnownHostFingerprint: strings.TrimSpace(mirrorOption.SSHKnownHostFingerprint),
	}
	if authType != repo_model.MirrorAuthSSH {
		pushMirror.SSHPrivateKeyEncrypted = ""
		pushMirror.SSHHostKeyPolicy = repo_model.MirrorSSHHostKeyFingerprint
		pushMirror.SSHKnownHostFingerprint = ""
	}
	mirror_service.ApplyDeployStampFromForm(pushMirror,
		mirrorOption.DeployStampEnabled,
		mirrorOption.DeployStampBranches,
		mirrorOption.DeployStampAuthorName,
		mirrorOption.DeployStampAuthorEmail,
		mirrorOption.DeployStampCommitMessage,
	)
	if err := mirror_service.ApplyRecordFileFromForm(pushMirror,
		mirrorOption.RecordFileEnabled,
		mirrorOption.RecordFileBranches,
		mirrorOption.RecordFilePath,
		mirrorOption.RecordFileTemplate,
		mirrorOption.RecordFileAuthorName,
		mirrorOption.RecordFileAuthorEmail,
		mirrorOption.RecordFileCommitMessage,
	); err != nil {
		ctx.APIError(http.StatusBadRequest, err.Error())
		return
	}

	if err = db.Insert(ctx, pushMirror); err != nil {
		ctx.APIErrorInternal(err)
		return
	}

	// if the registration of the push mirrorOption fails remove it from the database
	if err = mirror_service.AddPushMirrorRemote(ctx, pushMirror, address); err != nil {
		if err := repo_model.DeletePushMirrors(ctx, repo_model.PushMirrorOptions{ID: pushMirror.ID, RepoID: pushMirror.RepoID}); err != nil {
			ctx.APIErrorInternal(err)
			return
		}
		ctx.APIErrorInternal(err)
		return
	}
	m, err := convert.ToPushMirror(ctx, pushMirror)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	ctx.JSON(http.StatusOK, m)
}

// EditPushMirror updates push mirror options including deploy stamp settings.
func EditPushMirror(ctx *context.APIContext) {
	// swagger:operation PATCH /repos/{owner}/{repo}/push_mirrors/{name} repository repoEditPushMirror
	// ---
	// summary: Update a push mirror
	// consumes:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// - name: name
	//   in: path
	//   description: remote name of push mirror
	//   type: string
	//   required: true
	// - name: body
	//   in: body
	//   schema:
	//     "$ref": "#/definitions/EditPushMirrorOption"
	// responses:
	//   "200":
	//     "$ref": "#/definitions/PushMirror"
	//   "400":
	//     "$ref": "#/responses/error"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"
	if !setting.Mirror.Enabled {
		ctx.APIError(http.StatusBadRequest, "Mirror feature is disabled")
		return
	}

	remoteName := ctx.PathParam("name")
	opt := web.GetForm(ctx).(*api.EditPushMirrorOption)

	m, exist, err := db.Get[repo_model.PushMirror](ctx, repo_model.PushMirrorOptions{
		RepoID:     ctx.Repo.Repository.ID,
		RemoteName: remoteName,
	}.ToConds())
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	} else if !exist {
		ctx.APIError(http.StatusNotFound, nil)
		return
	}

	oldMirrorBranches := m.MirrorBranches
	var remoteAddressForRefresh string

	if opt.Interval != nil {
		interval, err := time.ParseDuration(*opt.Interval)
		if err != nil || (interval != 0 && interval < setting.Mirror.MinInterval) {
			ctx.APIError(http.StatusBadRequest, err)
			return
		}
		m.Interval = interval
	}
	if opt.SyncOnCommit != nil {
		m.SyncOnCommit = *opt.SyncOnCommit
	}
	if opt.DeployStampEnabled != nil {
		m.DeployStampEnabled = *opt.DeployStampEnabled
	}
	if opt.DeployStampBranches != nil {
		m.DeployStampBranches = strings.TrimSpace(*opt.DeployStampBranches)
	}
	if opt.DeployStampAuthorName != nil {
		m.DeployStampAuthorName = strings.TrimSpace(*opt.DeployStampAuthorName)
	}
	if opt.DeployStampAuthorEmail != nil {
		m.DeployStampAuthorEmail = strings.TrimSpace(*opt.DeployStampAuthorEmail)
	}
	if opt.DeployStampCommitMessage != nil {
		m.DeployStampCommitMessage = mirror_service.NormalizeDeployStampCommitMessage(*opt.DeployStampCommitMessage)
	}
	if opt.MirrorBranches != nil {
		branches, err := mirror_service.ParseMirrorBranches(*opt.MirrorBranches)
		if err != nil {
			ctx.APIError(http.StatusBadRequest, err.Error())
			return
		}
		m.MirrorBranches = mirror_service.JoinMirrorBranches(branches)
	}
	if opt.RecordFileEnabled != nil {
		m.RecordFileEnabled = *opt.RecordFileEnabled
	}
	if opt.RecordFileBranches != nil {
		branches, err := mirror_service.ParseMirrorBranches(*opt.RecordFileBranches)
		if err != nil {
			ctx.APIError(http.StatusBadRequest, err.Error())
			return
		}
		m.RecordFileBranches = mirror_service.JoinMirrorBranches(branches)
	}
	if opt.RecordFilePath != nil {
		m.RecordFilePath = strings.TrimSpace(*opt.RecordFilePath)
	}
	if opt.RecordFileTemplate != nil {
		m.RecordFileTemplate = *opt.RecordFileTemplate
	}
	if opt.RecordFileAuthorName != nil {
		m.RecordFileAuthorName = strings.TrimSpace(*opt.RecordFileAuthorName)
	}
	if opt.RecordFileAuthorEmail != nil {
		m.RecordFileAuthorEmail = strings.TrimSpace(*opt.RecordFileAuthorEmail)
	}
	if opt.RecordFileCommitMessage != nil {
		m.RecordFileCommitMessage = strings.TrimSpace(*opt.RecordFileCommitMessage)
	}
	if err := mirror_service.ApplyRecordFileFromForm(m,
		m.RecordFileEnabled,
		m.RecordFileBranches,
		m.RecordFilePath,
		m.RecordFileTemplate,
		m.RecordFileAuthorName,
		m.RecordFileAuthorEmail,
		m.RecordFileCommitMessage,
	); err != nil {
		ctx.APIError(http.StatusBadRequest, err.Error())
		return
	}

	if opt.AuthType != nil {
		m.AuthType = mirror_service.NormalizeMirrorAuthType(*opt.AuthType)
	}
	if opt.SSHHostKeyPolicy != nil {
		m.SSHHostKeyPolicy = mirror_service.NormalizeSSHHostKeyPolicy(*opt.SSHHostKeyPolicy)
	}
	if opt.SSHKnownHostFingerprint != nil {
		m.SSHKnownHostFingerprint = strings.TrimSpace(*opt.SSHKnownHostFingerprint)
	}
	if opt.SSHPrivateKey != nil && strings.TrimSpace(*opt.SSHPrivateKey) != "" {
		encKey, err := mirror_service.EncryptSSHPrivateKeyOrEmpty(*opt.SSHPrivateKey)
		if err != nil {
			ctx.APIErrorInternal(err)
			return
		}
		m.SSHPrivateKeyEncrypted = encKey
	}
	if m.AuthType == repo_model.MirrorAuthSSH {
		if err := mirror_service.ValidateSSHMirrorFields(m.SSHHostKeyPolicy, m.SSHKnownHostFingerprint, "", m.SSHPrivateKeyEncrypted); err != nil {
			ctx.APIError(http.StatusBadRequest, err.Error())
			return
		}
	} else {
		m.SSHPrivateKeyEncrypted = ""
		m.SSHHostKeyPolicy = repo_model.MirrorSSHHostKeyFingerprint
		m.SSHKnownHostFingerprint = ""
	}
	if opt.RemoteAddress != nil {
		authType := mirror_service.NormalizeMirrorAuthType(m.AuthType)
		var address string
		if authType == repo_model.MirrorAuthSSH {
			address = strings.TrimSpace(*opt.RemoteAddress)
			if err := migrations.IsMigrateURLAllowed(address, ctx.ContextUser); err != nil {
				HandleRemoteAddressError(ctx, err)
				return
			}
		} else {
			username := ""
			password := ""
			if opt.RemoteUsername != nil {
				username = *opt.RemoteUsername
			}
			if opt.RemotePassword != nil {
				password = *opt.RemotePassword
			}
			var err error
			address, err = git.ParseRemoteAddr(*opt.RemoteAddress, username, password)
			if err == nil {
				err = migrations.IsMigrateURLAllowed(address, ctx.ContextUser)
			}
			if err != nil {
				HandleRemoteAddressError(ctx, err)
				return
			}
		}
		remoteAddress, err := giturl.StripCredentialsForStorage(*opt.RemoteAddress)
		if err != nil {
			HandleRemoteAddressError(ctx, err)
			return
		}
		m.RemoteAddress = remoteAddress
		remoteAddressForRefresh = address
	}

	if err := repo_model.UpdatePushMirror(ctx, m); err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	if remoteAddressForRefresh != "" {
		if err := mirror_service.RemovePushMirrorRemote(ctx, m); err != nil {
			ctx.APIErrorInternal(err)
			return
		}
		if err := mirror_service.AddPushMirrorRemote(ctx, m, remoteAddressForRefresh); err != nil {
			ctx.APIErrorInternal(err)
			return
		}
	} else if m.MirrorBranches != oldMirrorBranches {
		if err := mirror_service.RefreshPushMirrorRemote(ctx, m); err != nil {
			ctx.APIErrorInternal(err)
			return
		}
	}
	resp, err := convert.ToPushMirror(ctx, m)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

// ListPushMirrorSyncTasks returns recent sync attempts for a push mirror.
func ListPushMirrorSyncTasks(ctx *context.APIContext) {
	if !setting.Mirror.Enabled {
		ctx.APIError(http.StatusBadRequest, "Mirror feature is disabled")
		return
	}
	remoteName := ctx.PathParam("name")
	m, exist, err := db.Get[repo_model.PushMirror](ctx, repo_model.PushMirrorOptions{
		RepoID:     ctx.Repo.Repository.ID,
		RemoteName: remoteName,
	}.ToConds())
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	} else if !exist {
		ctx.APIError(http.StatusNotFound, nil)
		return
	}
	page := ctx.FormInt("page")
	tasks, _, err := repo_model.GetMirrorSyncTasks(ctx, ctx.Repo.Repository.ID, repo_model.MirrorSyncTypePush, m.ID, page)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	out := make([]*api.MirrorSyncTask, 0, len(tasks))
	for _, t := range tasks {
		item := &api.MirrorSyncTask{
			UUID:         t.UUID,
			MirrorType:   t.MirrorType,
			TriggerType:  t.TriggerType,
			IsSucceed:    t.IsSucceed,
			Stdout:       t.Stdout,
			Stderr:       t.Stderr,
			ErrorMessage: t.ErrorMessage,
			StartedUnix:  t.StartedUnix.AsTime(),
		}
		if t.FinishedUnix > 0 {
			item.FinishedUnix = t.FinishedUnix.AsTime()
		}
		out = append(out, item)
	}
	ctx.JSON(http.StatusOK, out)
}

func HandleRemoteAddressError(ctx *context.APIContext, err error) {
	if git.IsErrInvalidCloneAddr(err) {
		addrErr := err.(*git.ErrInvalidCloneAddr)
		switch {
		case addrErr.IsProtocolInvalid:
			ctx.APIError(http.StatusBadRequest, "Invalid mirror protocol")
		case addrErr.IsURLError:
			ctx.APIError(http.StatusBadRequest, "Invalid Url ")
		case addrErr.IsPermissionDenied:
			ctx.APIError(http.StatusUnauthorized, "Permission denied")
		default:
			ctx.APIError(http.StatusBadRequest, "Unknown error")
		}
		return
	}
}
