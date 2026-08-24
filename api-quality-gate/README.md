# ThunderID API quality gate (Plan A)

Executable enforcement of API production-completeness. The design goal is to
**shrink human judgment to its irreducible core and make even that core
merge-blocking** — every rule that can be mechanized becomes a required status
check the merge cannot proceed without.

This directory is a **self-contained npm package**. It lives here (not at the
repo root) because the root is a pnpm monorepo; a second npm `package.json` at
the root would collide with pnpm. All commands below run from `api-quality-gate/`.

## What enforces what

| Failure class | Control | File | CI job |
|---|---|---|---|
| Collection endpoint that does not **declare** pagination/filter/sort | Spectral rule | `.spectral.yaml` + `functions/requiredCollectionQueryParams.js` | `validate-api-design` |
| Error responses not **declared**, or not using the agreed media type and schema | Spectral rules | `requiredErrorResponses.js`, `errorsUseStandardErrorSchema.js` | `validate-api-design` |
| Create that does not **declare** an `Idempotency-Key` header | Spectral rule | `writeHasIdempotency.js` | `validate-api-design` |
| Pagination `next` link that is not followable | Contract test (runtime) | `tests/integration/contract/<resource>/` | `contract-tests` |
| **A declared capability that the server ignores** (`filter`, `sort`, `Idempotency-Key`) | **Not mechanized.** The rules require the spec to declare them; nothing asserts the handler honours them, so adding the parameter satisfies the check | — | — |
| **A declared error response the server never returns** (400/401/403/500) | **Not mechanized.** Only 409 on conflict and 404 after delete are asserted at runtime | — | — |
| Handler drifts from spec | **Not mechanized.** Would need the HTTP layer generated from `api/*.yaml`; it is hand-written, so there is no generated output to diff | — | — |
| Backward-incompatible change | oasdiff | `.github/workflows/api-quality-gate.yml` | `detect-breaking-changes` |
| New endpoint ships with no test | Coverage meta-check | `scripts/check-coverage.mjs` | `validate-test-coverage` |
| Resource model / "is this the right design" | Human review | — | — |
| Someone weakens a rule or a waiver | Exemption governance | `governance/exemptions.*`, `validate-exemptions.mjs` | `validate-exemptions` |

Spec-lint proves the YAML *claims* completeness. Contract tests prove a few of those
claims hold at runtime: today the pagination `next` link, 409 on conflict, 404 after
delete, and the membership lifecycle. Most declared behaviour, and every declared
error response bar those two, is still unasserted. The audit measures coverage in
(operation, status) pairs observed from the server access log, so a success-path-only
test cannot read as a covered operation: api/group.yaml sits at 13 of 43 declared
responses. See the "not mechanized" rows above for what remains open.

## Current scope in this repo

- **Which specs are gated:** every `api/**/*.yaml` except those named in
  `governance/excluded-specs.yaml`, resolved by `scripts/list-specs.mjs`. The list
  is a deny list, so a newly added spec is gated because nobody did anything and
  opting out takes an explicit entry. It is currently empty: all 23 specs are in.
- **What a pull request is judged on:** only the specs it changes, and only against
  their own state on the base branch (`scripts/lint-diff.mjs`). A change may not add
  findings but is not answerable for a file's existing backlog. A newly added spec
  has no baseline, so it is enforced in full. The whole surface is measured daily by
  `api-design-audit.yml` instead.
- **Run model:** the Spectral ruleset resolves `overrides` globs relative to its own
  directory, so repo-root specs are referenced from here as `../api/<name>.yaml`.
- **No codegen-drift check:** the HTTP layer is hand-written (routes in
  `backend/internal/*/init.go`) and the only artifact generated from `api/*.yaml`
  is `docs/static/api/combined.yaml`, which is gitignored. With no committed
  generated output there is nothing to diff, so no such job exists. Add one if
  the server ever becomes spec-generated.
- **`contract-tests` uses SQLite**, the repo default, booting the built
  distribution via the `tests/integration` harness. No service container or
  schema seeding is required.

## Verify it yourself (don't trust the ruleset blind)

```bash
cd api-quality-gate
nvm use              # any Node matching engines (^18.18 || >=20.17); repo .nvmrc is 24
npm ci
npm run test:rules    # bad sample must trip rules; good sample must pass clean
node scripts/validate-exemptions.mjs
node scripts/list-specs.mjs                       # which specs are gated
npx spectral lint ../api/group.yaml -r .spectral.effective.yaml --fail-severity=warn
node scripts/lint-diff.mjs api/group.yaml --base-ref origin/main   # the PR check
CONTRACT_DIR=../tests/integration/contract/group \
  node scripts/check-coverage.mjs ../api/group.yaml
```

> **ajv note:** `@stoplight/spectral-*` pulls a transitive `ajv` whose
> `errorMessage` codegen is broken (garbled `{"str":...}` output, `SyntaxError`).
> This crashes Spectral on **every** Node version tested (18, 20, 22, 24), so it
> is not the "Node 22" issue and is not fixed by pinning Node. `package.json`
> pins `ajv` via an `overrides` block to a working version. With that in place
> Spectral runs cleanly on all of them, so there is **no Node pin** — CI follows
> the repo `.nvmrc` (24), within Spectral's declared support of `node >= 20.17`.

## Guarding the guard

- **Gap:** nothing currently requires review to change the ruleset, a rule
  function, or a workflow, so the gate can be loosened without sign-off. Adding
  CODEOWNERS entries for this directory would close it.
- Exemptions are first-class: every waiver in `governance/exemptions.yaml` must
  pass `governance/exemptions.schema.json` (justification ≥30 chars, owner,
  tracking issue) **and** carry an `expires` date. `validate-exemptions.mjs`
  fails the build on any expired waiver and emits `.spectral.effective.yaml`
  (base + waivers), which CI lints — a waiver cannot exist without governance.
- `api-design-audit.yml` re-lints every gated spec on `main` daily and reports the
  surface total, so debt on specs no pull request touches stays visible.

## Making it blocking

Branch protection is administered at the repository level and is out of scope for
this tooling. To make the gate blocking, an administrator marks these contexts as
required on the protected branch:

`validate-exemptions`, `validate-api-design`, `detect-breaking-changes`,
`contract-tests`, `validate-test-coverage`

A check must report at least once before it becomes selectable, which any run of
this workflow satisfies.

## Rolling the gate out to another resource

1. Add `operationId`s (and descriptions/contact) to the target `api/<resource>.yaml`.
2. Lint it: `npx spectral lint ../api/<resource>.yaml -r .spectral.effective.yaml`.
3. For genuine gaps (unimplemented filter/sort/idempotency), add a
   time-boxed entry to `governance/exemptions.yaml` with a tracking issue — do
   not downgrade the rule.
4. Add a contract-test package at `tests/integration/contract/<resource>/` whose
   `coveredOperations` manifest maps every operationId to a test that exists and
   does not skip. The jobs discover it by resource name; nothing needs wiring.
