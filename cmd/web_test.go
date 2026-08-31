// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
)

func TestAdminPortHandler(t *testing.T) {
	defer test.MockVariableValue(&setting.AppSubURL, "")()

	var handledPath string
	handler := adminPortHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handledPath = req.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))

	t.Run("RootRedirectsToAdmin", func(t *testing.T) {
		handledPath = ""
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		resp := httptest.NewRecorder()

		handler.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusSeeOther, resp.Code)
		assert.Equal(t, "/admin", resp.Header().Get("Location"))
		assert.Empty(t, handledPath)
	})

	for _, testCase := range []struct {
		name string
		path string
	}{
		{name: "AdminAlias", path: "/admin"},
		{name: "AdminPanel", path: "/-/admin"},
		{name: "AdminSubPath", path: "/-/admin/users"},
		{name: "Login", path: "/user/login"},
		{name: "TwoFactor", path: "/user/two_factor"},
		{name: "StaticAsset", path: "/assets/css/index.css"},
		{name: "HealthCheck", path: "/api/healthz"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handledPath = ""
			req := httptest.NewRequest(http.MethodGet, testCase.path, nil)
			resp := httptest.NewRecorder()

			handler.ServeHTTP(resp, req)

			assert.Equal(t, http.StatusNoContent, resp.Code)
			assert.Equal(t, testCase.path, handledPath)
		})
	}

	for _, testCase := range []struct {
		name string
		path string
	}{
		{name: "RepositoryPage", path: "/owner/repo"},
		{name: "PublicAPI", path: "/api/v1/version"},
		{name: "PackageAPI", path: "/api/packages/owner/npm"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handledPath = ""
			req := httptest.NewRequest(http.MethodGet, testCase.path, nil)
			resp := httptest.NewRecorder()

			handler.ServeHTTP(resp, req)

			assert.Equal(t, http.StatusNotFound, resp.Code)
			assert.Empty(t, handledPath)
		})
	}
}
