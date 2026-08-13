#!/usr/bin/env node
// check-coverage.mjs
//
// Guard against the "escapes by simply not matching" failure mode: every
// operation in the spec must (a) have an operationId, and (b) be tied to a
// contract test that actually exists and actually runs.
//
// (b) is deliberately NOT a text grep. Operation ids appear in the contract
// tests mostly inside assertion messages and comments, so grepping lets any
// mention — including the manifest entry itself — certify coverage. Instead the
// contract package declares a manifest
//
//     var coveredOperations = map[string]string{ "createGroup": "TestGroupLifecycle", ... }
//
// and this script resolves each named test in the Go source: the function must
// exist, and if its body only calls t.Skip it counts as SCAFFOLDED, not covered.
// Scaffolds are allowed but capped, so partial suites stay honest and the debt
// cannot grow silently.
//
// Usage: node scripts/check-coverage.mjs <openapi-spec-path>
// Deps: js-yaml

import fs from 'node:fs';
import path from 'node:path';
import yaml from 'js-yaml';

const SPEC = process.argv[2] || 'api/openapi.yaml';
const CONTRACT_DIR = process.env.CONTRACT_DIR || 'contract-tests';
const METHODS = ['get', 'post', 'put', 'patch', 'delete', 'head', 'options'];

// Operations whose only test is a t.Skip stub. Lower this as scaffolds become
// real assertions; raising it needs a governance review (see CODEOWNERS).
const MAX_SCAFFOLDED = Number(process.env.MAX_SCAFFOLDED ?? 3);

const fail = (msg) => {
  console.error(`✗ ${msg}`);
  process.exit(1);
};

if (!fs.existsSync(SPEC)) fail(`Spec not found: ${SPEC}`);
const spec = yaml.load(fs.readFileSync(SPEC, 'utf8'));

const operations = [];
for (const [p, item] of Object.entries(spec.paths || {})) {
  for (const m of METHODS) {
    if (item && item[m]) operations.push({ p, m, operationId: item[m].operationId });
  }
}

const missingId = operations.filter((o) => !o.operationId);
if (missingId.length) {
  for (const o of missingId) console.error(`  ${o.m.toUpperCase()} ${o.p} has no operationId`);
  fail(`${missingId.length} operation(s) missing operationId.`);
}

// --- read the contract package ---------------------------------------------
const sources = [];
const walk = (dir) => {
  if (!fs.existsSync(dir)) return;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(full);
    else if (full.endsWith('.go')) sources.push(fs.readFileSync(full, 'utf8'));
  }
};
walk(CONTRACT_DIR);
if (!sources.length) fail(`No Go contract tests found under ${CONTRACT_DIR}/`);
const code = sources.join('\n');

// --- manifest: operationId -> test function name ----------------------------
const manifest = new Map();
const block = code.match(/coveredOperations\s*=\s*map\[string\]string\{([\s\S]*?)\n\}/);
if (!block) fail(`No coveredOperations manifest found in ${CONTRACT_DIR}/`);
for (const [, op, test] of block[1].matchAll(/"([^"]+)"\s*:\s*"([^"]+)"/g)) {
  manifest.set(op, test);
}

// --- test functions: name -> body (to detect skips) -------------------------
const tests = new Map();
const fnRe = /^func\s+(?:\([^)]*\)\s*)?(\w+)\s*\([^)]*\)[^{]*\{/gm;
const marks = [...code.matchAll(fnRe)];
marks.forEach((m, i) => {
  const start = m.index + m[0].length;
  const end = i + 1 < marks.length ? marks[i + 1].index : code.length;
  tests.set(m[1], code.slice(start, end));
});

const isSkipped = (body) => /\.Skip(f|Now)?\s*\(/.test(body);

// --- resolve every operation through the manifest ---------------------------
const unlisted = [];
const unresolved = [];
const scaffolded = [];

for (const o of operations) {
  const testName = manifest.get(o.operationId);
  if (!testName) {
    unlisted.push(o);
    continue;
  }
  const body = tests.get(testName);
  if (body === undefined) {
    unresolved.push({ ...o, testName });
    continue;
  }
  if (isSkipped(body)) scaffolded.push({ ...o, testName });
}

if (unlisted.length) {
  console.error(`Not listed in the coveredOperations manifest:`);
  for (const o of unlisted) console.error(`  ${o.operationId} (${o.m.toUpperCase()} ${o.p})`);
  fail(`${unlisted.length} operation(s) missing from the contract-test manifest.`);
}

if (unresolved.length) {
  console.error(`Manifest names a test that does not exist in ${CONTRACT_DIR}/:`);
  for (const o of unresolved) console.error(`  ${o.operationId} -> ${o.testName}()`);
  fail(`${unresolved.length} manifest entr(ies) point at a missing test.`);
}

if (scaffolded.length > MAX_SCAFFOLDED) {
  console.error(`Scaffolded (t.Skip) operations exceed the budget of ${MAX_SCAFFOLDED}:`);
  for (const o of scaffolded) console.error(`  ${o.operationId} -> ${o.testName}() [skipped]`);
  fail(`${scaffolded.length} scaffolded operation(s); write real assertions or raise the budget deliberately.`);
}

const asserted = operations.length - scaffolded.length;
console.log(
  `✓ ${operations.length} operation(s): ${asserted} asserted, ` +
    `${scaffolded.length} scaffolded (budget ${MAX_SCAFFOLDED}).`
);
for (const o of scaffolded) console.log(`  ⚠ scaffolded: ${o.operationId} -> ${o.testName}()`);
