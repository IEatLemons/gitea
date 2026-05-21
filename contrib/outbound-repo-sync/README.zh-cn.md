# Gitea 出站同步：系统级权限与分支范围

本文对应站内「推送镜像」实现限制（出站 refspec 固定为全部分支与标签，见源码 [`services/mirror/mirror_push.go`](../../services/mirror/mirror_push.go)），从**权限与分支范围**两点给出可落地的系统级做法；无需在业务仓库里放置 Actions workflow 文件。

**更新：** 仓库管理员可在 **设置 → 镜像 → 推送镜像** 中填写「推送分支」列表（逗号分隔）；留空仍表示镜像全部分支与标签，填写则仅推送所列分支（且不推送标签）。该行为由服务端写入各 `remote.<mirror>.push` refspec，详见同目录源码。

## 1. 确认出站范围（全分支镜像 vs 仅指定分支）

请先选定其一（站点管理员与仓库管理员共同确认即可）。

| 需求 | 推荐做法 | 权限控制要点 |
|------|----------|----------------|
| **允许 GitHub 上与 Gitea 保持「全分支 + 标签」一致** | 使用仓库 **设置 → 镜像 → 推送镜像**，填写 GitHub 远端 URL，开启「提交时同步」类选项（若界面提供）。凭据仅限自动化账号。 | 在 GitHub 侧为该账号下发 **fine-grained PAT（单仓库）** 或 **Deploy Key**，权限收紧到内容写入所需最小集合。 |
| **必须只同步指定分支（例如仅 `develop`）** | **不要**依赖内置推送镜像实现分支裁剪（当前产品行为为推送 `refs/heads/*` 与 `refs/tags/*`）。改用下文 **Webhook + 分支过滤器 + 外部同步服务**。 | Webhook **Secret** 仅在 Gitea 与同步服务之间校验投递；**GitHub 凭据**仅存放在同步服务的密钥管理（环境变量、密钥库），不入库到业务仓库。 |

结论：**若要分支级出站**，请选择「Webhook + 外部同步」路径并完成第 2、3 节。

## 2. 通道选型：Webhook + 外部同步服务与密钥存放

适用于「仅指定分支出站」且不希望在 `.gitea/workflows` / `.github/workflows` 中维护逻辑的场景。

### 2.1 Gitea 侧配置

1. 打开目标仓库：**设置 → Web 钩子 → 添加 Webhook**。
2. **Payload URL**：指向你们运维部署的 HTTPS 接收端（自建微服务、网关后的脚本、Kubernetes Job 触发器等）。
3. **触发事件**：勾选 **推送（Push）**。
4. **分支过滤器**：填写 glob，例如仅 `develop`，或与界面说明一致的 `{develop}` / `{main,develop}` 等形式（参见模板 [`templates/repo/settings/webhook/settings.tmpl`](../../templates/repo/settings/webhook/settings.tmpl)）。
5. **密钥（Secret）**：设置高强度随机串；接收端必须用同一密钥校验签名或比对请求头（具体算法取决于钩子类型与 Gitea 版本文档）。

### 2.2 同步服务职责（建议）

- 校验 Webhook 真实性（Secret / 签名）。
- 解析推送 payload，确认 ref 为期望分支（双重校验，防止配置遗漏）。
- **克隆或抓取** Gitea 仓库（可用限时可用的 CI token、Deploy token，或仅限镜像机器人的只读凭证），执行受限的 `git push`，例如：`git push https://...github... HEAD:develop`。
- **绝不**把 GitHub PAT 写进业务仓库；使用进程环境变量、Kubernetes Secret、Vault 等。

### 2.3 密钥存放位置（摘要）

| 密钥 | 存放位置示例 |
|------|----------------|
| Webhook Secret | Gitea 钩子配置；接收端环境变量或密钥库 |
| 读 Gitea（抓取代码） | 同步服务侧：Deploy Token / 机器账号 PAT（最小 `read`/`pull`） |
| 写 GitHub | 同步服务侧：fine-grained PAT 或 SSH Deploy Key（最小写入目标分支所需权限） |

## 3. GitHub 身份：fine-grained PAT 与 Deploy Key

### 3.1 Fine-grained PAT（HTTPS `git push`）

1. GitHub：**Settings → Developer settings → Personal access tokens → Fine-grained tokens**。
2. **Repository access**：仅选择要被推送的目标仓库。
3. **Permissions**：至少包含对该仓库 **Contents** 的读写（具体以 GitHub 文档为准）；其余权限一律关闭。
4. **Expiration**：按需设置轮转周期。
5. 在同步服务中以 `https://x-access-token:<TOKEN>@github.com/<owner>/<repo>.git` 等形式使用（勿写入镜像日志）。

### 3.2 Deploy Key（SSH）

1. 在同步服务主机生成专用 SSH keypair，`ssh-ed25519` 为宜。
2. 目标 GitHub 仓库：**Settings → Deploy keys → Add**，勾选 **Allow write access**（若需要推送）。
3. 私钥仅存同步服务运行身份可读路径或密钥托管系统。

### 3.3 GitHub 侧分支策略（兜底）

分支保护与 Rulesets 可约束「哪些身份能改哪些分支」，但**无法替代**「不向远端推送多余 ref」——若仍使用全量推送镜像，多余分支仍可能被创建。分支级出站应以推送命令或 refspec **收窄**为主。

---

维护者可依据本节在三项决策上达成一致：**出站范围**、**Webhook + 外部同步拓扑**、**GitHub 最小权限主体**，即可完成系统级出站同步管控。
