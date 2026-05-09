#!/usr/bin/env bash

set -euo pipefail

ROOT="${BUILD_WORKSPACE_DIRECTORY:-$(cd "$(dirname "$0")/.." && pwd)}"
PLATFORM="${1:?platform is required}"
IMAGE_REPO="${IMAGE_REPO:-docker.io/sbezverk/routercommander}"
IMAGE_TAG="${IMAGE_TAG:-latest}"

case "${PLATFORM}" in
  linux/amd64)
    BAZEL_CONFIG="linux_amd64"
    TAG_SUFFIX="linux-amd64"
    ;;
  linux/arm64)
    BAZEL_CONFIG="linux_arm64"
    TAG_SUFFIX="linux-arm64"
    ;;
  *)
    echo "unsupported platform: ${PLATFORM}" >&2
    exit 1
    ;;
esac

cd "${ROOT}"

bazel build --config="${BAZEL_CONFIG}" //:routercommander --color=no --curses=no

TMPDIR="$(mktemp -d)"
trap 'rm -rf "${TMPDIR}"' EXIT
mkdir -p "${TMPDIR}/bin"

cp "${ROOT}/bazel-bin/cmd/routercommander_/routercommander" "${TMPDIR}/bin/routercommander"
cp -R "${ROOT}/testdata" "${TMPDIR}/testdata"
cp "${ROOT}/build/Dockerfile.routercommander" "${TMPDIR}/Dockerfile.routercommander"

docker buildx build \
  --platform "${PLATFORM}" \
  --load \
  -f "${TMPDIR}/Dockerfile.routercommander" \
  -t "${IMAGE_REPO}:${IMAGE_TAG}-${TAG_SUFFIX}" \
  "${TMPDIR}"
