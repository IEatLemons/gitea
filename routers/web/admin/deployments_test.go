// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package admin

import (
	"testing"

	deployment_service "code.gitea.io/gitea/services/deployment"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeploymentTargetEncoding(t *testing.T) {
	target := deployment_service.RemoteTarget{
		ProjectID: "project|id", ProjectName: "Project / Name",
		ServiceID: "service-id", ServiceName: "Web API",
		EnvironmentID: "environment-id", EnvironmentName: "测试环境",
	}
	decoded, err := decodeDeploymentTarget(encodeDeploymentTarget(target))
	require.NoError(t, err)
	assert.Equal(t, target, decoded)
}

func TestInvalidDeploymentTargetEncoding(t *testing.T) {
	_, err := decodeDeploymentTarget("invalid")
	require.Error(t, err)
}
