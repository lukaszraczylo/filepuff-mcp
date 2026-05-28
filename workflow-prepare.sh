#!/usr/bin/env bash
# Invoked by lukaszraczylo/shared-actions go-release-cgo.yaml before GoReleaser,
# with TARGET_GOOS/TARGET_GOARCH set from the build matrix entry.
#
# GoReleaser Pro `release --split` partitions builds across matrix runners.
# Its default partition is by GOOS only, so every linux runner would build
# BOTH linux/amd64 and linux/arm64. With CGO_ENABLED=1 (go-tree-sitter needs
# CGO) the native gcc on one runner cannot assemble the other arch's CGO
# runtime (`gcc_arm64.S: no such instruction`), failing the release.
#
# Exporting GGOOS/GGOARCH restricts each runner to its own target. These are
# filter-only vars (unlike GOOS/GOARCH, which bleed into before-hooks such as
# `go run`/`go generate`) and require `partial.by: target` in .goreleaser.yaml.
set -euo pipefail

if [ -n "${GITHUB_ENV:-}" ] && [ -n "${TARGET_GOOS:-}" ] && [ -n "${TARGET_GOARCH:-}" ]; then
  echo "GGOOS=${TARGET_GOOS}" >>"${GITHUB_ENV}"
  echo "GGOARCH=${TARGET_GOARCH}" >>"${GITHUB_ENV}"
  echo "Pinned GoReleaser split target: GGOOS=${TARGET_GOOS} GGOARCH=${TARGET_GOARCH}"
fi
