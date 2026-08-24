#!/usr/bin/env node
// lint-diff.mjs
//
// Ratchet: lint a spec as the PR leaves it, lint the same spec as it exists on the
// base branch, and fail only on findings the change INTRODUCES.
//
// Why: linting a changed spec outright would charge the author for the whole
// file's backlog. api/user.yaml carries 65 findings, so a one-line typo fix would
// report 65 failures and train everyone to ignore the check. The ratchet lets
// existing debt sit still while making it impossible to add more.
//
// Findings are matched on rule id + JSON path, never on line number: any edit
// shifts lines, and a line-based key would report the whole file as new.
//
// Two deliberate choices:
//   * The BASE content is linted with the PR's ruleset, not the base branch's.
//     Otherwise adding or tightening a rule would make every pre-existing
//     violation look new, and the next person to touch a spec inherits the blame.
//   * A spec that CRASHES Spectral fails hard. A crash yields zero findings, which
//     a naive diff reads as "nothing added" and passes green. That is the same
//     escape-by-not-matching failure this gate exists to prevent.
//
// A spec with no base version (newly added) has no baseline, so every finding
// counts as new and it is enforced in full.
//
// Usage: node scripts/lint-diff.mjs <spec> --base-ref <ref> [--ruleset <path>]
// Exit:  0 clean, 1 new findings, 2 tooling/lint error.

import fs from 'node:fs';
import path from 'node:path';
import url from 'node:url';
import { spawnSync } from 'node:child_process';

const ROOT = path.resolve(path.dirname(url.fileURLToPath(import.meta.url)), '..');
const REPO = path.resolve(ROOT, '..');

const argv = process.argv.slice(2);
const flags = {};
const positional = [];
for (let i = 0; i < argv.length; i++) {
  if (argv[i].startsWith('--')) flags[argv[i].slice(2)] = argv[++i];
  else positional.push(argv[i]);
}
const spec = positional[0];
const baseRef = flags['base-ref'] || 'origin/main';
const ruleset = path.resolve(ROOT, flags.ruleset || '.spectral.effective.yaml');

if (!spec) {
  console.error('usage: lint-diff.mjs <spec> --base-ref <ref> [--ruleset <path>]');
  process.exit(2);
}

const die = (msg) => {
  console.error(`✗ ${msg}`);
  process.exit(2);
};

// Findings at or above the gate's threshold: error (0) and warn (1).
const BLOCKING = new Set([0, 1]);
const fingerprint = (f) => `${f.code}|${(f.path || []).join('.')}`;

// `spectral lint -f json` prints the JSON array and then a human-readable summary
// on the same stream, so the raw stdout does not parse. Take the first balanced
// array and ignore the trailing prose.
const extractArray = (out) => {
  const start = out.indexOf('[');
  if (start === -1) return null;
  let depth = 0;
  let inStr = false;
  let esc = false;
  for (let i = start; i < out.length; i++) {
    const c = out[i];
    if (inStr) {
      if (esc) esc = false;
      else if (c === '\\') esc = true;
      else if (c === '"') inStr = false;
      continue;
    }
    if (c === '"') inStr = true;
    else if (c === '[') depth++;
    else if (c === ']' && --depth === 0) return out.slice(start, i + 1);
  }
  return null;
};

// Runs Spectral and returns its findings. A crash is fatal rather than "no
// findings", so a spec that stops parsing cannot sneak through as unchanged.
const lint = (file, label) => {
  const res = spawnSync(
    'npx',
    ['spectral', 'lint', file, '-r', ruleset, '-f', 'json'],
    { cwd: ROOT, encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 }
  );
  if (res.error) die(`could not run spectral for ${label}: ${res.error.message}`);
  if (res.status === 2 || !res.stdout.trim()) {
    console.error(res.stderr.trim() || '(no output)');
    die(`Spectral failed on the ${label} copy of ${spec}. Fix the spec or the tooling; a crashing spec is not "no findings".`);
  }
  const json = extractArray(res.stdout);
  if (json === null) {
    console.error(res.stdout.slice(0, 400));
    die(`could not find JSON output for the ${label} copy of ${spec}`);
  }
  try {
    return JSON.parse(json).filter((f) => BLOCKING.has(f.severity));
  } catch {
    console.error(json.slice(0, 400));
    die(`could not parse Spectral output for the ${label} copy of ${spec}`);
  }
};

// --- head -------------------------------------------------------------------
const headFile = path.join(REPO, spec);
if (!fs.existsSync(headFile)) die(`${spec} does not exist in the working tree`);
const head = lint(headFile, 'current');

// --- base -------------------------------------------------------------------
const show = spawnSync('git', ['show', `${baseRef}:${spec}`], {
  cwd: REPO,
  encoding: 'utf8',
  maxBuffer: 64 * 1024 * 1024,
});

let base = [];
let isNew = false;
if (show.status !== 0) {
  // Absent on the base branch means a newly added spec: no baseline, so the whole
  // file is held to the rules. Anything else (bad ref, unreadable object) is a
  // tooling failure and must not be mistaken for "new spec".
  if (/exists on disk, but not in|does not exist|unknown revision|invalid object|fatal: path/i.test(show.stderr)) {
    isNew = true;
  } else {
    console.error(show.stderr.trim());
    die(`could not read ${spec} at ${baseRef}`);
  }
} else {
  // Written beside the real spec so any relative $ref would still resolve.
  const tmp = path.join(path.dirname(headFile), `.lint-base-${path.basename(spec)}`);
  try {
    fs.writeFileSync(tmp, show.stdout);
    base = lint(tmp, `${baseRef}`);
  } finally {
    fs.rmSync(tmp, { force: true });
  }
}

// --- diff -------------------------------------------------------------------
const baseKeys = new Set(base.map(fingerprint));
const headKeys = new Set(head.map(fingerprint));
const added = head.filter((f) => !baseKeys.has(fingerprint(f)));
const fixed = base.filter((f) => !headKeys.has(fingerprint(f)));

const sev = (s) => (s === 0 ? 'error' : 'warn');

if (isNew) {
  console.log(`${spec}: new spec, no baseline; all ${head.length} finding(s) apply.`);
} else {
  console.log(
    `${spec}: ${head.length} finding(s) now, ${base.length} on ${baseRef}` +
      ` (${added.length} added, ${fixed.length} fixed).`
  );
}

if (added.length) {
  console.error(`\nThis change introduces ${added.length} new finding(s) in ${spec}:`);
  for (const f of added) {
    const where = (f.path || []).join('.');
    console.error(`  ${sev(f.severity)}  [${f.code}]  ${f.message}${where ? `  (${where})` : ''}`);
  }
  console.error(
    `\nPre-existing findings are not your problem; these ${added.length} are. ` +
      `Fix them, or add a governed exemption.`
  );
  process.exit(1);
}

if (fixed.length) console.log(`  ${fixed.length} pre-existing finding(s) fixed by this change. Thank you.`);
console.log(`  No new findings.`);
