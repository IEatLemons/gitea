// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package deployment

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	deployment_model "code.gitea.io/gitea/models/deployment"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeDeploymentStatus(t *testing.T) {
	assert.Equal(t, deployment_model.StatusQueued, normalizeRailwayStatus("WAITING"))
	assert.Equal(t, deployment_model.StatusRunning, normalizeRailwayStatus("DEPLOYING"))
	assert.Equal(t, deployment_model.StatusSuccess, normalizeRailwayStatus("SUCCESS"))
	assert.Equal(t, deployment_model.StatusFailure, normalizeRailwayStatus("CRASHED"))
	assert.Equal(t, deployment_model.StatusCanceled, normalizeRailwayStatus("SKIPPED"))
	assert.Equal(t, deployment_model.StatusUnknown, normalizeRailwayStatus("NEW_STATE"))

	assert.Equal(t, deployment_model.StatusQueued, normalizeVercelStatus("QUEUED"))
	assert.Equal(t, deployment_model.StatusRunning, normalizeVercelStatus("BUILDING"))
	assert.Equal(t, deployment_model.StatusSuccess, normalizeVercelStatus("READY"))
	assert.Equal(t, deployment_model.StatusFailure, normalizeVercelStatus("ERROR"))
	assert.Equal(t, deployment_model.StatusCanceled, normalizeVercelStatus("CANCELED"))
}

func TestVercelFetchPreview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "team-1", r.URL.Query().Get("teamId"))
		assert.Equal(t, "preview", r.URL.Query().Get("target"))
		assert.Equal(t, "develop", r.URL.Query().Get("meta-gitCommitRef"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deployments":[
          {"uid":"latest","url":"latest.example","readyState":"ERROR","created":1700000002000,"meta":{"githubCommitSha":"2222222222","githubCommitRef":"develop"}},
          {"uid":"active","url":"active.example","readyState":"READY","created":1700000001000,"meta":{"githubCommitSha":"1111111111","githubCommitRef":"develop"}}
        ]}`))
	}))
	defer server.Close()

	client := &vercelClient{token: "test-token", teamID: "team-1", httpClient: server.Client(), endpoint: server.URL}
	summary, err := client.Fetch(t.Context(), &deployment_model.Binding{
		ProjectID: "project-1", EnvironmentID: "preview", BranchFilter: "develop",
	})
	require.NoError(t, err)
	require.NotNil(t, summary.Latest)
	require.NotNil(t, summary.Active)
	assert.Equal(t, "latest", summary.Latest.ID)
	assert.Equal(t, deployment_model.StatusFailure, summary.Latest.Status)
	assert.Equal(t, "active", summary.Active.ID)
	assert.Equal(t, "1111111111", summary.Active.CommitSHA)
	assert.Equal(t, "https://active.example", summary.Active.URL)
}

func TestVercelFetchPaginatesUntilActiveDeployment(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("from") == "1700000001000" {
			_, _ = w.Write([]byte(`{"deployments":[{"uid":"active","url":"active.example","readyState":"READY","created":1700000000000,"meta":{"gitCommitSha":"1111111111"}}],"pagination":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"deployments":[{"uid":"latest","url":"latest.example","readyState":"ERROR","created":1700000002000,"meta":{"gitCommitSha":"2222222222"}}],"pagination":{"next":1700000001000}}`))
	}))
	defer server.Close()

	client := &vercelClient{token: "test-token", httpClient: server.Client(), endpoint: server.URL}
	summary, err := client.Fetch(t.Context(), &deployment_model.Binding{ProjectID: "project-1", EnvironmentID: "production"})
	require.NoError(t, err)
	assert.Equal(t, 2, requests)
	require.NotNil(t, summary.Latest)
	require.NotNil(t, summary.Active)
	assert.Equal(t, "latest", summary.Latest.ID)
	assert.Equal(t, "active", summary.Active.ID)
}

func TestRailwayGraphQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"forbidden"}]}`))
	}))
	defer server.Close()

	client := &railwayClient{token: "test-token", httpClient: server.Client(), endpoint: server.URL}
	err := client.Test(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Railway API error")
}

func TestResponseSizeLimit(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(make([]byte, maxResponseSize+1))),
	}
	var target map[string]any
	require.Error(t, decodeJSONResponse(response, &target))
}

func TestSafeDeploymentURL(t *testing.T) {
	assert.Equal(t, "https://example.com/deployment", safeDeploymentURL("example.com/deployment", ""))
	assert.Empty(t, safeDeploymentURL("javascript:alert(1)", ""))
	assert.Empty(t, safeDeploymentURL("http://example.com", ""))
}
