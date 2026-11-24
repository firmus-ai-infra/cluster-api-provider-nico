#!/usr/bin/env bash
# FKE downstream build of CAPNICO: test -> multi-arch image -> release manifests pinned by digest.
# Runs the same locally and in Jenkins (fke/Jenkinsfile). Not part of the upstream project.
#
#   fke/build.sh test       make test lint, inside the Go image the Dockerfile builds with
#   fke/build.sh image      build + push linux/amd64,linux/arm64 as $IMAGE:$VERSION (refuses to overwrite)
#   fke/build.sh manifests  out/fke/{infrastructure-components.yaml,metadata.yaml} with the image pinned by digest
#
# VERSION   defaults to the exact tag on HEAD (v<upstream>-fke.<n>); required for image/manifests.
# IMAGE     defaults to harbor.cbr01.in.firmus.co/ai-infra/capnico
# REGISTRY_USER / REGISTRY_PASS   push credentials (Jenkins: harbor-nico-push); never echoed.
set -euo pipefail
cd "$(dirname "$0")/.."

IMAGE="${IMAGE:-harbor.cbr01.in.firmus.co/ai-infra/capnico}"
VERSION="${VERSION:-$(git describe --tags --exact-match 2>/dev/null || true)}"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
OUT="out/fke"
# The Go image the upstream Dockerfile builds with, so tests and image use one toolchain.
GO_IMAGE="$(sed -n 's/^FROM \(golang:[^ ]*\) AS builder$/\1/p' Dockerfile)"
tool="${CONTAINER_TOOL:-$(command -v docker >/dev/null && echo docker || echo podman)}"

die() { echo "fke/build.sh: $*" >&2; exit 1; }
need_version() {
  [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?-fke\.[0-9]+$ ]] \
    || die "VERSION must be a v<upstream>-fke.<n> tag on HEAD (got '${VERSION}')"
}
registry_login() {
  [[ -n "${REGISTRY_USER:-}" && -n "${REGISTRY_PASS:-}" ]] || die "REGISTRY_USER/REGISTRY_PASS required"
  printf '%s' "$REGISTRY_PASS" | "$tool" login "${IMAGE%%/*}" -u "$REGISTRY_USER" --password-stdin >/dev/null
}
digest_of() {  # registry digest of the pushed manifest list
  if [[ "$tool" == docker ]]; then
    docker buildx imagetools inspect "$1" --format '{{json .Manifest.Digest}}' | tr -d '"'
  else
    skopeo inspect --format '{{.Digest}}' "docker://$1"
  fi
}

in_go() {  # run make targets in the pinned Go image: the agent needs only a container tool
  [[ -n "$GO_IMAGE" ]] || die "cannot find the Go builder image in Dockerfile"
  # Linux tool binaries and Go caches in a named volume: outside the source tree (./... must not see
  # them) and never the host's bin/.
  "$tool" run --rm -v "$PWD:/src" -v capnico-fke-cache:/cache -w /src -e HOME=/tmp -e GOFLAGS=-buildvcs=false \
    -e LOCALBIN=/cache/bin -e GOMODCACHE=/cache/gomod -e GOCACHE=/cache/gocache "$GO_IMAGE" \
    bash -c "git config --global --add safe.directory /src && $*; rc=\$?; $(own_back); exit \$rc"
}
# Docker runs the container as root, so files it writes into the workspace would be root's and the Jenkins agent could
# no longer clean it. Rootless podman maps root to the invoking user already.
own_back() { [[ "$tool" == docker ]] && echo "chown -R $(id -u):$(id -g) /src" || echo true; }

case "${1:-}" in
  test)
    in_go make test lint
    ;;
  image)
    need_version; registry_login
    if "$tool" manifest inspect "$IMAGE:$VERSION" >/dev/null 2>&1; then
      die "$IMAGE:$VERSION already exists; tags are immutable - bump the -fke.<n> suffix"
    fi
    # Cross-compile in the builder stage (no emulation): the final stage only copies the binary.
    # shellcheck disable=SC2016 # ${BUILDPLATFORM} is for the Dockerfile, not the shell
    sed -e '1,/^FROM golang/ s/^FROM golang/FROM --platform=${BUILDPLATFORM} golang/' Dockerfile > Dockerfile.fke
    trap 'rm -f Dockerfile.fke' EXIT
    labels=(--label "org.opencontainers.image.version=$VERSION"
            --label "org.opencontainers.image.revision=$(git rev-parse HEAD)"
            --label "org.opencontainers.image.source=https://github.com/firmus-ai-infra/cluster-api-provider-nico")
    if [[ "$tool" == docker ]]; then
      # BuildKit runs in its own container and does not see the host's CA trust: hand it the CA bundle that
      # signs the registry (Firmus root CA for Harbor). buildx copies the files named in the config into it.
      registry="${IMAGE%%/*}"
      ca="${REGISTRY_CA:-}"
      for candidate in "/etc/docker/certs.d/$registry/ca.crt" /etc/ssl/certs/ca-certificates.crt /etc/pki/tls/certs/ca-bundle.crt; do
        [[ -z "$ca" && -s "$candidate" ]] && ca="$candidate"
      done
      [[ -n "$ca" ]] || die "no CA bundle found for $registry; set REGISTRY_CA"
      echo "registry CA for $registry: $ca"
      cfg="$(mktemp)"
      printf '[registry."%s"]\n  ca = ["%s"]\n' "$registry" "$ca" > "$cfg"
      trap 'rm -f Dockerfile.fke "$cfg"; docker buildx rm capnico-fke >/dev/null 2>&1 || true' EXIT
      # Recreate the builder every run, so a stale one without the CA config is never reused.
      docker buildx rm capnico-fke >/dev/null 2>&1 || true
      docker buildx create --name capnico-fke --driver docker-container --buildkitd-config "$cfg" >/dev/null
      docker buildx build --builder capnico-fke --push --platform "$PLATFORMS" "${labels[@]}" \
        -t "$IMAGE:$VERSION" -f Dockerfile.fke .
    else
      podman build --platform "$PLATFORMS" "${labels[@]}" --manifest "$IMAGE:$VERSION" -f Dockerfile.fke .
      podman manifest push --all "$IMAGE:$VERSION" "docker://$IMAGE:$VERSION"
    fi
    mkdir -p "$OUT"
    digest="$(digest_of "$IMAGE:$VERSION")"
    [[ "$digest" == sha256:* ]] || die "could not read the pushed digest"
    printf '%s@%s\n' "$IMAGE:$VERSION" "$digest" > "$OUT/image.txt"
    echo "pushed $(cat "$OUT/image.txt")"
    ;;
  manifests)
    need_version
    [[ -s "$OUT/image.txt" ]] || die "$OUT/image.txt missing - run 'fke/build.sh image' first"
    in_go make release-manifests RELEASE_DIR="$OUT" CONTROLLER_IMG="$(cat "$OUT/image.txt")"
    grep -q "image: $(cat "$OUT/image.txt")" "$OUT/infrastructure-components.yaml" || die "image not pinned in components"
    (cd "$OUT" && sha256sum infrastructure-components.yaml metadata.yaml > SHA256SUMS)
    echo "release manifests in $OUT"
    ;;
  *) die "usage: fke/build.sh test|image|manifests" ;;
esac
