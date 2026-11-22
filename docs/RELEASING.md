# Merge tags and GitHub releases

[← Back to GoSync](../readme.md)

The [release workflow](../.github/workflows/TagReleaseBuildPush.yaml) tags merged
GitHub pull requests. A merge into the repository's default branch also publishes
downloadable binaries in [GitHub Releases](https://github.com/delduked/go_sync/releases).
The default branch is currently `main`; the same rule applies if it is renamed to
`master` later.

| Event | Result |
|:---|:---|
| PR merged into any other branch | Tag at that PR's merged commit |
| PR merged into the default branch | Tag, then tests, builds, and a release using that tag |
| PR closed without merging | No tag or release |
| Ordinary branch push | Regular regression CI; no automatic tag or release |
| Rerun of the same merge workflow | Reuses the tag; resumes an unfinished draft or leaves a complete published release unchanged |

Use GitHub PR merges to drive this workflow. A merge performed locally and pushed
without a merged GitHub PR does not produce a PR-closed event. Existing commits
and historical merges are not backfilled.

## Tags and exact source versions

Tags use the project's original `v_<6-character SHA>.<hex Unix timestamp>` format.
Release display names use `v_<6-character SHA>.<YYYYMMDD>`. For example, a merge
of commit `1234567890ab…` at `2026-11-22T00:00:00Z` produces:

- Tag: `v_123456.6b023080`
- Release name: `v_123456.20261122`

The timestamp and UTC date come from the PR's recorded merge time, rather than
the runner's current clock. Delayed builds and reruns therefore reuse the same
tag and release name, including when retried on another day. These identify merge
builds; they are not semantic-version stability promises. The merged PR's
`merge_commit_sha` is used for both the tag and
checkout, covering GitHub merge, squash, and rebase merges. Later pushes cannot
change which source gets built.

The tag is created first, even if subsequent tests or builds fail. A failed build
does not publish a release. An existing tag pointing elsewhere is an error; the
workflow never moves it. Per-PR concurrency prevents overlapping reruns without
cancelling unrelated merges. Previously published tags and releases are retained.

## Release contents and checks

Default-branch merges run the release behavior tests, Go race tests, `go vet`, and
the three-container mDNS/file regression suite before building all six binaries:

| Platform | amd64 / Intel or AMD | arm64 / ARM or Apple Silicon |
|:---|:---|:---|
| Linux | `gosync-linux-amd64` | `gosync-linux-arm64` |
| macOS | `gosync-darwin-amd64` | `gosync-darwin-arm64` |
| Windows | `gosync-windows-amd64.exe` | `gosync-windows-arm64.exe` |

`SHA256SUMS` accompanies the six raw executables. Download the matching binary;
Linux/macOS users also need to make it executable. macOS and Windows builds remain
cross-compiled, with native runtime validation still outstanding. Packaging,
Homebrew, installers, and OS code signing are separate future work.

The publishing job verifies the expected files and their checksums, creates a
draft with generated release notes, uploads all seven files, and only then makes
the release public. It uses GitHub's `legacy` latest-release selection instead of
unconditionally making whichever build finishes last the latest release.

## Runner and permissions

Jobs use GitHub-hosted `ubuntu-latest` runners and the built-in `GITHUB_TOKEN`.
No custom runner or `ACCESS_TOKEN` secret is required. Repository or organization
policies must permit Actions and the declared `contents: write` permission.

The workflow uses `pull_request_target: closed` with a merged-only condition so
merged contributions from forks can be tagged too. GitHub loads this workflow from
the default branch. Tagging and publishing use inline API calls and never check
out or execute repository code. Tests and compilation run in a separate job with
read-only contents permission, no persisted checkout credentials, and no Go cache.
The publishing job downloads the current run's artifact; it treats the files as
data and never executes them.

GitHub suppresses most new workflow triggers from operations made with
`GITHUB_TOKEN`, so publication deliberately happens in the same workflow as tag
creation. It does not depend on a second tag-push workflow firing.

## Recovering a failed run

Open **Actions → Tag merges and publish releases**, choose the merge run, and
rerun its failed jobs (or all jobs). The same event produces the same tag.

- A failed test or build leaves the tag and no public release.
- An interrupted upload leaves a draft; a rerun replaces draft assets and
  publishes only after all uploads succeed.
- A complete published release is left untouched. Missing public assets,
  unexpected draft assets, or a moved tag require investigation instead of
  automatic replacement.
- Build artifacts expire after seven days. Rerun all jobs to rebuild them if
  retrying publication after that period.

Once this workflow is on the default branch, future PR merges trigger it. Updating
the workflow by a direct push does not itself create a release.

## Local validation

```sh
node --test scripts/release-workflow.test.cjs
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7
make dist
```

The Node tests execute the workflow's actual GitHub API scripts with a simulated
GitHub API and temporary local artifacts. They cover branch selection, exact
commits, duplicate runs, invalid tags, interrupted uploads, incomplete public
releases, and bad checksums. They create no real tags or releases. The ordinary
regression workflow also runs these tests and workflow linting on pushes and PRs.

GitHub references: [merged PR events](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#pull_request_target),
[token-trigger behavior](https://docs.github.com/en/actions/how-tos/writing-workflows/choosing-when-your-workflow-runs/triggering-a-workflow),
and the [release API](https://docs.github.com/en/rest/releases/releases).
