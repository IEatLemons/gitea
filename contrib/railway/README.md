# 在 Railway 上部署 Gitea

本文说明如何避免「重新部署后无法读取 Git 数据」以及如何恢复。**根本原因**通常是：未把官方镜像使用的数据目录挂载到 Railway **持久卷**，容器重建后磁盘上的裸仓库（`/data/git/repositories`）丢失，而数据库里仍有仓库记录。

官方镜像（本仓库 [`Dockerfile`](/Dockerfile)）约定：

| 路径 | 用途 |
|------|------|
| `/data/git/repositories` | Git 裸仓库（由 [`docker/root/etc/templates/app.ini`](/docker/root/etc/templates/app.ini) 中 `[repository] ROOT` 指定） |
| `/data/gitea` | `APP_DATA_PATH`：配置、日志、附件、索引器、默认 SQLite 数据库路径等 |
| `/data/git/lfs` | LFS 对象 |

[`ENTRYPOINT`](/docker/root/usr/bin/entrypoint) 会创建 `/data/gitea/conf`、`/data/git` 等目录。持久化时应挂载**整块** `/data`，而不是只挂子目录。

## 1. 持久卷（必做）

Railway 的卷挂载目前需在 **控制台** 创建并绑定服务，无法在 `railway.toml` 内声明挂载（参见 [Railway Volumes](https://docs.railway.com/guides/volumes)）。

1. 在 Railway 中为 Gitea 服务 **添加 Volume**。
2. 将卷的挂载路径设为 **`/data`**（与镜像声明的 [`VOLUME ["/data"]`](/Dockerfile) 一致）。
3. 重新部署后确认同一卷仍挂在 `/data`。

若使用托管数据库（如 Railway Postgres），仍需上述卷以保存 **Git 仓库文件与 LFS**；仅数据库持久化无法替代磁盘上的 `*.git` 目录。

推荐环境变量（与仓库根目录 [`railway.toml`](/railway.toml) 一致）：

- `GITEA__server__HTTP_PORT` = `${{PORT}}`
- `GITEA__server__ROOT_URL` = `https://${{RAILWAY_PUBLIC_DOMAIN}}/`

若需要在 Railway 上跳过网页安装流程，可以通过环境变量提供初始管理员账号。数据库配置必须也来自环境变量，例如 `GITEA__database__DB_TYPE` 和 `GITEA__database__CONN_STR`：

- `GITEA_INSTALL_ADMIN_NAME` = `admin`
- `GITEA_INSTALL_ADMIN_PASSWORD` = `<strong-password>`
- `GITEA_INSTALL_ADMIN_EMAIL` = `admin@example.com`

当实例尚未安装且数据库与管理员环境变量都完整时，启动流程会自动完成首次安装并创建管理员。若 `app.ini` 已存在但 Postgres 被重置为空库，正常启动后也会在没有任何用户时自动补建该管理员。完成初始化后建议删除 `GITEA_INSTALL_ADMIN_PASSWORD`，或至少轮换管理员密码。

若自行修改了 `APP_DATA_PATH` 或 `[repository] ROOT`，卷挂载点必须覆盖这些路径所在的父目录。

## 2. 数据已丢失时的恢复

若已有仓库页面提示「无法读取此仓库下的 Git 数据」，对应逻辑见源码 [`services/context/repo.go`](../../services/context/repo.go)（打开仓库目录失败且报「不存在」类错误）。

可选做法：

1. **有卷备份**：将备份还原到挂载在 `/data` 的卷后重启服务。
2. **无备份**：在 Gitea 中删除损坏的仓库记录（或通过管理员界面删除仓库），重新创建空仓库，再从本地或其他远端执行 `git push` / `git push --mirror` 推回历史。
3. **误用容器内 SQLite**：若数据库文件也在未持久化的磁盘上，还会出现用户、仓库记录一并丢失；生产环境建议使用托管数据库 **并把 `/data` 挂卷**，或至少保证 `gitea.db` 与 `repositories` 同在持久路径下。

排查时可查看日志中的 `broken repository on the file system`。

## 3. 日常迭代与 Redeploy 的区别

| 操作 | 是否需要对 Gitea 做 Railway Redeploy |
|------|--------------------------------------|
| 本地修改代码后 `git push` 到本实例 | **不需要**。由 Gitea 的 Git 服务写入 `/data/git/repositories`。 |
| 更换镜像、改环境变量、平台强制换新容器 | **需要** Redeploy；若未挂卷，仓库文件会再次丢失。 |

若「每次改代码都要 Redeploy」，一般是把 **应用构建/部署流水线** 与 **向 Gitea 推送代码** 混在了一起，或 CI 在 push 时触发了别的服务的构建；应对 **Gitea** 与 **被部署的业务应用** 分开配置触发条件。

---

更多配置项见 [Config Cheat Sheet](https://docs.gitea.com/administration/config-cheat-sheet)。
