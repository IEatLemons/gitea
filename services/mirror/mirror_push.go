// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package mirror

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/lfs"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/process"
	"code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/migrations"
	repo_service "code.gitea.io/gitea/services/repository"
)

var stripExitStatus = regexp.MustCompile(`exit status \d+ - `)

const stalePushMirrorSyncMessage = "Mirror sync did not finish before the configured timeout " +
	"and was marked as failed."

func addFullMirrorRemoteAndConfig(ctx context.Context, storageRepo gitrepo.Repository, remoteName, addr string) error {
	if err := gitrepo.GitRemoteAdd(ctx, storageRepo, remoteName, addr, gitrepo.RemoteOptionMirrorPush); err != nil {
		return err
	}
	if err := gitrepo.GitConfigAdd(ctx, storageRepo, "remote."+remoteName+".push", "+refs/heads/*:refs/heads/*"); err != nil {
		return err
	}
	return gitrepo.GitConfigAdd(ctx, storageRepo, "remote."+remoteName+".push", "+refs/tags/*:refs/tags/*")
}

func addBranchRestrictedMainRemote(ctx context.Context, storageRepo gitrepo.Repository, remoteName, addr string, branches []string) error {
	if err := gitrepo.GitRemoteAdd(ctx, storageRepo, remoteName, addr); err != nil {
		return err
	}
	for _, b := range branches {
		ref := "+refs/heads/" + b + ":refs/heads/" + b
		if err := gitrepo.GitConfigAdd(ctx, storageRepo, "remote."+remoteName+".push", ref); err != nil {
			return err
		}
	}
	return nil
}

// AddPushMirrorRemote registers the push mirror remote.
func AddPushMirrorRemote(ctx context.Context, m *repo_model.PushMirror, addr string) error {
	branches, err := ParseMirrorBranches(m.MirrorBranches)
	if err != nil {
		return err
	}

	if len(branches) == 0 {
		if err := addFullMirrorRemoteAndConfig(ctx, m.Repo, m.RemoteName, addr); err != nil {
			return err
		}
	} else {
		if err := addBranchRestrictedMainRemote(ctx, m.Repo, m.RemoteName, addr, branches); err != nil {
			return err
		}
	}

	if repo_service.HasWiki(ctx, m.Repo) {
		wikiRemoteURL := repository.WikiRemoteURL(ctx, addr)
		if len(wikiRemoteURL) > 0 {
			if err := addFullMirrorRemoteAndConfig(ctx, m.Repo.WikiStorageRepo(), m.RemoteName, wikiRemoteURL); err != nil {
				return err
			}
		}
	}

	return nil
}

// RefreshPushMirrorRemote reapplies git remote and push refspecs from the PushMirror row using the current remote URL.
func RefreshPushMirrorRemote(ctx context.Context, m *repo_model.PushMirror) error {
	_ = m.GetRepository(ctx)
	remoteURL, err := gitrepo.GitRemoteGetURL(ctx, m.Repo, m.RemoteName)
	if err != nil {
		return err
	}
	addr := remoteURL.String()
	if err := RemovePushMirrorRemote(ctx, m); err != nil {
		return err
	}
	return AddPushMirrorRemote(ctx, m, addr)
}

// RemovePushMirrorRemote removes the push mirror remote.
func RemovePushMirrorRemote(ctx context.Context, m *repo_model.PushMirror) error {
	_ = m.GetRepository(ctx)
	if err := gitrepo.GitRemoteRemove(ctx, m.Repo, m.RemoteName); err != nil {
		return err
	}

	if repo_service.HasWiki(ctx, m.Repo) {
		if err := gitrepo.GitRemoteRemove(ctx, m.Repo.WikiStorageRepo(), m.RemoteName); err != nil {
			// The wiki remote may not exist
			log.Warn("Wiki Remote[%d] could not be removed: %v", m.ID, err)
		}
	}

	return nil
}

// SyncPushMirror starts the sync of the push mirror and schedules the next run.
// triggerType should be one of repo.MirrorSyncTrigger*; empty defaults to scheduled.
func SyncPushMirror(ctx context.Context, mirrorID int64, triggerType string) bool {
	log.Trace("SyncPushMirror [mirror: %d]", mirrorID)
	defer func() {
		err := recover()
		if err == nil {
			return
		}
		// There was a panic whilst syncPushMirror...
		log.Error("PANIC whilst syncPushMirror[%d] Panic: %v\nStacktrace: %s", mirrorID, err, log.Stack(2))
	}()

	if triggerType == "" {
		triggerType = repo_model.MirrorSyncTriggerScheduled
	}

	// TODO: Handle "!exist" better
	m, exist, err := db.GetByID[repo_model.PushMirror](ctx, mirrorID)
	if err != nil || !exist {
		log.Error("GetPushMirrorByID [%d]: %v", mirrorID, err)
		return false
	}

	_ = m.GetRepository(ctx)

	m.LastError = ""

	if hasRunning, err := cleanupAndCheckRunningPushMirrorSync(ctx, m); err != nil {
		log.Error("cleanupAndCheckRunningPushMirrorSync push mirror[%d]: %v", m.ID, err)
	} else if hasRunning {
		log.Trace("SyncPushMirror [mirror: %d][repo: %-v]: Skipping sync because another task is still running", m.ID, m.Repo)
		return false
	}

	task := &repo_model.MirrorSyncTask{
		RepoID:       m.RepoID,
		MirrorType:   repo_model.MirrorSyncTypePush,
		PushMirrorID: m.ID,
		TriggerType:  triggerType,
	}
	if err := repo_model.InsertMirrorSyncTask(ctx, task); err != nil {
		log.Error("InsertMirrorSyncTask push mirror[%d]: %v", m.ID, err)
	}

	ctx, _, finished := process.GetManager().AddContext(ctx, fmt.Sprintf("Syncing PushMirror %s/%s to %s", m.Repo.OwnerName, m.Repo.Name, m.RemoteName))
	defer finished()

	log.Trace("SyncPushMirror [mirror: %d][repo: %-v]: Running Sync", m.ID, m.Repo)
	stdout, stderr, runErr := runPushSync(ctx, m, triggerType)
	if runErr != nil {
		log.Error("SyncPushMirror [mirror: %d][repo: %-v]: %v", m.ID, m.Repo, runErr)
		m.LastError = stripExitStatus.ReplaceAllLiteralString(runErr.Error(), "")
	}

	m.LastUpdateUnix = timeutil.TimeStampNow()

	syncErr := runErr
	if task.ID != 0 {
		task.IsSucceed = syncErr == nil
		task.Stdout = repo_model.TruncateMirrorSyncOutput(util.SanitizeCredentialURLs(stdout))
		task.Stderr = repo_model.TruncateMirrorSyncOutput(util.SanitizeCredentialURLs(stderr))
		task.FinishedUnix = timeutil.TimeStampNow()
		if syncErr != nil {
			task.ErrorMessage = stripExitStatus.ReplaceAllLiteralString(syncErr.Error(), "")
		}
		if err := repo_model.UpdateMirrorSyncTaskCompleted(ctx, task); err != nil {
			log.Error("UpdateMirrorSyncTask [%d]: %v", task.ID, err)
		}
	}

	if err := repo_model.UpdatePushMirror(ctx, m); err != nil {
		log.Error("UpdatePushMirror [%d]: %v", m.ID, err)

		return false
	}

	log.Trace("SyncPushMirror [mirror: %d][repo: %-v]: Finished", m.ID, m.Repo)

	return syncErr == nil
}

func cleanupAndCheckRunningPushMirrorSync(ctx context.Context, m *repo_model.PushMirror) (bool, error) {
	timeout := time.Duration(setting.Git.Timeout.Mirror) * time.Second
	if timeout > 0 {
		startedBefore := timeutil.TimeStampNow().AddDuration(-timeout)
		if _, err := repo_model.MarkStaleMirrorSyncTasksFailed(
			ctx,
			m.RepoID,
			repo_model.MirrorSyncTypePush,
			m.ID,
			startedBefore,
			stalePushMirrorSyncMessage,
		); err != nil {
			return false, err
		}
	}
	return repo_model.HasRunningMirrorSyncTask(ctx, m.RepoID, repo_model.MirrorSyncTypePush, m.ID)
}

func runPushSync(ctx context.Context, m *repo_model.PushMirror, triggerType string) (stdout, stderr string, err error) {
	timeout := time.Duration(setting.Git.Timeout.Mirror) * time.Second

	var outBuf, errBuf strings.Builder

	performPush := func(repo *repo_model.Repository, isWiki bool) error {
		var storageRepo gitrepo.Repository = repo
		if isWiki {
			storageRepo = repo.WikiStorageRepo()
		}
		if !isWiki {
			if err := MaybeRecordFileBeforePush(ctx, m, triggerType); err != nil {
				return err
			}
		}
		remoteURL, err := gitrepo.GitRemoteGetURL(ctx, storageRepo, m.RemoteName)
		if err != nil {
			log.Error("GetRemoteURL(%s) Error %v", storageRepo.RelativePath(), err)
			return errors.New("Unexpected error")
		}

		if setting.LFS.StartServer {
			log.Trace("SyncMirrors [repo: %-v]: syncing LFS objects...", m.Repo)

			gitRepo, err := gitrepo.OpenRepository(ctx, storageRepo)
			if err != nil {
				log.Error("OpenRepository: %v", err)
				return errors.New("Unexpected error")
			}
			defer gitRepo.Close()

			endpoint := lfs.DetermineEndpoint(remoteURL.String(), "")
			lfsClient := lfs.NewClient(endpoint, migrations.NewMigrationHTTPTransport())
			if err := pushAllLFSObjects(ctx, gitRepo, lfsClient); err != nil {
				return util.SanitizeErrorCredentialURLs(err)
			}
		}

		log.Trace("Pushing %s mirror[%d] remote %s", storageRepo.RelativePath(), m.ID, m.RemoteName)

		restrictedBranches, _ := ParseMirrorBranches(m.MirrorBranches)
		useMirrorPush := len(restrictedBranches) == 0 || isWiki

		envs, envCleanup, err := BuildPushMirrorGitEnvs(remoteURL, m)
		if err != nil {
			return err
		}
		defer envCleanup()

		pushErr := gitrepo.PushToExternal(ctx, storageRepo, git.PushOptions{
			Remote:  m.RemoteName,
			Force:   true,
			Mirror:  useMirrorPush,
			Timeout: timeout,
			Env:     envs,
		})
		if pushErr != nil {
			so, se := extractPushGitOutput(pushErr)
			if so != "" {
				outBuf.WriteString(so)
				outBuf.WriteByte('\n')
			}
			if se != "" {
				errBuf.WriteString(se)
				errBuf.WriteByte('\n')
			}
			log.Error("Error pushing %s mirror[%d] remote %s: %v", storageRepo.RelativePath(), m.ID, m.RemoteName, pushErr)

			return util.SanitizeErrorCredentialURLs(pushErr)
		}

		return nil
	}

	err = performPush(m.Repo, false)
	if err != nil {
		return strings.TrimSpace(outBuf.String()), strings.TrimSpace(errBuf.String()), err
	}

	if repo_service.HasWiki(ctx, m.Repo) {
		_, wikiErr := gitrepo.GitRemoteGetURL(ctx, m.Repo.WikiStorageRepo(), m.RemoteName)
		if wikiErr == nil {
			err = performPush(m.Repo, true)
			if err != nil {
				return strings.TrimSpace(outBuf.String()), strings.TrimSpace(errBuf.String()), err
			}
		} else if !errors.Is(wikiErr, util.ErrNotExist) {
			log.Error("GetRemote of wiki failed: %v", wikiErr)
		}
	}

	return strings.TrimSpace(outBuf.String()), strings.TrimSpace(errBuf.String()), nil
}

func pushAllLFSObjects(ctx context.Context, gitRepo *git.Repository, lfsClient lfs.Client) error {
	contentStore := lfs.NewContentStore()

	pointerChan := make(chan lfs.PointerBlob)
	errChan := make(chan error, 1)
	go func() {
		errChan <- lfs.SearchPointerBlobs(ctx, gitRepo, pointerChan)
	}()

	uploadObjects := func(pointers []lfs.Pointer) error {
		err := lfsClient.Upload(ctx, pointers, func(p lfs.Pointer, objectError error) (io.ReadCloser, error) {
			if objectError != nil {
				return nil, objectError
			}

			content, err := contentStore.Get(p)
			if err != nil {
				log.Error("Error reading LFS object %v: %v", p, err)
			}
			return content, err
		})
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
		}
		return err
	}

	var batch []lfs.Pointer
	for pointerBlob := range pointerChan {
		exists, err := contentStore.Exists(pointerBlob.Pointer)
		if err != nil {
			log.Error("Error checking if LFS object %v exists: %v", pointerBlob.Pointer, err)
			return err
		}
		if !exists {
			log.Trace("Skipping missing LFS object %v", pointerBlob.Pointer)
			continue
		}

		batch = append(batch, pointerBlob.Pointer)
		if len(batch) >= lfsClient.BatchSize() {
			if err := uploadObjects(batch); err != nil {
				return err
			}
			batch = nil
		}
	}
	if len(batch) > 0 {
		if err := uploadObjects(batch); err != nil {
			return err
		}
	}

	err := <-errChan
	if err != nil {
		log.Error("Error enumerating LFS objects for repository: %v", err)
	}

	return err
}

func syncPushMirrorWithSyncOnCommit(ctx context.Context, repoID int64) {
	pushMirrors, err := repo_model.GetPushMirrorsSyncedOnCommit(ctx, repoID)
	if err != nil {
		log.Error("repo_model.GetPushMirrorsSyncedOnCommit failed: %v", err)
		return
	}

	for _, mirror := range pushMirrors {
		AddPushMirrorToQueue(mirror.ID, repo_model.MirrorSyncTriggerCommit)
	}
}
