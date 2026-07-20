// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

type navbarQuickAccessOrganizationsResponse struct {
	OK   bool `json:"ok"`
	Data []struct {
		Name       string `json:"name"`
		FullName   string `json:"full_name"`
		NumRepos   int    `json:"num_repos"`
		Visibility string `json:"visibility"`
		Link       string `json:"link"`
	} `json:"data"`
}

func TestNavbarQuickAccessOrganizations(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	req := NewRequest(t, "GET", "/-/navbar/organizations")
	resp := MakeRequest(t, req, http.StatusSeeOther)
	assert.Contains(t, resp.Header().Get("Location"), "/user/login")

	session := loginUser(t, "user4")
	req = NewRequest(t, "GET", "/-/navbar/organizations?limit=8")
	resp = session.MakeRequest(t, req, http.StatusOK)
	assert.Equal(t, "1", resp.Header().Get("X-Total-Count"))

	var orgs navbarQuickAccessOrganizationsResponse
	DecodeJSON(t, resp, &orgs)
	assert.True(t, orgs.OK)
	if assert.Len(t, orgs.Data, 1) {
		assert.Equal(t, "org3", orgs.Data[0].Name)
		assert.Equal(t, 2, orgs.Data[0].NumRepos)
		assert.Equal(t, "public", orgs.Data[0].Visibility)
		assert.Equal(t, "/org3", orgs.Data[0].Link)
	}

	req = NewRequest(t, "GET", "/-/navbar/organizations?q=missing&limit=8")
	resp = session.MakeRequest(t, req, http.StatusOK)
	assert.Equal(t, "0", resp.Header().Get("X-Total-Count"))
	DecodeJSON(t, resp, &orgs)
	assert.Empty(t, orgs.Data)

	session = loginUser(t, "user2")
	req = NewRequest(t, "GET", "/-/navbar/organizations?limit=1")
	resp = session.MakeRequest(t, req, http.StatusOK)
	assert.Equal(t, "3", resp.Header().Get("X-Total-Count"))
	DecodeJSON(t, resp, &orgs)
	assert.Len(t, orgs.Data, 1)
}

func TestNavbarQuickAccessTemplateVisibility(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	req := NewRequest(t, "GET", "/")
	resp := MakeRequest(t, req, http.StatusOK)
	AssertHTMLElement(t, NewHTMLParser(t, resp.Body), "#navbar-quick-access", false)

	session := loginUser(t, "user4")
	req = NewRequest(t, "GET", "/")
	resp = session.MakeRequest(t, req, http.StatusOK)
	AssertHTMLElement(t, NewHTMLParser(t, resp.Body), "#navbar-quick-access", true)
}
