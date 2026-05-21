// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"

	gouuid "github.com/google/uuid"
	"xorm.io/builder"
)

const mirrorSyncOutputMaxLen = 65536

// MirrorSyncTask stores one mirror sync attempt (pull or push).
type MirrorSyncTask struct {
	ID           int64  `xorm:"pk autoincr"`
	UUID         string `xorm:"VARCHAR(40) UNIQUE"`
	RepoID       int64  `xorm:"INDEX NOT NULL"`
	MirrorType   string `xorm:"VARCHAR(8) NOT NULL"` // MirrorSyncTypePull / MirrorSyncTypePush
	PushMirrorID int64  `xorm:"INDEX NOT NULL DEFAULT 0"`
	TriggerType  string `xorm:"VARCHAR(16) NOT NULL"`
	IsSucceed    bool
	Stdout       string `xorm:"LONGTEXT"`
	Stderr       string `xorm:"LONGTEXT"`
	ErrorMessage string `xorm:"TEXT"`
	StartedUnix  timeutil.TimeStamp `xorm:"INDEX"`
	FinishedUnix timeutil.TimeStamp
}

func init() {
	db.RegisterModel(new(MirrorSyncTask))
}

// TruncateMirrorSyncOutput limits stored git output size.
func TruncateMirrorSyncOutput(s string) string {
	if len(s) <= mirrorSyncOutputMaxLen {
		return s
	}
	return s[:mirrorSyncOutputMaxLen] + "\n...[truncated]"
}

type findMirrorSyncTaskOptions struct {
	db.ListOptions
	RepoID       int64
	MirrorType   string
	PushMirrorID int64 // 0 = any for pull mirror tasks when MirrorType=Pull
}

func (opts findMirrorSyncTaskOptions) ToConds() builder.Cond {
	cond := builder.NewCond()
	if opts.RepoID > 0 {
		cond = cond.And(builder.Eq{"repo_id": opts.RepoID})
	}
	if opts.MirrorType != "" {
		cond = cond.And(builder.Eq{"mirror_type": opts.MirrorType})
	}
	if opts.PushMirrorID != 0 {
		cond = cond.And(builder.Eq{"push_mirror_id": opts.PushMirrorID})
	}
	return cond
}

// InsertMirrorSyncTask creates a new sync task row.
func InsertMirrorSyncTask(ctx context.Context, t *MirrorSyncTask) error {
	if t.UUID == "" {
		t.UUID = gouuid.New().String()
	}
	t.StartedUnix = timeutil.TimeStampNow()
	_, err := db.GetEngine(ctx).Insert(t)
	return err
}

// UpdateMirrorSyncTaskCompleted persists stdout/stderr and outcome after a sync finishes.
func UpdateMirrorSyncTaskCompleted(ctx context.Context, t *MirrorSyncTask) error {
	_, err := db.GetEngine(ctx).ID(t.ID).Cols(
		"is_succeed", "stdout", "stderr", "error_message", "finished_unix",
	).Update(t)
	return err
}

// GetMirrorSyncTaskByUUID returns a task belonging to the repo (for replay authorization).
func GetMirrorSyncTaskByUUID(ctx context.Context, repoID int64, uuid string) (*MirrorSyncTask, bool, error) {
	var row MirrorSyncTask
	has, err := db.GetEngine(ctx).Where("repo_id = ? AND uuid = ?", repoID, uuid).Get(&row)
	if err != nil || !has {
		return nil, has, err
	}
	return &row, true, nil
}

// GetMirrorSyncTasks returns paginated history for a push mirror or pull mirror.
func GetMirrorSyncTasks(ctx context.Context, repoID int64, mirrorType string, pushMirrorID int64, page int) ([]*MirrorSyncTask, int64, error) {
	if page <= 0 {
		page = 1
	}
	paging := setting.Mirror.SyncHistoryLimit
	if paging <= 0 {
		paging = 50
	}
	opts := findMirrorSyncTaskOptions{
		ListOptions: db.ListOptions{
			Page:     page,
			PageSize: paging,
		},
		RepoID:       repoID,
		MirrorType:   mirrorType,
		PushMirrorID: pushMirrorID,
	}
	return db.FindAndCount[MirrorSyncTask](ctx, opts)
}

// DeleteMirrorSyncTasksForPushMirror removes history when a push mirror is deleted.
func DeleteMirrorSyncTasksForPushMirror(ctx context.Context, repoID, pushMirrorID int64) error {
	_, err := db.GetEngine(ctx).Where("repo_id = ? AND push_mirror_id = ?", repoID, pushMirrorID).Delete(new(MirrorSyncTask))
	return err
}

// DeleteMirrorSyncTasksForPullMirror removes pull mirror sync history for a repo.
func DeleteMirrorSyncTasksForPullMirror(ctx context.Context, repoID int64) error {
	_, err := db.GetEngine(ctx).Where("repo_id = ? AND mirror_type = ?", repoID, MirrorSyncTypePull).Delete(new(MirrorSyncTask))
	return err
}
