// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path"
	"strings"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

func TestGoGet(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	req := NewRequest(t, "GET", "/blah/glah/plah?go-get=1")
	resp := MakeRequest(t, req, http.StatusOK)

	expected := fmt.Sprintf(`<!doctype html>
<html>
	<head>
		<meta name="go-import" content="%[1]s:%[2]s/blah/glah git %[3]sblah/glah.git">
		<meta name="go-source" content="%[1]s:%[2]s/blah/glah _ %[3]sblah/glah/src/branch/master{/dir} %[3]sblah/glah/src/branch/master{/dir}/{file}#L{line}">
	</head>
	<body>
		go get --insecure %[1]s:%[2]s/blah/glah
	</body>
</html>`, setting.Domain, setting.HTTPPort, setting.AppURL)

	assert.Equal(t, expected, resp.Body.String())
}

func TestGoGetForSSH(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	old := setting.Repository.GoGetCloneURLProtocol
	defer func() {
		setting.Repository.GoGetCloneURLProtocol = old
	}()
	setting.Repository.GoGetCloneURLProtocol = "ssh"

	ctx := t.Context()
	ownerName := "blah"
	repoName := "glah"
	trimmedRepoName := strings.TrimSuffix(repoName, ".git")
	branchName := setting.Repository.DefaultBranch
	prefix := setting.AppURL + path.Join(url.PathEscape(ownerName), url.PathEscape(repoName), "src", "branch", util.PathEscapeSegments(branchName))

	appURL, _ := url.Parse(setting.AppURL)
	insecure := ""
	if appURL.Scheme == string(setting.HTTP) {
		insecure = "--insecure "
	}

	goGetImport := context.ComposeGoGetImport(ctx, ownerName, trimmedRepoName)
	cloneURL := repo_model.ComposeSSHCloneURL(ctx, nil, ownerName, repoName)
	goImportContent := fmt.Sprintf("%s git %s", goGetImport, cloneURL)
	goSourceContent := fmt.Sprintf("%s _ %s %s", goGetImport, prefix+"{/dir}", prefix+"{/dir}/{file}#L{line}")
	goGetCli := fmt.Sprintf("go get %s%s", insecure, goGetImport)

	expected := fmt.Sprintf(`<!doctype html>
<html>
	<head>
		<meta name="go-import" content="%s">
		<meta name="go-source" content="%s">
	</head>
	<body>
		%s
	</body>
</html>`, html.EscapeString(goImportContent), html.EscapeString(goSourceContent), html.EscapeString(goGetCli))

	req := NewRequest(t, "GET", "/blah/glah/plah?go-get=1")
	resp := MakeRequest(t, req, http.StatusOK)

	assert.Equal(t, expected, resp.Body.String())
}
