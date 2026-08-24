#!/usr/bin/env node
// render-audit.mjs
//
// Renders the audit as one table with three groups of columns, because three
// different questions get asked of it:
//
//   DESIGN    does the spec declare what it should?
//   TESTS     did the contract tests run and pass?
//   COVERAGE  how much of what the spec declares did they actually exercise?
//
// Coverage is counted in declared contract elements, (operation, status) pairs and
// query parameters, not operations. Counting operations overstated it badly:
// api/group.yaml read 10/10 and 100% while producing 13 of its 43 declared responses
// and exercising 4 of its 10 declared parameters.
//
// Usage: node scripts/render-audit.mjs --lint <jsonl> --paths <jsonl> [--tests <json>]

import fs from 'node:fs';

const arg = (n) => {
  const i = process.argv.indexOf(`--${n}`);
  return i === -1 ? undefined : process.argv[i + 1];
};
const lintPath = arg('lint');
const pathsPath = arg('paths');
const testsPath = arg('tests');
if (!lintPath || !pathsPath) {
  console.error('usage: render-audit.mjs --lint <jsonl> --paths <jsonl> [--tests <json>]');
  process.exit(2);
}

const readJsonl = (p) =>
  fs
    .readFileSync(p, 'utf8')
    .split('\n')
    .filter((l) => l.trim())
    .map((l) => JSON.parse(l));

const lint = readJsonl(lintPath);
const paths = new Map(readJsonl(pathsPath).map((r) => [r.spec, r]));

const tests = new Map();
let testTotals = { tests: 0, failed: 0, skipped: 0 };
if (testsPath && fs.existsSync(testsPath)) {
  const t = JSON.parse(fs.readFileSync(testsPath, 'utf8'));
  for (const r of t.rows || []) tests.set(r.resource, r);
  testTotals = { ...testTotals, ...(t.totals || {}) };
}

const pct = (n, d) => (d > 0 ? Math.round((n * 100) / d) : 0);
const ratio = (n, d) => (d > 0 ? `${n}/${d} · ${pct(n, d)}%` : '—');

const out = [];
out.push('## API design audit');
out.push('');
// An HTML table, not markdown: markdown allows a single header row with no colspan,
// so the group each column belongs to could not be shown. Job summaries render HTML,
// so the real two-level header is used and every number keeps its own column.
out.push('<table>');
out.push('<thead>');
out.push(
  '<tr>' +
    '<th rowspan="2" align="left">spec</th>' +
    '<th colspan="2">design</th>' +
    '<th rowspan="2">operations</th>' +
    '<th colspan="3">contract tests</th>' +
    '<th colspan="2">responses</th>' +
    '<th colspan="2">query params</th>' +
    '<th rowspan="2">%<br>tested</th>' +
    '</tr>'
);
out.push(
  '<tr>' +
    '<th>issues</th><th>waived</th>' +
    '<th>run</th><th>failed</th><th>skipped</th>' +
    '<th>declared</th><th>tested</th>' +
    '<th>declared</th><th>tested</th>' +
    '</tr>'
);
out.push('</thead>');
out.push('<tbody>');

const tot = { issues: 0, waived: 0, ops: 0, opsFull: 0, resp: 0, respCov: 0, par: 0, parCov: 0 };
let unlintable = 0;

for (const r of lint) {
  const spec = r.spec;
  const resource = spec.replace(/^.*\//, '').replace(/\.yaml$/, '');
  const c = paths.get(spec) || {};
  const t = tests.get(resource) || { tests: 0, failed: 0, skipped: 0 };

  if (r.issues === null) unlintable++;
  else tot.issues += r.issues;
  tot.waived += r.waived || 0;
  tot.ops += c.operations || 0;
  tot.opsFull += c.operationsFullyCovered || 0;
  tot.resp += c.responsePaths || 0;
  tot.respCov += c.responsePathsCovered || 0;
  tot.par += c.queryParams || 0;
  tot.parCov += c.queryParamsCovered || 0;

  const dash = '<td align="right">&mdash;</td>';
  const num = (v) => `<td align="right">${v}</td>`;
  const declaredTotal = (c.responsePaths || 0) + (c.queryParams || 0);
  const coveredTotal = (c.responsePathsCovered || 0) + (c.queryParamsCovered || 0);
  out.push(
    '<tr>' +
      `<td><code>${spec}</code></td>` +
      (r.issues === null
        ? '<td align="right"><strong>NOT LINTABLE</strong></td>'
        : num(r.issues)) +
      num(r.waived || 0) +
      num(c.operations || 0) +
      (t.tests ? num(t.tests) + num(t.failed) + num(t.skipped) : dash + dash + dash) +
      num(c.responsePaths || 0) +
      num(c.responsePathsCovered || 0) +
      num(c.queryParams || 0) +
      num(c.queryParamsCovered || 0) +
      num(declaredTotal ? `${pct(coveredTotal, declaredTotal)}%` : '&mdash;') +
      '</tr>'
  );
}

out.push('</tbody>');
out.push('</table>');
out.push('');
out.push('### Totals');
out.push('');
out.push(
  `- **Design:** ${tot.issues} issues` +
    (tot.waived ? `, plus ${tot.waived} suppressed by a tracked, expiring waiver` : ', none waived') +
    (unlintable ? `. **${unlintable} spec(s) could not be linted; the gate is blind to them.**` : '.')
);
out.push(
  `- **Tests:** ${testTotals.tests} contract test(s) run, ` +
    `${testTotals.failed} failed, ${testTotals.skipped} skipped.`
);
out.push(
  `- **Coverage:** ${tot.respCov}/${tot.resp} declared responses tested; ` +
    `${tot.parCov}/${tot.par} declared query parameters tested ` +
    `(${pct(tot.respCov + tot.parCov, tot.resp + tot.par)}% of the declared contract).`
);
out.push('');
out.push(
  '> An operation is fully covered only when every response its spec declares has been ' +
    'produced AND every query parameter it declares has been exercised, observed from ' +
    'what the test client sent and received. Producing a response is weaker than ' +
    'asserting it is correct, and declaring a capability is not implementing it: the ' +
    'rules require a collection to declare `filter` and `sort`, and a server that ' +
    'ignores them still passes. Read this as the floor.'
);

// --- per-spec detail, collapsed ---------------------------------------------
// Everything the summary row aggregates, spelled out: which design issues, which
// operations, and for each the responses and parameters the spec declares against
// the ones actually exercised. Collapsed because api/connections.yaml alone carries
// 154 findings, and nobody wants that expanded by default.
out.push('');
out.push('## Detail by spec');
out.push('');

for (const r of lint) {
  const spec = r.spec;
  const c = paths.get(spec) || {};
  const ops = c.perOperation || [];
  const declaredTotal = (c.responsePaths || 0) + (c.queryParams || 0);
  const coveredTotal = (c.responsePathsCovered || 0) + (c.queryParamsCovered || 0);

  const headline =
    `${r.issues === null ? 'NOT LINTABLE' : `${r.issues} design issue(s)`}` +
    ` · ${c.responsePathsCovered || 0}/${c.responsePaths || 0} responses tested` +
    ` · ${c.queryParamsCovered || 0}/${c.queryParams || 0} params tested` +
    (declaredTotal ? ` · ${pct(coveredTotal, declaredTotal)}% covered` : '');

  out.push('<details>');
  out.push(`<summary><code>${spec}</code> &mdash; ${headline}</summary>`);
  out.push('');

  // 1. design issues
  out.push(`**Design issues (${r.issues === null ? 'not lintable' : r.issues})**`);
  out.push('');
  if (r.issues === null) {
    out.push('This spec could not be linted, so the gate is blind to it.');
  } else if (!r.findings || !r.findings.length) {
    out.push('None.');
  } else {
    out.push('| severity | rule | location | message |');
    out.push('|---|---|---|---|');
    for (const f of r.findings)
      out.push(
        `| ${f.severity} | \`${f.code}\` | \`${f.location || '(document)'}\` | ${f.message.replace(/\|/g, '\\|')} |`
      );
  }
  out.push('');

  // 2/3/4. operations, with declared vs covered responses and parameters
  out.push(`**Operations (${ops.length})**`);
  out.push('');
  if (!ops.length) {
    out.push('No operations found, or no contract package for this resource.');
  } else {
    out.push('| operation | responses declared | tested | not tested | params declared | tested | not tested |');
    out.push('|---|---:|---:|---|---:|---:|---|');
    for (const o of ops)
      out.push(
        `| \`${o.operationId}\` | ${o.declared} | ${o.covered} | ` +
          `${o.missing.length ? o.missing.join(', ') : '&mdash;'} | ` +
          `${o.params || 0} | ${o.paramsCovered || 0} | ` +
          `${(o.paramsMissing || []).length ? o.paramsMissing.join(', ') : '&mdash;'} |`
      );
  }
  out.push('');
  out.push('</details>');
  out.push('');
}

console.log(out.join('\n'));
