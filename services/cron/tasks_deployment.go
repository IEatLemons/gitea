// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cron

import (
	"context"

	user_model "code.gitea.io/gitea/models/user"
	deployment_service "code.gitea.io/gitea/services/deployment"
)

func initDeploymentTasks() {
	RegisterTaskFatal("sync_deployment_platforms", &BaseConfig{
		Enabled:    true,
		RunAtStart: false,
		Schedule:   "@every 5m",
	}, func(ctx context.Context, _ *user_model.User, _ Config) error {
		return deployment_service.SyncAll(ctx)
	})
}
