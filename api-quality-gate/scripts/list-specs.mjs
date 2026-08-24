#!/usr/bin/env node
// list-specs.mjs
//
// Prints the specs the gate validates, one repo-relative path per line: every
// api/*.yaml that is NOT named in governance/excluded-specs.yaml.
//
// The list is a DENY list on purpose. An allow list would mean a newly added spec
// is ungated by default, and omission is silent, so nothing would catch it. Here
// a new spec is validated because nobody did anything, and opting out requires an
// explicit entry in a governed file.
//
// Usage: node scripts/list-specs.mjs
// Deps: js-yaml

import fs from 'node:fs';
import path from 'node:path';
import url from 'node:url';
import yaml from 'js-yaml';

const ROOT = path.resolve(path.dirname(url.fileURLToPath(import.meta.url)), '..');
const REPO = path.resolve(ROOT, '..');
const API_DIR = path.join(REPO, 'api');
const EXCLUDED = path.join(ROOT, 'governance', 'excluded-specs.yaml');

const fail = (msg) => {
  console.error(`✗ ${msg}`);
  process.exit(1);
};

const doc = yaml.load(fs.readFileSync(EXCLUDED, 'utf8')) || {};
const excluded = doc.excluded || [];
if (!Array.isArray(excluded)) fail('excluded-specs.yaml: `excluded` must be a list of spec paths.');

// Walk recursively: a spec in a subdirectory (api/extensions/) is still a spec,
// and a top-level-only scan would let it escape the gate entirely.
const walk = (dir) =>
  fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) return walk(full);
    return e.name.endsWith('.yaml') ? [path.relative(REPO, full)] : [];
  });

const all = walk(API_DIR).sort();

// A stale entry means the list is lying about what it covers, so say so loudly
// rather than silently ignoring it.
const stale = excluded.filter((s) => !all.includes(s));
if (stale.length) fail(`excluded-specs.yaml names spec(s) that do not exist: ${stale.join(', ')}`);

for (const spec of all.filter((s) => !excluded.includes(s))) console.log(spec);
