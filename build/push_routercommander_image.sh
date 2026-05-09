#!/usr/bin/env bash

set -euo pipefail

ROOT="${BUILD_WORKSPACE_DIRECTORY:-$(cd "$(dirname "$0")/.." && pwd)}"
IMAGE_REPO="${IMAGE_REPO:-docker.io/sbezverk/routercommander}"
IMAGE_TAG="${IMAGE_TAG:-latest}"

build_and_push() {
  local platform="$1"
  local bazel_config="$2"
  local tag_suffix="$3"
  local tmpdir

  cd "${ROOT}"
  bazel build --config="${bazel_config}" //:routercommander --color=no --curses=no

  tmpdir="$(mktemp -d)"
  mkdir -p "${tmpdir}/bin"
  cp "${ROOT}/bazel-bin/cmd/routercommander_/routercommander" "${tmpdir}/bin/routercommander"
  cp -R "${ROOT}/testdata" "${tmpdir}/testdata"
  cp "${ROOT}/build/Dockerfile.routercommander" "${tmpdir}/Dockerfile.routercommander"

  docker buildx build \
    --platform "${platform}" \
    --push \
    -f "${tmpdir}/Dockerfile.routercommander" \
    -t "${IMAGE_REPO}:${IMAGE_TAG}-${tag_suffix}" \
    "${tmpdir}"

  rm -rf "${tmpdir}"
}

build_and_push "linux/amd64" "linux_amd64" "linux-amd64"
build_and_push "linux/arm64" "linux_arm64" "linux-arm64"

docker buildx imagetools create \
  -t "${IMAGE_REPO}:${IMAGE_TAG}" \
  "${IMAGE_REPO}:${IMAGE_TAG}-linux-amd64" \
  "${IMAGE_REPO}:${IMAGE_TAG}-linux-arm64"
