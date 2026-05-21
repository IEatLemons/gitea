# Outbound repository sync (system-level permissions)

This directory documents **operations-only** guidance for syncing from Gitea to GitHub (or another remote) with minimal privilege and optional branch scope.

**Chinese runbook (recommended for full detail):** [README.zh-cn.md](README.zh-cn.md).

The repository **Settings → Mirror → Push mirrors** page supports an optional comma-separated **Push branches** list. Leave it empty to mirror all branches and tags; set it to restrict outbound refs (tags are not pushed when restricted). Implementation: [`services/mirror/mirror_push.go`](../../services/mirror/mirror_push.go).

Summary:

1. **Scope:** Push Mirror mirrors **all branches and tags** today (`services/mirror/mirror_push.go`). For **branch-specific** outbound sync without workflow YAML in the repo, use **Webhooks + branch filter + an external sync service** holding credentials.
2. **Channel:** Configure a repo webhook with **Branch filter**, HTTPS endpoint, and shared **Secret**; run a small trusted receiver that performs a scoped `git push`.
3. **GitHub principal:** Prefer **fine-grained PAT** (single repo, Contents only as needed) or **SSH Deploy Key** with write access limited by repo settings; rotate and store secrets outside Git.
