// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"net/http"
	"strings"

	"code.gitea.io/gitea/models/organization"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/convert"
)

const navbarQuickAccessDefaultLimit = 8

type navbarOrganization struct {
	Name       string `json:"name"`
	FullName   string `json:"full_name"`
	NumRepos   int    `json:"num_repos"`
	Visibility string `json:"visibility"`
	Link       string `json:"link"`
}

type navbarOrganizationResults struct {
	OK   bool                 `json:"ok"`
	Data []navbarOrganization `json:"data"`
}

// NavbarOrganizations returns organizations for the signed-in user's navbar quick access menu.
func NavbarOrganizations(ctx *context.Context) {
	limit := ctx.FormInt("limit")
	if limit <= 0 {
		limit = navbarQuickAccessDefaultLimit
	}
	limit = convert.ToCorrectPageSize(limit)

	query := strings.ToLower(ctx.FormTrim("q"))

	orgs, err := organization.GetUserOrgsList(ctx, ctx.Doer)
	if err != nil {
		ctx.ServerError("GetUserOrgsList", err)
		return
	}

	results := make([]navbarOrganization, 0, min(len(orgs), limit))
	var total int64
	for _, org := range orgs {
		if query != "" && !strings.Contains(strings.ToLower(org.Name), query) && !strings.Contains(strings.ToLower(org.FullName), query) {
			continue
		}

		total++
		if len(results) >= limit {
			continue
		}

		results = append(results, navbarOrganization{
			Name:       org.Name,
			FullName:   org.FullName,
			NumRepos:   org.NumRepos,
			Visibility: org.Visibility.String(),
			Link:       org.OrganisationLink(),
		})
	}

	ctx.SetTotalCountHeader(total)
	ctx.JSON(http.StatusOK, navbarOrganizationResults{
		OK:   true,
		Data: results,
	})
}
