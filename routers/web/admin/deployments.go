// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package admin

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	deployment_model "code.gitea.io/gitea/models/deployment"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/services/context"
	deployment_service "code.gitea.io/gitea/services/deployment"
)

const tplAdminDeployments templates.TplName = "admin/deployments"

type deploymentBindingView struct {
	Binding        *deployment_model.Binding
	RepoName       string
	Provider       deployment_model.Provider
	ConnectionName string
}

type deploymentTargetView struct {
	Value string
	Label string
}

func deploymentAdminURL() string {
	return setting.AppSubURL + "/-/admin/deployments"
}

func renderDeployments(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.deployments.title")
	ctx.Data["PageIsAdminDeployments"] = true
	connections, err := deployment_model.ListConnections(ctx)
	if err != nil {
		ctx.ServerError("ListDeploymentConnections", err)
		return
	}
	ctx.Data["DeploymentConnections"] = connections
	connectionMap := make(map[int64]*deployment_model.Connection, len(connections))
	for _, connection := range connections {
		connectionMap[connection.ID] = connection
	}
	bindings, err := deployment_model.ListBindings(ctx)
	if err != nil {
		ctx.ServerError("ListDeploymentBindings", err)
		return
	}
	bindingViews := make([]deploymentBindingView, 0, len(bindings))
	for _, binding := range bindings {
		repo, err := repo_model.GetRepositoryByID(ctx, binding.RepoID)
		if err != nil {
			continue
		}
		if err := repo.LoadOwner(ctx); err != nil {
			continue
		}
		view := deploymentBindingView{Binding: binding, RepoName: repo.FullName()}
		if connection := connectionMap[binding.ConnectionID]; connection != nil {
			view.Provider = connection.Provider
			view.ConnectionName = connection.Name
		}
		bindingViews = append(bindingViews, view)
	}
	ctx.Data["DeploymentBindings"] = bindingViews

	selectedID, _ := strconv.ParseInt(ctx.FormString("connection_id"), 10, 64)
	if selectedID > 0 {
		connection := connectionMap[selectedID]
		if connection != nil {
			ctx.Data["SelectedDeploymentConnection"] = connection
			targets, err := deployment_service.Discover(ctx, connection)
			if err != nil {
				ctx.Data["DiscoveryError"] = err.Error()
			} else {
				targetViews := make([]deploymentTargetView, 0, len(targets))
				for _, target := range targets {
					targetViews = append(targetViews, deploymentTargetView{Value: encodeDeploymentTarget(target), Label: deploymentTargetLabel(target)})
				}
				ctx.Data["DeploymentTargets"] = targetViews
			}
		}
	}
	ctx.HTML(http.StatusOK, tplAdminDeployments)
}

// Deployments renders deployment platform connections and repository bindings.
func Deployments(ctx *context.Context) {
	renderDeployments(ctx)
}

// CreateDeploymentConnection validates and stores a new platform connection.
func CreateDeploymentConnection(ctx *context.Context) {
	provider := deployment_model.Provider(strings.TrimSpace(ctx.FormString("provider")))
	connection := &deployment_model.Connection{
		Provider:       provider,
		Name:           strings.TrimSpace(ctx.FormString("name")),
		ScopeID:        strings.TrimSpace(ctx.FormString("scope_id")),
		IsActive:       true,
		LastSyncStatus: deployment_model.StatusUnknown,
	}
	if !provider.IsValid() || connection.Name == "" {
		ctx.Flash.Error(ctx.Tr("admin.deployments.invalid_connection"))
		ctx.Redirect(deploymentAdminURL())
		return
	}
	if err := connection.SetToken(ctx.FormString("token")); err != nil {
		ctx.Flash.Error(ctx.Tr("admin.deployments.invalid_connection"))
		ctx.Redirect(deploymentAdminURL())
		return
	}
	if connection.IsActive {
		if err := deployment_service.TestConnection(ctx, connection); err != nil {
			ctx.Flash.Error(ctx.Tr("admin.deployments.test_failed", err))
			ctx.Redirect(deploymentAdminURL())
			return
		}
	}
	if err := deployment_model.InsertConnection(ctx, connection); err != nil {
		ctx.Flash.Error(ctx.Tr("admin.deployments.save_failed", err))
		ctx.Redirect(deploymentAdminURL())
		return
	}
	ctx.Flash.Success(ctx.Tr("admin.deployments.connection_created"))
	ctx.Redirect(deploymentAdminURL())
}

// UpdateDeploymentConnection updates and validates a platform connection.
func UpdateDeploymentConnection(ctx *context.Context) {
	connection, err := deployment_model.GetConnectionByID(ctx, ctx.PathParamInt64("id"))
	if err != nil {
		ctx.NotFound(err)
		return
	}
	connection.Name = strings.TrimSpace(ctx.FormString("name"))
	connection.ScopeID = strings.TrimSpace(ctx.FormString("scope_id"))
	connection.IsActive = ctx.FormBool("is_active")
	updateToken := strings.TrimSpace(ctx.FormString("token")) != ""
	if updateToken {
		if err := connection.SetToken(ctx.FormString("token")); err != nil {
			ctx.Flash.Error(ctx.Tr("admin.deployments.invalid_connection"))
			ctx.Redirect(deploymentAdminURL())
			return
		}
	}
	if connection.Name == "" {
		ctx.Flash.Error(ctx.Tr("admin.deployments.invalid_connection"))
		ctx.Redirect(deploymentAdminURL())
		return
	}
	if connection.IsActive {
		if err := deployment_service.TestConnection(ctx, connection); err != nil {
			ctx.Flash.Error(ctx.Tr("admin.deployments.test_failed", err))
			ctx.Redirect(deploymentAdminURL())
			return
		}
	}
	if err := deployment_model.UpdateConnection(ctx, connection, updateToken); err != nil {
		ctx.Flash.Error(ctx.Tr("admin.deployments.save_failed", err))
	} else {
		ctx.Flash.Success(ctx.Tr("admin.deployments.connection_updated"))
	}
	ctx.Redirect(deploymentAdminURL())
}

// TestDeploymentConnection validates an existing platform credential.
func TestDeploymentConnection(ctx *context.Context) {
	connection, err := deployment_model.GetConnectionByID(ctx, ctx.PathParamInt64("id"))
	if err != nil {
		ctx.NotFound(err)
		return
	}
	if err := deployment_service.TestConnection(ctx, connection); err != nil {
		ctx.Flash.Error(ctx.Tr("admin.deployments.test_failed", err))
	} else {
		ctx.Flash.Success(ctx.Tr("admin.deployments.test_success"))
	}
	ctx.Redirect(deploymentAdminURL())
}

// SyncDeploymentConnection queues a manual platform synchronization.
func SyncDeploymentConnection(ctx *context.Context) {
	if _, err := deployment_model.GetConnectionByID(ctx, ctx.PathParamInt64("id")); err != nil {
		ctx.NotFound(err)
		return
	}
	if err := deployment_service.EnqueueConnection(ctx.PathParamInt64("id")); err != nil {
		ctx.Flash.Error(ctx.Tr("admin.deployments.sync_failed", err))
	} else {
		ctx.Flash.Success(ctx.Tr("admin.deployments.sync_started"))
	}
	ctx.Redirect(deploymentAdminURL())
}

// DeleteDeploymentConnection deletes a connection and its local deployment data.
func DeleteDeploymentConnection(ctx *context.Context) {
	if err := deployment_model.DeleteConnection(ctx, ctx.PathParamInt64("id")); err != nil {
		ctx.Flash.Error(ctx.Tr("admin.deployments.delete_failed", err))
	} else {
		ctx.Flash.Success(ctx.Tr("admin.deployments.connection_deleted"))
	}
	ctx.Redirect(deploymentAdminURL())
}

func encodeDeploymentTarget(target deployment_service.RemoteTarget) string {
	parts := []string{target.ProjectID, target.ProjectName, target.ServiceID, target.ServiceName, target.EnvironmentID, target.EnvironmentName}
	for i := range parts {
		parts[i] = url.QueryEscape(parts[i])
	}
	return strings.Join(parts, "|")
}

func decodeDeploymentTarget(value string) (deployment_service.RemoteTarget, error) {
	parts := strings.Split(value, "|")
	if len(parts) != 6 {
		return deployment_service.RemoteTarget{}, errors.New("invalid deployment target")
	}
	for i := range parts {
		decoded, err := url.QueryUnescape(parts[i])
		if err != nil {
			return deployment_service.RemoteTarget{}, err
		}
		parts[i] = decoded
	}
	return deployment_service.RemoteTarget{ProjectID: parts[0], ProjectName: parts[1], ServiceID: parts[2], ServiceName: parts[3], EnvironmentID: parts[4], EnvironmentName: parts[5]}, nil
}

func deploymentTargetLabel(target deployment_service.RemoteTarget) string {
	parts := []string{target.ProjectName}
	if target.ServiceName != "" {
		parts = append(parts, target.ServiceName)
	}
	parts = append(parts, target.EnvironmentName)
	return strings.Join(parts, " / ")
}

// CreateDeploymentBinding validates and stores a remote target mapping.
func CreateDeploymentBinding(ctx *context.Context) {
	connection, err := deployment_model.GetConnectionByID(ctx, ctx.PathParamInt64("id"))
	if err != nil {
		ctx.NotFound(err)
		return
	}
	target, err := decodeDeploymentTarget(ctx.FormString("target"))
	if err != nil {
		ctx.Flash.Error(ctx.Tr("admin.deployments.invalid_binding"))
		ctx.Redirect(deploymentAdminURL())
		return
	}
	targets, err := deployment_service.Discover(ctx, connection)
	if err != nil {
		ctx.Flash.Error(ctx.Tr("admin.deployments.discovery_failed", err))
		ctx.Redirect(deploymentAdminURL())
		return
	}
	validTarget := false
	for _, candidate := range targets {
		if encodeDeploymentTarget(candidate) == encodeDeploymentTarget(target) {
			validTarget = true
			break
		}
	}
	if !validTarget {
		ctx.Flash.Error(ctx.Tr("admin.deployments.invalid_binding"))
		ctx.Redirect(deploymentAdminURL())
		return
	}
	repoParts := strings.Split(strings.TrimSpace(ctx.FormString("repository")), "/")
	if len(repoParts) != 2 {
		ctx.Flash.Error(ctx.Tr("admin.deployments.invalid_repository"))
		ctx.Redirect(deploymentAdminURL())
		return
	}
	repo, err := repo_model.GetRepositoryByOwnerAndName(ctx, repoParts[0], repoParts[1])
	if err != nil {
		ctx.Flash.Error(ctx.Tr("admin.deployments.invalid_repository"))
		ctx.Redirect(deploymentAdminURL())
		return
	}
	displayName := strings.TrimSpace(ctx.FormString("display_name"))
	if displayName == "" {
		displayName = target.EnvironmentName
	}
	binding := &deployment_model.Binding{
		ConnectionID: connection.ID, RepoID: repo.ID,
		ProjectID: target.ProjectID, ProjectName: target.ProjectName,
		ServiceID: target.ServiceID, ServiceName: target.ServiceName,
		EnvironmentID: target.EnvironmentID, EnvironmentName: target.EnvironmentName,
		BranchFilter: strings.TrimSpace(ctx.FormString("branch_filter")), DisplayName: displayName,
		IsActive: true, RemoteValid: true,
	}
	if connection.Provider == deployment_model.ProviderVercel && target.EnvironmentID == "preview" && binding.BranchFilter == "" {
		ctx.Flash.Error(ctx.Tr("admin.deployments.preview_branch_required"))
		ctx.Redirect(deploymentAdminURL() + "?connection_id=" + strconv.FormatInt(connection.ID, 10))
		return
	}
	if err := deployment_model.InsertBinding(ctx, binding); err != nil {
		ctx.Flash.Error(ctx.Tr("admin.deployments.save_failed", err))
	} else {
		ctx.Flash.Success(ctx.Tr("admin.deployments.binding_created"))
		_ = deployment_service.EnqueueConnection(connection.ID)
	}
	ctx.Redirect(deploymentAdminURL())
}

// DeleteDeploymentBinding deletes one repository deployment mapping.
func DeleteDeploymentBinding(ctx *context.Context) {
	if err := deployment_model.DeleteBinding(ctx, ctx.PathParamInt64("id")); err != nil {
		ctx.Flash.Error(ctx.Tr("admin.deployments.delete_failed", err))
	} else {
		ctx.Flash.Success(ctx.Tr("admin.deployments.binding_deleted"))
	}
	ctx.Redirect(deploymentAdminURL())
}
