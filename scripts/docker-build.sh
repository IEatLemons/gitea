#!/usr/bin/env bash
# 一键构建 Gitea 镜像，并更新仓库根目录 VERSION。
#
# 镜像内二进制版本：Dockerfile 中 `make backend` 会按 Makefile 规则读取根目录 VERSION
#（STORED_VERSION_FILE），无需再传 GITEA_VERSION build-arg；本脚本在构建前写入 VERSION，
# 是为让编译进二进制的版本号与镜像 tag（ieatlemon/gitea: x.x.x）一致。
#
# 用法:
#   ./scripts/docker-build.sh              # VERSION patch +1 后构建
#   ./scripts/docker-build.sh 1.0.7       # 指定版本（可写 V1.0.7）
# 环境变量:
#   DOCKER_REGISTRY          默认 ieatlemon
#   DOCKER_IMAGE_NAME        默认 gitea（完整镜像为 ${REGISTRY}/${NAME}）
#   DOCKER_FILE              默认 Dockerfile；可设为 Dockerfile.rootless
#   DOCKER_PLATFORM          默认 linux/amd64（与 Railway 等云一致；本地可设 linux/arm64 等）
#   DOCKER_PLATFORM_NATIVE=1 不传 --platform，按本机架构构建（更快，勿用于部署到 amd64 云）
#   DOCKER_TAG_LATEST        设为 0 则不打 :latest（默认打版本号 + latest）
#   DOCKER_PUSH              设为 0 则构建后不推送（默认推送；需先 docker login）
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION_FILE="$ROOT/VERSION"

normalize_version() {
  local v="${1#V}"
  v="${v#v}"
  echo "$v" | tr -d ' \n\r\t'
}

read_version_file() {
  if [[ -f "$VERSION_FILE" ]]; then
    normalize_version "$(cat "$VERSION_FILE")"
  else
    echo "1.0.0"
  fi
}

bump_patch() {
  local v="$1"
  if [[ ! "$v" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "error: VERSION 须为 MAJOR.MINOR.PATCH，当前为: $v" >&2
    exit 1
  fi
  local major minor patch
  IFS=. read -r major minor patch <<<"$v"
  echo "$major.$minor.$((patch + 1))"
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  sed -n '2,12p' "$0" | sed 's/^# //'
  exit 0
fi

if [[ $# -ge 1 ]]; then
  NEW_VERSION="$(normalize_version "$1")"
else
  NEW_VERSION="$(bump_patch "$(read_version_file)")"
fi

if [[ ! "$NEW_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: 非法版本号: $NEW_VERSION" >&2
  exit 1
fi

printf '%s\n' "$NEW_VERSION" >"$VERSION_FILE"

REGISTRY="${DOCKER_REGISTRY:-ieatlemon}"
IMAGE_NAME="${DOCKER_IMAGE_NAME:-gitea}"
DOCKER_FILE="${DOCKER_FILE:-Dockerfile}"

IMAGE="${REGISTRY}/${IMAGE_NAME}:${NEW_VERSION}"
IMAGE_LATEST="${REGISTRY}/${IMAGE_NAME}:latest"

PLATFORM_ARGS=()
if [[ "${DOCKER_PLATFORM_NATIVE:-0}" == "1" ]]; then
  :
else
  PLATFORM_ARGS=(--platform "${DOCKER_PLATFORM:-linux/amd64}")
fi

TAG_ARGS=(-t "$IMAGE")
if [[ "${DOCKER_TAG_LATEST:-1}" != "0" ]]; then
  TAG_ARGS+=(-t "$IMAGE_LATEST")
fi

echo "=> VERSION=${NEW_VERSION}（已写入 $VERSION_FILE）"
if [[ "${DOCKER_PLATFORM_NATIVE:-0}" == "1" ]]; then
  echo "=> 目标平台: 本机（DOCKER_PLATFORM_NATIVE=1，未指定 --platform）"
else
  echo "=> 目标平台: ${DOCKER_PLATFORM:-linux/amd64}"
fi
echo "=> Dockerfile: $DOCKER_FILE"
if [[ "${DOCKER_TAG_LATEST:-1}" != "0" ]]; then
  echo "=> 构建: $IMAGE 与 $IMAGE_LATEST"
else
  echo "=> 构建: $IMAGE"
fi

docker_build() {
  command docker build "$@"
}

docker_build "${PLATFORM_ARGS[@]}" \
  -f "$ROOT/$DOCKER_FILE" \
  "${TAG_ARGS[@]}" \
  "$ROOT"

echo "=> 完成: $IMAGE"
if [[ "${DOCKER_TAG_LATEST:-1}" != "0" ]]; then
  echo "=> 完成: $IMAGE_LATEST"
fi

if [[ "${DOCKER_PUSH:-1}" != "0" ]]; then
  echo "=> 推送 $IMAGE"
  command docker push "$IMAGE"
  if [[ "${DOCKER_TAG_LATEST:-1}" != "0" ]]; then
    echo "=> 推送 $IMAGE_LATEST"
    command docker push "$IMAGE_LATEST"
  fi
  echo "=> 推送完成"
fi
