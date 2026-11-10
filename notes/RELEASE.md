# Renderer releases

Each renderer has an independent VERSION, image and release. Use the common
operator tool below to coordinate releases; renderer-owned Actions still perform
verification and publication. Current versions, immutable references and actual
acceptance evidence belong in each renderer's README, VALIDATION.md and GitHub
Release, rather than in this procedure.

| Renderer argument | Directory | Annotated tag | Release workflow | GHCR image |
| --- | --- | --- | --- | --- |
| `picsum` | `renderers/picsum/` | `picsum/vMAJOR.MINOR.PATCH` | `release-picsum.yaml` | `ghcr.io/dayflower/arprt-renderer-picsum` |
| `rss` | `renderers/rss/` | `rss/vMAJOR.MINOR.PATCH` | `release-rss.yaml` | `ghcr.io/dayflower/arprt-renderer-rss` |
| `ical` | `renderers/ical/` | `ical/vMAJOR.MINOR.PATCH` | `release-ical.yaml` | `ghcr.io/dayflower/arprt-renderer-ical` |
| `weather` | `renderers/weather/` | `weather/vMAJOR.MINOR.PATCH` | `release-weather.yaml` | `ghcr.io/dayflower/arprt-renderer-weather` |
| `prometheus` | `renderers/prometheus/` | `prometheus/vMAJOR.MINOR.PATCH` | `release-prometheus.yaml` | `ghcr.io/dayflower/arprt-renderer-prometheus` |
| `countdown` | `renderers/countdown/` | `countdown/vMAJOR.MINOR.PATCH` | `release-countdown.yaml` | `ghcr.io/dayflower/arprt-renderer-countdown` |

## Initial setup

- Enable GitHub Actions and package/release write permissions for release jobs.
  Connect existing GHCR packages to this repository and grant Actions access.
- Require CI on main and restrict renderer release tag creation/update/deletion
  to maintainers. Never force-move a release tag.
- Confirm native `ubuntu-24.04` amd64 and `ubuntu-24.04-arm` runners are available.
- Install Python 3.12+, Git and GitHub CLI, and authenticate `gh` for github.com.
  Configure Git push authentication separately, for example with
  `gh auth setup-git`. The coordinator does not change credentials or Git config.
- Use a checkout whose origin fetch and push URLs both identify
  `dayflower/arprt-renderers` on github.com. HTTPS and standard Git SSH URLs are
  accepted. Custom host aliases, forks and multiple push URLs are rejected.

The coordinator is `python3 tools/release.py`. It requires neither Docker nor
additional Python packages. Commands can be invoked from any working directory;
the checkout containing the script is used. Read-only commands do not fetch,
switch branches or move local refs. It does not create commits, PRs or merges,
modify branch protection, rerun failed workflows, or deploy a renderer.

## Prepare a version

Choose a renderer and update its VERSION on your working branch:

```sh
python3 tools/release.py bump weather minor --dry-run
python3 tools/release.py bump weather minor
```

The second positional argument accepts `major`, `minor`, `patch` or an explicit
stable version such as `0.3.0`. For example, `0.2.3 minor` becomes `0.3.0`.
Explicit versions must be greater than the current VERSION. Leading zeros,
prereleases and build metadata are rejected. The working tree must be clean,
including untracked files, even with `--dry-run`.

Only `renderers/<renderer>/VERSION` is replaced. Update the changelog and review
other release documentation manually; changelog formats differ and not every
renderer has one. Commit and merge the preparation through the normal review
process. Keep versions independent of other renderers and ARPRT. Preserve the
required Codex co-author attribution and inspect actual commit messages before
pushing Codex-contributed commits.

Run `make check`, then build and run the relevant renderer's container checker,
as described in [DEVELOP.md](DEVELOP.md). These local checks are manual; the
coordinator checks GitHub CI rather than claiming to have run them.

## Check an exact commit

```sh
python3 tools/release.py check weather --revision <commit>
```

Use the reviewed main commit intended for publication. The tool resolves the
revision once to a full SHA, reads VERSION and release-tool/workflow paths from
that commit, and derives the tag from its VERSION. It checks that:

- Origin identifies the intended repository and GitHub access works.
- The exact commit is an ancestor of remote main.
- The newest main-push `ci.yaml` run for that SHA succeeded, including common,
  renderer source and native amd64/arm64 jobs. PR runs or successful older runs
  do not substitute for the newest matching run.
- The remote release tag and GitHub Release do not already exist.

Success means `ready` to start publication, not proof that the package version
is unused or publicly pullable. GHCR version/platform-tag availability remains
the final responsibility of the existing Actions guard, using the workflow
token. Local package API permissions are not required. Authentication, network
and malformed-response failures are not treated as an unused version.

## Publish

Only execute publication when it has been explicitly authorized:

```sh
python3 tools/release.py publish weather --revision <commit> --dry-run
python3 tools/release.py publish weather --revision <commit>
```

`publish` requires a clean working tree, repeats the exact-commit checks, prints
the target to stderr, creates an annotated tag and pushes only that tag. It does
not push a branch or retarget publication if main advances. The full SHA must
already be available locally; fetch it yourself if needed.

By default it tracks the matching tag-push release workflow until completion.
Use `--no-wait` to return its current state after the tag is confirmed, or
`--timeout <seconds>` to limit waiting (default 900 seconds).
`--dry-run` performs reads and checks only, with no tag creation or push.
It does not guarantee that a later Actions guard or publication will succeed.

Existing local tags must be annotated and directly point to the requested SHA.
An existing matching remote annotated tag resumes observation without another
push. Conflicting/lightweight tags stop the command. A push error is followed
by a remote tag check: if the expected SHA is confirmed, observation continues;
otherwise the result is uncertain and requires inspection. Tags are never
force-pushed, moved or deleted automatically.

The renderer's release workflow checks VERSION, annotated tag, exact checkout
and main ancestry. It calls only that renderer's verification workflow, builds
and tests native amd64/arm64 images, and exports those exact image archives.
Publication loads the tested archives without rebuilding, verifies platform and
OCI labels, rejects existing version/platform tags and Releases, publishes the
two platform tags and a multi-platform version index, and creates a GitHub
Release with `<renderer>-index.json`. It does not publish `latest`.

## Observe or resume

```sh
python3 tools/release.py status weather --version 0.3.0
python3 tools/release.py status weather --version 0.3.0 --watch --timeout 900
python3 tools/release.py status weather --version 0.3.0 --json
```

Status reads GitHub, independent of the current local branch or working tree.
It verifies the annotated remote tag and its VERSION, and selects the newest
push run matching the renderer workflow, tag and exact SHA.

| State | Meaning |
| --- | --- |
| `not_started` | No remote tag or Release exists |
| `awaiting_run` | Tag exists; its workflow has not been observed |
| `running` | Matching workflow is queued or running |
| `published` | Workflow succeeded and the stable Release/index evidence agrees |
| `failed` | Workflow failed, was cancelled or otherwise did not succeed |
| `inconsistent` | Tag, VERSION, Release or index evidence conflicts/is missing |
| `unknown` | Data or remote state could not be read reliably |
| `timeout` | Waiting ended; publication may still be running |

`published` requires a stable, non-draft Release and its uploaded index asset
with a SHA-256 digest and exactly linux/amd64 and linux/arm64 manifests. Existing
release-note source and immutable-reference fields must match the tag and index.
This is workflow/metadata evidence, not anonymous registry or device acceptance.

`--watch` waits for `published` or `failed`, or stops on error/timeout.
No state file is needed: rerun `status` to observe progress after interruption.
A completed failure is never automatically rerun, even if some publication
steps might not have run.

All commands accept `--json`, emitting one final result object on stdout.
Publication notices go to stderr. Without `--watch`, a successfully read pending
status exits 0; a known failed state/check exits 1. Invalid arguments/version
exit 2; unreadable/uncertain state or timeout exits 3; interruption exits 130.
JSON contains `state`, `renderer` and `message`, plus known version, revision,
tag, run URL, Release URL and digest fields. Consumers should use `state` to
distinguish pending observation from completed publication.

## Failure and recovery

Never delete existing tags to bypass guards. After any partial image push,
investigate and choose a new version rather than rerunning publication.
If the complete image was published but only Release creation failed, inspect
the recorded digest and manually finish that Release without rebuilding or
repushing images. The coordinator reports failure/inconsistency; it does not
attempt this repair.

The workflow serializes renderer releases, but external pushes are outside that
serialization. GHCR tags are not inherently immutable: limit package write
permissions and use digest references for deployment. A 404 can conceal
inaccessible resources, so establish repository/package access separately when
investigating first-publication failures.

## Manual postpublication work

These tasks remain manual; the coordinator does not perform or claim them:

1. Confirm package visibility is Public. Verify anonymous version-tag and
   index-digest pulls on amd64 and arm64 with a fresh empty Docker configuration.
2. Confirm platform and OCI version/source/revision labels. Run
   `make -C renderers/<renderer> container-check IMAGE=<digest-reference>`
   on available native hosts. Pulling arm64 does not establish arm64 execution.
3. Record prepublication CI, anonymous pulls, postpublication execution and
   browser/cluster/physical acceptance separately. If release immutability
   prevents adding assets, record later evidence in editable notes/documents.
4. Update the renderer README, validation record, GitHub Release notes and Screen
   digest pins only after availability has been verified. Keep examples synthetic.

For example, after creating an empty Docker config directory:

```sh
DOCKER_CONFIG=/absolute/empty-config docker pull --platform linux/amd64 ghcr.io/dayflower/arprt-renderer-weather:0.3.0
DOCKER_CONFIG=/absolute/empty-config docker pull --platform linux/arm64 ghcr.io/dayflower/arprt-renderer-weather:0.3.0
```

Live sources, browser layout, Kubernetes/PVC integration and physical displays
have separate acceptance scopes. Job renderer publication needs no Helm release
and does not change existing Device assignments.
