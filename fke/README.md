# FKE downstream build

Branch `fke/main` of `firmus-ai-infra/cluster-api-provider-nico` = upstream `dsx-ai-factory/cluster-api-provider-nico`
plus the changes FKE needs before they merge upstream, plus this directory. Nothing here goes upstream.

| Commit on top of upstream | Upstream |
| ------------------------- | -------- |
| `feat(api): let NicoMachine carry its site and credentials without a NicoCluster` | issue #210 |
| `chore(fke): downstream build` (this directory) | - |

## Release

| Item | Value |
| ---- | ----- |
| Version | `v<upstream>-fke.<n>`, for example `v0.1.0-rc.3-fke.1`; `<n>` restarts at 1 for each upstream base |
| Image | `harbor.cbr01.in.firmus.co/ai-infra/capnico:<version>`, `linux/amd64` + `linux/arm64`, immutable |
| Manifests | `infrastructure-components.yaml` (image pinned by digest) + `metadata.yaml`, archived by the Jenkins job |
| Consumer | `cbr01-fke-mgmt` `components/capi/providers`: ConfigMap `<version>` selected by the `InfrastructureProvider` |

1. Tag the `fke/main` commit: `git tag -s v0.1.0-rc.3-fke.<n>` (signed; needs `export GPG_TTY=$(tty)`), push the tag.
2. Run Jenkins job `capnico-fke-build` with `TAG=<version>`.
3. Copy the archived `infrastructure-components.yaml` and `metadata.yaml` into the provider ConfigMap in `cbr01-fke-mgmt`
   and bump `InfrastructureProvider.spec.version`.

Locally, with the same commands: `fke/build.sh test`, `VERSION=... REGISTRY_USER=... REGISTRY_PASS=... fke/build.sh image`,
`fke/build.sh manifests`. The agent needs only docker (buildx) or podman: tests and manifests run in the Go image the
Dockerfile builds with, and the image cross-compiles.

## Rebase onto a new upstream release

```bash
git fetch upstream --tags
git rebase --onto <new-upstream-tag> <old-upstream-tag> fke/main   # drop commits that merged upstream
fke/build.sh test
```

## Jenkins job

`fke/jenkins/capnico-fke-build.xml` is the job definition (agent `arm64-native`, credentials `github-app-firmus-ai-infra`
and `harbor-nico-push`). Apply it as the FKD jobs are applied (`fkd-accelerated-compute/jenkins/README.md`): `createItem`
with `name=capnico-fke-build`, later `job/capnico-fke-build/config.xml`.
