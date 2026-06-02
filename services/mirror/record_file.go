// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package mirror

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"text/template"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/log"
	repo_module "code.gitea.io/gitea/modules/repository"
)

const DefaultRecordFileCommitMessage = "chore: update mirror record"

type recordFileTemplateData struct {
	Repo          string
	RemoteName    string
	RemoteAddress string
	Branch        string
	OldCommit     string
	Timestamp     string
	TriggerType   string
}

func normalizeRecordFileCommitMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return DefaultRecordFileCommitMessage
	}
	return msg
}

func cleanRecordFilePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("record file path is required")
	}
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("record file path must be relative")
	}
	clean := path.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("record file path must stay inside the repository")
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".git" {
			return "", fmt.Errorf("record file path cannot be inside .git")
		}
	}
	return clean, nil
}

// ApplyRecordFileFromForm applies record-file commit settings from API or web forms.
func ApplyRecordFileFromForm(m *repo_model.PushMirror, enabled bool, branches, filePath, contentTemplate, authorName, authorEmail, commitMessage string) error {
	branchList, err := ParseMirrorBranches(branches)
	if err != nil {
		return err
	}
	m.RecordFileEnabled = enabled
	m.RecordFileBranches = JoinMirrorBranches(branchList)
	m.RecordFilePath = strings.TrimSpace(filePath)
	if m.RecordFilePath != "" {
		cleanPath, err := cleanRecordFilePath(m.RecordFilePath)
		if err != nil {
			return err
		}
		m.RecordFilePath = cleanPath
	}
	if enabled && m.RecordFilePath == "" {
		return fmt.Errorf("record file path is required")
	}
	if _, err := template.New("record-file").Option("missingkey=error").Parse(contentTemplate); err != nil {
		return err
	}
	m.RecordFileTemplate = contentTemplate
	m.RecordFileAuthorName = strings.TrimSpace(authorName)
	m.RecordFileAuthorEmail = strings.TrimSpace(authorEmail)
	if enabled && (m.RecordFileAuthorName == "" || m.RecordFileAuthorEmail == "") {
		return fmt.Errorf("record file author name and email are required")
	}
	m.RecordFileCommitMessage = normalizeRecordFileCommitMessage(commitMessage)
	return nil
}

func recordFileBranches(ctx context.Context, m *repo_model.PushMirror) ([]string, error) {
	if strings.TrimSpace(m.RecordFileBranches) != "" {
		return ParseMirrorBranches(m.RecordFileBranches)
	}
	if strings.TrimSpace(m.MirrorBranches) != "" {
		return ParseMirrorBranches(m.MirrorBranches)
	}
	repo := m.GetRepository(ctx)
	if repo == nil || strings.TrimSpace(repo.DefaultBranch) == "" {
		return nil, nil
	}
	return []string{repo.DefaultBranch}, nil
}

func renderRecordFile(ctx context.Context, m *repo_model.PushMirror, branch, oldCommit, triggerType string) ([]byte, error) {
	tmpl, err := template.New("record-file").Option("missingkey=error").Parse(m.RecordFileTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, recordFileTemplateData{
		Repo:          m.GetRepository(ctx).FullName(),
		RemoteName:    m.RemoteName,
		RemoteAddress: m.RemoteAddress,
		Branch:        branch,
		OldCommit:     oldCommit,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		TriggerType:   triggerType,
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func commitRecordFileToBranch(ctx context.Context, repo *repo_model.Repository, m *repo_model.PushMirror, branch, triggerType string) error {
	gitRepo, err := gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		return err
	}
	defer gitRepo.Close()

	oldHead, err := gitRepo.GetBranchCommitID(branch)
	if err != nil {
		return err
	}
	headCommit, err := gitRepo.GetCommit(oldHead)
	if err != nil {
		return err
	}
	content, err := renderRecordFile(ctx, m, branch, oldHead, triggerType)
	if err != nil {
		return err
	}
	blobID, err := gitRepo.HashObjectBytes(content)
	if err != nil {
		return err
	}

	indexFile, workTree, cancel, err := gitRepo.ReadTreeToTemporaryIndex(headCommit.ID.String())
	if err != nil {
		return err
	}
	defer cancel()
	env := append(os.Environ(), "GIT_INDEX_FILE="+indexFile, "GIT_WORK_TREE="+workTree)
	indexInput := []byte("100644 blob " + blobID.String() + "\t" + m.RecordFilePath + "\000")
	if err := gitcmd.NewCommand("update-index", "--add", "--replace", "-z", "--index-info").
		WithDir(gitRepo.Path).
		WithEnv(env).
		WithStdinBytes(indexInput).
		RunWithStderr(ctx); err != nil {
		return err
	}
	treeID, _, err := gitcmd.NewCommand("write-tree").
		WithDir(gitRepo.Path).
		WithEnv(env).
		RunStdString(ctx)
	if err != nil {
		return err
	}
	tree, err := gitRepo.GetTree(strings.TrimSpace(treeID))
	if err != nil {
		return err
	}

	author := &git.Signature{Name: m.RecordFileAuthorName, Email: m.RecordFileAuthorEmail}
	newID, err := gitRepo.CommitTree(author, author, tree, git.CommitTreeOpts{
		Parents:   []string{headCommit.ID.String()},
		Message:   normalizeRecordFileCommitMessage(m.RecordFileCommitMessage),
		NoGPGSign: true,
	})
	if err != nil {
		return err
	}
	refName := git.RefNameFromBranch(branch).String()
	if _, _, err := gitcmd.NewCommand("update-ref").
		AddDynamicArguments(refName, newID.String(), oldHead).
		WithDir(gitRepo.Path).
		RunStdString(ctx); err != nil {
		return err
	}
	return nil
}

// MaybeRecordFileBeforePush overwrites configured record files before pushing a mirror.
func MaybeRecordFileBeforePush(ctx context.Context, m *repo_model.PushMirror, triggerType string) error {
	if m == nil || !m.RecordFileEnabled {
		return nil
	}
	repo := m.GetRepository(ctx)
	if repo == nil {
		return nil
	}
	if _, err := cleanRecordFilePath(m.RecordFilePath); err != nil {
		return err
	}
	branches, err := recordFileBranches(ctx, m)
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if err := commitRecordFileToBranch(ctx, repo, m, branch, triggerType); err != nil {
			return err
		}
	}
	if _, err := repo_module.SyncRepoBranches(ctx, repo.ID, 0); err != nil {
		log.Error("RecordFile: SyncRepoBranches[%s]: %v", repo.FullName(), err)
	}
	if err := repo_module.UpdateRepoSize(ctx, repo); err != nil {
		log.Error("RecordFile: UpdateRepoSize[%s]: %v", repo.FullName(), err)
	}
	return nil
}
