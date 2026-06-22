// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"testing"
	"time"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
)

func TestPushMirrorsIterate(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	now := timeutil.TimeStampNow()

	db.Insert(t.Context(), &repo_model.PushMirror{
		RemoteName:     "test-1",
		LastUpdateUnix: now,
		Interval:       1,
	})

	long, _ := time.ParseDuration("24h")
	db.Insert(t.Context(), &repo_model.PushMirror{
		RemoteName:     "test-2",
		LastUpdateUnix: now,
		Interval:       long,
	})

	db.Insert(t.Context(), &repo_model.PushMirror{
		RemoteName:     "test-3",
		LastUpdateUnix: now,
		Interval:       0,
	})

	repo_model.PushMirrorsIterate(t.Context(), 1, func(idx int, bean any) error {
		m, ok := bean.(*repo_model.PushMirror)
		assert.True(t, ok)
		assert.Equal(t, "test-1", m.RemoteName)
		assert.Equal(t, m.RemoteName, m.GetRemoteName())
		return nil
	})
}

func TestMirrorSyncTaskRunningAndStale(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	assert.NoError(t, db.TruncateBeans(t.Context(), &repo_model.MirrorSyncTask{}))

	task := &repo_model.MirrorSyncTask{
		RepoID:       1,
		MirrorType:   repo_model.MirrorSyncTypePush,
		PushMirrorID: 2,
		TriggerType:  repo_model.MirrorSyncTriggerScheduled,
	}
	assert.NoError(t, repo_model.InsertMirrorSyncTask(t.Context(), task))

	hasRunning, err := repo_model.HasRunningMirrorSyncTask(t.Context(), 1, repo_model.MirrorSyncTypePush, 2)
	assert.NoError(t, err)
	assert.True(t, hasRunning)

	startedUnix := timeutil.TimeStampNow().AddDuration(-10 * time.Minute)
	_, err = db.GetEngine(t.Context()).ID(task.ID).Cols("started_unix").Update(&repo_model.MirrorSyncTask{
		StartedUnix: startedUnix,
	})
	assert.NoError(t, err)

	updated, err := repo_model.MarkStaleMirrorSyncTasksFailed(
		t.Context(),
		1,
		repo_model.MirrorSyncTypePush,
		2,
		timeutil.TimeStampNow().AddDuration(-5*time.Minute),
		"timed out",
	)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, updated)

	hasRunning, err = repo_model.HasRunningMirrorSyncTask(t.Context(), 1, repo_model.MirrorSyncTypePush, 2)
	assert.NoError(t, err)
	assert.False(t, hasRunning)

	task = unittest.AssertExistsAndLoadBean(t, &repo_model.MirrorSyncTask{ID: task.ID})
	assert.False(t, task.IsSucceed)
	assert.NotZero(t, task.FinishedUnix)
	assert.Equal(t, "timed out", task.ErrorMessage)
}
