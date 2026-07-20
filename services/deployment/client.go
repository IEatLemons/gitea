// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package deployment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	deployment_model "code.gitea.io/gitea/models/deployment"
	"code.gitea.io/gitea/modules/json"
)

const maxResponseSize = 4 * 1024 * 1024

// RemoteTarget is a platform project, service, and environment selectable by an administrator.
type RemoteTarget struct {
	ProjectID       string
	ProjectName     string
	ServiceID       string
	ServiceName     string
	EnvironmentID   string
	EnvironmentName string
}

// RemoteDeployment is the provider-neutral subset displayed by Gitea.
type RemoteDeployment struct {
	ID        string
	CommitSHA string
	Status    deployment_model.Status
	URL       string
	Created   time.Time
}

// Summary contains the active deployment and the latest deployment attempt.
type Summary struct {
	Active *RemoteDeployment
	Latest *RemoteDeployment
}

// Client defines the read-only operations supported by deployment platforms.
type Client interface {
	Test(context.Context) error
	Discover(context.Context) ([]RemoteTarget, error)
	Fetch(context.Context, *deployment_model.Binding) (*Summary, error)
}

// NewClient creates a read-only client for a stored platform connection.
func NewClient(connection *deployment_model.Connection) (Client, error) {
	token, err := connection.Token()
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Timeout: 20 * time.Second}
	switch connection.Provider {
	case deployment_model.ProviderRailway:
		return &railwayClient{token: token, workspaceID: connection.ScopeID, httpClient: httpClient, endpoint: "https://backboard.railway.com/graphql/v2"}, nil
	case deployment_model.ProviderVercel:
		return &vercelClient{token: token, teamID: connection.ScopeID, httpClient: httpClient, endpoint: "https://api.vercel.com"}, nil
	default:
		return nil, fmt.Errorf("unsupported deployment provider %q", connection.Provider)
	}
}

func decodeJSONResponse(resp *http.Response, target any) error {
	defer resp.Body.Close()
	reader := io.LimitReader(resp.Body, maxResponseSize+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if len(data) > maxResponseSize {
		return errors.New("deployment platform response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("deployment platform returned HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("invalid deployment platform response: %w", err)
	}
	return nil
}

func doRequestWithRateLimitRetry(ctx context.Context, client *http.Client, request func() (*http.Request, error)) (*http.Response, error) {
	for attempt := range 2 {
		req, err := request()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt == 1 {
			return resp, nil
		}
		resp.Body.Close()
		delay := time.Second
		if retryAfter, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && retryAfter > 0 && retryAfter <= 5 {
			delay = time.Duration(retryAfter) * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, errors.New("deployment platform rate-limit retry failed")
}

func normalizeRailwayStatus(status string) deployment_model.Status {
	switch strings.ToUpper(status) {
	case "INITIALIZING", "QUEUED", "WAITING":
		return deployment_model.StatusQueued
	case "BUILDING", "DEPLOYING":
		return deployment_model.StatusRunning
	case "SUCCESS", "SLEEPING":
		return deployment_model.StatusSuccess
	case "FAILED", "CRASHED", "REMOVED":
		return deployment_model.StatusFailure
	case "CANCELED", "CANCELLED", "SKIPPED":
		return deployment_model.StatusCanceled
	default:
		return deployment_model.StatusUnknown
	}
}

func normalizeVercelStatus(status string) deployment_model.Status {
	switch strings.ToUpper(status) {
	case "QUEUED", "INITIALIZING":
		return deployment_model.StatusQueued
	case "BUILDING":
		return deployment_model.StatusRunning
	case "READY":
		return deployment_model.StatusSuccess
	case "ERROR":
		return deployment_model.StatusFailure
	case "CANCELED", "CANCELLED":
		return deployment_model.StatusCanceled
	default:
		return deployment_model.StatusUnknown
	}
}

func safeDeploymentURL(rawURL, fallback string) string {
	if rawURL == "" {
		rawURL = fallback
	}
	if rawURL != "" && !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}

type railwayClient struct {
	token       string
	workspaceID string
	httpClient  *http.Client
	endpoint    string
}

type railwayGraphQLResponse struct {
	Data   map[string]any `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *railwayClient) query(ctx context.Context, query string, variables map[string]any, target any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	resp, err := doRequestWithRateLimitRetry(ctx, c.httpClient, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+c.token)
			req.Header.Set("Content-Type", "application/json")
		}
		return req, err
	})
	if err != nil {
		return err
	}
	var envelope railwayGraphQLResponse
	if err := decodeJSONResponse(resp, &envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("Railway API error: %s", envelope.Errors[0].Message)
	}
	if len(envelope.Data) == 0 {
		return errors.New("Railway API returned no data")
	}
	data, err := json.Marshal(envelope.Data)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func (c *railwayClient) Test(ctx context.Context) error {
	if c.workspaceID != "" {
		var result struct {
			Workspace struct {
				ID string `json:"id"`
			} `json:"workspace"`
		}
		return c.query(ctx, `query($workspaceId: String!) { workspace(workspaceId: $workspaceId) { id } }`, map[string]any{"workspaceId": c.workspaceID}, &result)
	}
	var result struct {
		Me struct {
			ID string `json:"id"`
		} `json:"me"`
	}
	return c.query(ctx, `query { me { id } }`, nil, &result)
}

func (c *railwayClient) Discover(ctx context.Context) ([]RemoteTarget, error) {
	type edge[T any] struct {
		Node T `json:"node"`
	}
	type environment struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type service struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type project struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Services struct {
			Edges []edge[service] `json:"edges"`
		} `json:"services"`
		Environments struct {
			Edges []edge[environment] `json:"edges"`
		} `json:"environments"`
	}
	type projectConnection struct {
		Edges    []edge[project] `json:"edges"`
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	}
	targets := make([]RemoteTarget, 0)
	after := ""
	for {
		var result struct {
			Projects  projectConnection `json:"projects"`
			Workspace struct {
				Projects projectConnection `json:"projects"`
			} `json:"workspace"`
		}
		query := `query($after: String) { projects(first: 100, after: $after) { edges { node { id name services(first: 100) { edges { node { id name } } } environments(first: 100) { edges { node { id name } } } } } pageInfo { hasNextPage endCursor } } }`
		variables := map[string]any{"after": after}
		if c.workspaceID != "" {
			query = `query($workspaceId: String!, $after: String) { workspace(workspaceId: $workspaceId) { projects(first: 100, after: $after) { edges { node { id name services(first: 100) { edges { node { id name } } } environments(first: 100) { edges { node { id name } } } } } pageInfo { hasNextPage endCursor } } } }`
			variables["workspaceId"] = c.workspaceID
		}
		if err := c.query(ctx, query, variables, &result); err != nil {
			return nil, err
		}
		projects := result.Projects
		if c.workspaceID != "" {
			projects = result.Workspace.Projects
		}
		for _, projectEdge := range projects.Edges {
			for _, serviceEdge := range projectEdge.Node.Services.Edges {
				for _, environmentEdge := range projectEdge.Node.Environments.Edges {
					targets = append(targets, RemoteTarget{
						ProjectID: projectEdge.Node.ID, ProjectName: projectEdge.Node.Name,
						ServiceID: serviceEdge.Node.ID, ServiceName: serviceEdge.Node.Name,
						EnvironmentID: environmentEdge.Node.ID, EnvironmentName: environmentEdge.Node.Name,
					})
				}
			}
		}
		if !projects.PageInfo.HasNextPage || projects.PageInfo.EndCursor == "" {
			break
		}
		after = projects.PageInfo.EndCursor
	}
	return targets, nil
}

func railwayMetaCommit(meta map[string]any) string {
	for _, key := range []string{"commitHash", "commitSha", "commitSHA", "githubCommitSha"} {
		if value, ok := meta[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func (c *railwayClient) Fetch(ctx context.Context, binding *deployment_model.Binding) (*Summary, error) {
	type deployment struct {
		ID        string         `json:"id"`
		Status    string         `json:"status"`
		CreatedAt time.Time      `json:"createdAt"`
		StaticURL string         `json:"staticUrl"`
		Meta      map[string]any `json:"meta"`
	}
	type edge struct {
		Node deployment `json:"node"`
	}
	var result struct {
		Deployments struct {
			Edges []edge `json:"edges"`
		} `json:"deployments"`
		DeploymentActive *deployment `json:"deploymentActive"`
	}
	query := `query($projectId: String!, $serviceId: String!, $environmentId: String!) {
  deployments(first: 20, input: {projectId: $projectId, serviceId: $serviceId, environmentId: $environmentId}) { edges { node { id status createdAt staticUrl meta } } }
  deploymentActive(projectId: $projectId, serviceId: $serviceId, environmentId: $environmentId) { id status createdAt staticUrl meta }
}`
	variables := map[string]any{"projectId": binding.ProjectID, "serviceId": binding.ServiceID, "environmentId": binding.EnvironmentID}
	if err := c.query(ctx, query, variables, &result); err != nil {
		return nil, err
	}
	convert := func(item *deployment) *RemoteDeployment {
		if item == nil {
			return nil
		}
		detailURL := safeDeploymentURL(item.StaticURL, fmt.Sprintf("https://railway.com/project/%s/service/%s?environmentId=%s", url.PathEscape(binding.ProjectID), url.PathEscape(binding.ServiceID), url.QueryEscape(binding.EnvironmentID)))
		return &RemoteDeployment{ID: item.ID, CommitSHA: railwayMetaCommit(item.Meta), Status: normalizeRailwayStatus(item.Status), URL: detailURL, Created: item.CreatedAt}
	}
	summary := &Summary{Active: convert(result.DeploymentActive)}
	if len(result.Deployments.Edges) > 0 {
		summary.Latest = convert(&result.Deployments.Edges[0].Node)
	}
	return summary, nil
}

type vercelClient struct {
	token      string
	teamID     string
	httpClient *http.Client
	endpoint   string
}

func (c *vercelClient) get(ctx context.Context, path string, query url.Values, target any) error {
	if c.teamID != "" {
		query.Set("teamId", c.teamID)
	}
	endpoint := c.endpoint + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	resp, err := doRequestWithRateLimitRetry(ctx, c.httpClient, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		return req, err
	})
	if err != nil {
		return err
	}
	return decodeJSONResponse(resp, target)
}

func (c *vercelClient) Test(ctx context.Context) error {
	var result struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	return c.get(ctx, "/v2/user", url.Values{}, &result)
}

type vercelProject struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	CustomEnvironments []struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"customEnvironments"`
}

func (c *vercelClient) Discover(ctx context.Context) ([]RemoteTarget, error) {
	targets := make([]RemoteTarget, 0)
	from := ""
	for {
		query := url.Values{"limit": {"100"}}
		if from != "" {
			query.Set("from", from)
		}
		var result struct {
			Projects   []vercelProject `json:"projects"`
			Pagination struct {
				Next int64 `json:"next"`
			} `json:"pagination"`
		}
		if err := c.get(ctx, "/v9/projects", query, &result); err != nil {
			return nil, err
		}
		for _, project := range result.Projects {
			for _, environment := range []string{"production", "preview"} {
				targets = append(targets, RemoteTarget{ProjectID: project.ID, ProjectName: project.Name, EnvironmentID: environment, EnvironmentName: environment})
			}
			for _, environment := range project.CustomEnvironments {
				name := environment.Name
				if name == "" {
					name = environment.Slug
				}
				targets = append(targets, RemoteTarget{ProjectID: project.ID, ProjectName: project.Name, EnvironmentID: environment.ID, EnvironmentName: name})
			}
		}
		if result.Pagination.Next == 0 {
			break
		}
		from = strconv.FormatInt(result.Pagination.Next, 10)
	}
	return targets, nil
}

type vercelDeployment struct {
	UID        string `json:"uid"`
	URL        string `json:"url"`
	State      string `json:"state"`
	ReadyState string `json:"readyState"`
	Created    int64  `json:"created"`
	Meta       struct {
		GitCommitSHA       string `json:"gitCommitSha"`
		GithubCommitSHA    string `json:"githubCommitSha"`
		GitlabCommitSHA    string `json:"gitlabCommitSha"`
		BitbucketCommitSHA string `json:"bitbucketCommitSha"`
		GitCommitRef       string `json:"gitCommitRef"`
		GithubCommitRef    string `json:"githubCommitRef"`
	} `json:"meta"`
}

func (d *vercelDeployment) commitSHA() string {
	for _, value := range []string{d.Meta.GitCommitSHA, d.Meta.GithubCommitSHA, d.Meta.GitlabCommitSHA, d.Meta.BitbucketCommitSHA} {
		if value != "" {
			return value
		}
	}
	return ""
}

func (d *vercelDeployment) branch() string {
	if d.Meta.GitCommitRef != "" {
		return d.Meta.GitCommitRef
	}
	return d.Meta.GithubCommitRef
}

func (c *vercelClient) Fetch(ctx context.Context, binding *deployment_model.Binding) (*Summary, error) {
	baseQuery := url.Values{"projectId": {binding.ProjectID}, "limit": {"100"}}
	if binding.EnvironmentID == "production" || binding.EnvironmentID == "preview" {
		baseQuery.Set("target", binding.EnvironmentID)
	} else {
		baseQuery.Set("customEnvironmentId", binding.EnvironmentID)
	}
	if binding.BranchFilter != "" {
		baseQuery.Set("meta-gitCommitRef", binding.BranchFilter)
	}
	convert := func(item *vercelDeployment) *RemoteDeployment {
		if item == nil {
			return nil
		}
		status := item.ReadyState
		if status == "" {
			status = item.State
		}
		deploymentURL := safeDeploymentURL(item.URL, "")
		return &RemoteDeployment{ID: item.UID, CommitSHA: item.commitSHA(), Status: normalizeVercelStatus(status), URL: deploymentURL, Created: time.UnixMilli(item.Created)}
	}
	summary := &Summary{}
	from := ""
	for {
		query := url.Values{}
		for key, values := range baseQuery {
			query[key] = append([]string(nil), values...)
		}
		if from != "" {
			query.Set("from", from)
		}
		var result struct {
			Deployments []vercelDeployment `json:"deployments"`
			Pagination  struct {
				Next int64 `json:"next"`
			} `json:"pagination"`
		}
		if err := c.get(ctx, "/v6/deployments", query, &result); err != nil {
			return nil, err
		}
		for i := range result.Deployments {
			item := &result.Deployments[i]
			if binding.BranchFilter != "" && item.branch() != binding.BranchFilter {
				continue
			}
			if summary.Latest == nil {
				summary.Latest = convert(item)
			}
			if summary.Active == nil && normalizeVercelStatus(item.ReadyState) == deployment_model.StatusSuccess {
				summary.Active = convert(item)
			}
		}
		if summary.Active != nil || result.Pagination.Next == 0 {
			break
		}
		from = strconv.FormatInt(result.Pagination.Next, 10)
	}
	return summary, nil
}
