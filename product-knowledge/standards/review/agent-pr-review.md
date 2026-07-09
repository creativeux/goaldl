<!--
GLaDOS-MANAGED STANDARD
Last Updated: 2026-07-09
-->
# Every PR gets an agent first-pass review

**Rule**: Every pull request MUST receive an automated agent review before human review. The agent publishes a `claude-review` commit status that is a **required check** on `main`; it blocks the merge only on `must`-severity findings (the GLaDOS gate tiers: `must` = block, `should` = warn, `may` = info). One human approval is still required — the agent supplements, never replaces, a person.

## How it works

`.github/workflows/pr-review.yml` (auth: Claude GitHub App + `CLAUDE_CODE_OAUTH_TOKEN` secret). Two review modes, selected by head branch:

- **Standard** (any normal PR): checks the PR description against `.github/pull_request_template.md`; checks the diff against every applicable standard in `standards/index.yml` and both philosophies; checks spec criteria if the PR maps to a `specs/` work unit; gives PM/Architect/QA quick-takes from `personas/`; verifies the Conventional-Commit type matches the changed files (`feat:`/`fix:` = product code only — CI/docs-only changes use `ci:`/`docs:`, or release-please cuts spurious releases).
- **Release** (`release-please--*` branches): the code already passed review when it merged to `main`, so the agent checks ONLY release items — file scope (`CHANGELOG.md` + manifest, anything else blocks), version bump per [release/versioning.md](../release/versioning.md) against the commits since the last tag, and changelog↔commit correspondence.

Triggers: automatic for in-repo branches (`opened`/`synchronize`/`reopened`); fork PRs get no secrets, so a maintainer comments **`@claude review`** to run one. The agent posts one structured comment ending in a `<!-- claude-review-verdict: PASS|BLOCK -->` marker; a deterministic `always()` step parses the marker and sets the status (fail-closed to `error` if no marker exists). Labels: `agent-reviewed` always, `needs-description` on incomplete descriptions (never on release PRs).

## GitHub Actions setup

The repo runs three workflows: **`ci.yml`** (the build gate — gofmt · vet · build · `test -race`, on push/PR), **`release.yml`** (release-please keeps the release PR; GoReleaser builds binaries on release — see [release/versioning.md](../release/versioning.md)), and **`pr-review.yml`** (this standard).

A verbatim snapshot of the workflow ships alongside this standard as [pr-review.yml](pr-review.yml) (incl. the full review prompt/rubric), so the implementation travels with the doc and can be reused in other repos. The **canonical, running copy is `.github/workflows/pr-review.yml`** — if they diverge, the canonical file wins; re-sync the snapshot when the workflow changes materially.

`pr-review.yml` anatomy:

- **Triggers**: `pull_request` (`opened`/`synchronize`/`reopened`) for the auto path; `issue_comment` (`created`) for the on-demand path. The job's `if:` admits non-fork PRs automatically, and comments containing `@claude review` only from `OWNER`/`MEMBER`/`COLLABORATOR` (blocks untrusted trigger spam).
- **Permissions**: `contents: read`, `pull-requests: write`, `issues: write`, `statuses: write`, `id-token: write` (OIDC for the app-token exchange). Attacker-controllable event fields (e.g. the head branch name) are passed into scripts via `env`, never inline-interpolated.
- **Concurrency**: grouped per PR number; `cancel-in-progress` **only** for `pull_request` events (a new push obsoletes the running review; comment events must never cancel — see below).
- **Steps**: resolve PR number/head SHA/review mode → set a `pending` `claude-review` status → checkout the PR head → run `anthropics/claude-code-action@v1` (the agent; `continue-on-error`, 12-min timeout, `--max-turns 40`, tools limited to `Read,Grep,Glob,Bash`; read-only analysis — it never builds or executes PR code, that stays CI's job) → the deterministic verdict-publishing step (`if: always()`).
- **Auth**: the **Claude GitHub App** must be installed on the repo, and the `CLAUDE_CODE_OAUTH_TOKEN` Actions secret set (generated via `claude setup-token`, a Pro/Max subscription token — no per-token API billing). Without the secret, runs fail closed to an `error` status; they never pass silently.
- **Branch protection on `main`**: PRs required; 1 approving review; required status check `claude-review` (`strict: false`); force-pushes and deletions blocked; `enforce_admins: false` so an admin can override a false block (also the escape hatch for bootstrap merges, below).
- **Labels** used by the agent: `agent-reviewed`, `needs-description` (create them before first run; the workflow does not create labels).

## Operational rules (learned the hard way)

- **PRs that edit `pr-review.yml` cannot review themselves** — the action skips unless the workflow file matches `main` (guardrail: a PR must not be able to rewrite its own reviewer). Workflow changes are admin-merged, then verified on a separate non-workflow PR.
- **The verdict lives in the comment marker, not the agent's last turn.** Don't revert to the agent setting the status directly or to file hand-offs — both lost verdicts when runs were interrupted.
- **Comment events must never cancel in-progress runs** (`cancel-in-progress` only for `pull_request`): the agent's own comment, posted as `claude[bot]`, fires `issue_comment` on the same PR and would cancel the run that posted it.
- CI/docs-only changes MUST use `ci:`/`docs:`/`chore:` commit types (see versioning standard) — the agent now flags mismatches.

**Why**: Prepares the repo for first community contributors — every PR (including the maintainer's) gets structured, standards-grounded feedback within minutes, before any human spends review time, and a genuine `must`-violation (raw-data-policy filtering, forbidden-seam edits, golden churn) cannot merge unnoticed. Shipped and verified end-to-end 2026-07-08/09 (PRs #26, #29, #32, #35, #38).
