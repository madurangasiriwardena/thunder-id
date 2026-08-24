#!/usr/bin/env node
// report-test-outcomes.mjs
//
// Per-resource contract-test outcomes, read from the `go test -json` event stream:
// how many tests ran, and how many failed or skipped.
//
// This reports on the TESTS. How much of the contract they cover is a different
// question, answered by response-path-coverage.mjs in the units the spec is written
// in. Keeping them apart avoids the earlier mistake of treating "an operation has a
// passing test" as coverage, which read 100% for a suite exercising 13 of 43
// declared responses.
//
// Usage: node scripts/report-test-outcomes.mjs --results <go-test-json> [--json]

import fs from 'node:fs';

const arg = (n) => {
  const i = process.argv.indexOf(`--${n}`);
  return i === -1 ? undefined : process.argv[i + 1];
};
const results = arg('results');
const asJson = process.argv.includes('--json');
if (!results) {
  console.error('usage: report-test-outcomes.mjs --results <path> [--json]');
  process.exit(2);
}

// Keyed by resource, taken from the package path (…/contract/<resource>). Only leaf
// tests are counted: a testify suite reports "TestSuite/TestCase" plus a parent
// aggregate, and counting the parent would double every result.
const byResource = new Map();
if (fs.existsSync(results)) {
  for (const line of fs.readFileSync(results, 'utf8').split('\n')) {
    if (!line.trim().startsWith('{')) continue;
    let e;
    try {
      e = JSON.parse(line);
    } catch {
      continue;
    }
    if (!e.Test || !['pass', 'fail', 'skip'].includes(e.Action)) continue;
    if (!e.Test.includes('/')) continue; // parent aggregate
    const m = /\/contract\/([^/]+)$/.exec(e.Package || '');
    if (!m) continue;
    const r = m[1];
    if (!byResource.has(r)) byResource.set(r, new Map());
    const seen = byResource.get(r);
    // A failure anywhere under a name wins over a later pass.
    if (seen.get(e.Test) === 'fail') continue;
    seen.set(e.Test, e.Action);
  }
}

const rows = [...byResource.entries()].map(([resource, tests]) => {
  const v = [...tests.values()];
  return {
    resource,
    tests: v.length,
    passed: v.filter((x) => x === 'pass').length,
    failed: v.filter((x) => x === 'fail').length,
    skipped: v.filter((x) => x === 'skip').length,
  };
});
const totals = ['tests', 'passed', 'failed', 'skipped'].reduce(
  (a, k) => ({ ...a, [k]: rows.reduce((s, r) => s + r[k], 0) }),
  {}
);

if (asJson) {
  console.log(JSON.stringify({ rows, totals }, null, 2));
  process.exit(0);
}
for (const r of rows)
  console.log(
    `${r.resource}: ${r.tests} test(s), ${r.passed} passed, ${r.failed} failed, ${r.skipped} skipped.`
  );
console.log(
  `total: ${totals.tests} test(s), ${totals.passed} passed, ${totals.failed} failed, ` +
    `${totals.skipped} skipped.`
);
