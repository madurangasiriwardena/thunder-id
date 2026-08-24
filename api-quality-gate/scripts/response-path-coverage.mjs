#!/usr/bin/env node
// response-path-coverage.mjs
//
// Measures coverage in the unit the contract is actually written in: the
// (operation, status code) pair. A spec declares what each operation can return, so
// an operation is only fully covered once every declared response has been produced
// by a real request.
//
// Counting operations instead was misleading: api/group.yaml reported 10/10 while
// asserting 11 of its 43 declared responses and none of its 400s, 401s or 500s.
// "Every operation has a test" and "every operation's behaviour is covered" are very
// different claims, and only the second is worth reporting.
//
// Observed, not inferred. The evidence is what the client actually sent and received:
// testutils wraps the test HTTP client under CONTRACT_TRACE and prints one
// CONTRACT_REQ line per exchange. There is no test annotation to keep in sync and no
// way to claim a response the server never produced.
//
// The server access log is deliberately NOT used. It records only the path, so
// `?limit=1` is invisible there, and half the contract (the query parameters) cannot
// be measured from it. Observing at the client keeps the full request target.
//
// Traffic is attributed only to the resource being reported on, so seeding a user
// while testing groups does not credit the user API.
//
// Usage: node scripts/response-path-coverage.mjs --resource <name> --log <run output>
//        [--json]
// Deps: js-yaml

import fs from 'node:fs';
import path from 'node:path';
import url from 'node:url';
import yaml from 'js-yaml';

const ROOT = path.resolve(path.dirname(url.fileURLToPath(import.meta.url)), '..');
const REPO = path.resolve(ROOT, '..');
const METHODS = ['get', 'post', 'put', 'patch', 'delete', 'head', 'options'];

const arg = (n) => {
  const i = process.argv.indexOf(`--${n}`);
  return i === -1 ? undefined : process.argv[i + 1];
};
// --spec takes a repo-relative path so specs in subdirectories work too. A missing
// or absent log is not an error: it means no contract test ran for this spec, which
// is exactly 0% coverage and worth reporting as such.
const specRel = arg('spec') || (arg('resource') ? `api/${arg('resource')}.yaml` : undefined);
const logPath = arg('log');
const asJson = process.argv.includes('--json');
if (!specRel) {
  console.error('usage: response-path-coverage.mjs --spec <api/x.yaml> [--log <file>] [--json]');
  process.exit(2);
}
const resource = path.basename(specRel, '.yaml');

const specPath = path.join(REPO, specRel);
if (!fs.existsSync(specPath)) {
  console.error(`✗ no spec at ${specRel}`);
  process.exit(2);
}
const spec = yaml.load(fs.readFileSync(specPath, 'utf8')) || {};

// Query parameters are part of the promise too: a collection that declares `limit`
// and `filter` has committed to honouring them. Resolved through components so a
// $ref'd parameter counts.
const comps = (spec.components && spec.components.parameters) || {};
const deref = (x) => (x && x.$ref ? comps[x.$ref.split('/').pop()] : x);
const queryParams = (item, op) =>
  [...(item.parameters || []), ...(op.parameters || [])]
    .map(deref)
    .filter((x) => x && x.in === 'query' && x.name)
    .map((x) => x.name);

// --- declared: every (operation, status) the spec promises -------------------
const declared = [];
const params = [];
for (const [tpl, item] of Object.entries(spec.paths || {}))
  for (const m of METHODS) {
    const op = item && item[m];
    if (!op) continue;
    const opId = op.operationId || `${m} ${tpl}`;
    for (const code of Object.keys(op.responses || {}))
      declared.push({ tpl, method: m, code, operationId: opId });
    for (const name of queryParams(item, op))
      params.push({ tpl, method: m, name, operationId: opId });
  }

// A path template becomes a regex. The final template variable may contain slashes:
// /groups/tree/{path} is served as a wildcard, so "a/b" is one value, not two.
const toRegex = (tpl) => {
  const parts = tpl.split('/');
  const lastVar = parts.reduce((acc, p, i) => (/^\{.*\}$/.test(p) ? i : acc), -1);
  const body = parts
    .map((p, i) => {
      if (!/^\{.*\}$/.test(p)) return p.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      return i === lastVar && i === parts.length - 1 ? '.+' : '[^/]+';
    })
    .join('/');
  return new RegExp(`^${body}$`);
};
const templates = [...new Set(declared.map((d) => d.tpl))]
  .map((tpl) => ({ tpl, re: toRegex(tpl) }))
  // Longest first so /groups/{id}/members wins over /groups/{id}.
  .sort((a, b) => b.tpl.length - a.tpl.length);

// --- observed: what the server actually served ------------------------------
// Not anchored: the trace line is embedded in a `go test -json` Output field, so it
// is never at the start of a line in the raw log.
const TRACE = /CONTRACT_REQ (GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS) (\S+) (\d{3})/g;
const observed = new Set();
const observedParams = new Set();
const undeclared = new Set();
// The trace lines travel inside `go test -json` Output fields, so "&" arrives as
// \u0026. Decode before matching or every query string parses as one parameter.
const rawText = logPath && fs.existsSync(logPath) ? fs.readFileSync(logPath, 'utf8') : '';
const text = rawText.replace(/\\u([0-9a-fA-F]{4})/g, (_, h) =>
  String.fromCharCode(parseInt(h, 16))
);
const requests = [...text.matchAll(TRACE)].map((m) => [m[1], m[2], m[3]]);
for (const [rawMethod, target, code] of requests) {
  const method = rawMethod.toLowerCase();
  const [pathOnly, query = ''] = target.split('?');
  const raw = pathOnly.replace(/\/{2,}/g, '/').replace(/\/$/, '') || '/';
  const hit = templates.find((t) => t.re.test(raw));
  if (!hit) continue; // another resource's endpoint, e.g. seeding a user
  for (const [name] of new URLSearchParams(query))
    observedParams.add(`${method} ${hit.tpl} ${name}`);
  const key = `${method} ${hit.tpl} ${code}`;
  if (declared.some((d) => d.method === method && d.tpl === hit.tpl && d.code === code))
    observed.add(key);
  else undeclared.add(key);
}

// --- roll up per operation --------------------------------------------------
const byOp = new Map();
for (const d of declared) {
  if (!byOp.has(d.operationId))
    byOp.set(d.operationId, { operationId: d.operationId, declared: [], covered: [] });
  const e = byOp.get(d.operationId);
  e.declared.push(d.code);
  if (observed.has(`${d.method} ${d.tpl} ${d.code}`)) e.covered.push(d.code);
}
for (const p of params) {
  if (!byOp.has(p.operationId))
    byOp.set(p.operationId, { operationId: p.operationId, declared: [], covered: [] });
  const e = byOp.get(p.operationId);
  e.params = e.params || [];
  e.paramsCovered = e.paramsCovered || [];
  e.params.push(p.name);
  if (observedParams.has(`${p.method} ${p.tpl} ${p.name}`)) e.paramsCovered.push(p.name);
}
const ops = [...byOp.values()];
// Fully covered means every declared response produced AND every declared query
// parameter exercised: honouring `limit` is as much a promise as returning a 400.
const fully = ops.filter(
  (o) =>
    o.covered.length === o.declared.length &&
    (o.paramsCovered || []).length === (o.params || []).length
).length;
const result = {
  spec: specRel,
  resource,
  operations: ops.length,
  operationsFullyCovered: fully,
  responsePaths: declared.length,
  responsePathsCovered: observed.size,
  queryParams: params.length,
  queryParamsCovered: observedParams.size,
  tracedRequests: requests.length,
  undeclaredResponses: [...undeclared].sort(),
  perOperation: ops.map((o) => ({
    operationId: o.operationId,
    declared: o.declared.length,
    covered: o.covered.length,
    missing: o.declared.filter((c) => !o.covered.includes(c)).sort(),
    params: (o.params || []).length,
    paramsCovered: (o.paramsCovered || []).length,
    paramsMissing: (o.params || []).filter((n) => !(o.paramsCovered || []).includes(n)).sort(),
  })),
};

if (asJson) {
  console.log(JSON.stringify(result, null, 2));
  process.exit(0);
}
const pc = (n, d) => (d > 0 ? Math.round((n * 100) / d) : 0);
console.log(
  `${resource}: ${result.responsePathsCovered}/${result.responsePaths} declared responses ` +
    `tested (${pc(result.responsePathsCovered, result.responsePaths)}%), ` +
    `${result.queryParamsCovered}/${result.queryParams} declared query params tested ` +
    `(${pc(result.queryParamsCovered, result.queryParams)}%), ` +
    `${fully}/${ops.length} operations fully tested.`
);
if (!result.tracedRequests && logPath)
  console.log(
    '  (no CONTRACT_REQ lines found: was the run made with CONTRACT_TRACE=1?)'
  );
for (const o of result.perOperation) {
  const done = o.covered === o.declared && o.paramsCovered === o.params;
  console.log(
    `  ${done ? '✓' : ' '} ${o.operationId.padEnd(20)} responses ${o.covered}/${o.declared}` +
      (o.params ? `  params ${o.paramsCovered}/${o.params}` : '') +
      (o.missing.length ? `   missing: ${o.missing.join(', ')}` : '') +
      (o.paramsMissing.length ? `   params missing: ${o.paramsMissing.join(', ')}` : '')
  );
}
if (result.undeclaredResponses.length) {
  console.log('\n  Responses produced but NOT declared in the spec:');
  for (const u of result.undeclaredResponses) console.log(`    ${u}`);
}
