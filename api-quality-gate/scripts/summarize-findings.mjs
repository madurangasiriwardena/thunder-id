#!/usr/bin/env node
// summarize-findings.mjs
//
// Reads a `spectral lint -f json` payload on stdin and emits one compact JSON line
// per spec for the audit: the blocking findings themselves, not just a count, so the
// report can list what is actually wrong rather than only how much.
//
// Usage: <spectral json> | node scripts/summarize-findings.mjs --spec api/x.yaml \
//          [--waived N] [--unlintable]

const arg = (n) => {
  const i = process.argv.indexOf(`--${n}`);
  return i === -1 ? undefined : process.argv[i + 1];
};
const spec = arg('spec');
if (!spec) {
  console.error('usage: summarize-findings.mjs --spec <path> [--waived N] [--unlintable]');
  process.exit(2);
}
const waived = Number(arg('waived') || 0);

if (process.argv.includes('--unlintable')) {
  console.log(JSON.stringify({ spec, issues: null, waived, findings: [] }));
  process.exit(0);
}

let raw = '';
process.stdin.on('data', (d) => (raw += d));
process.stdin.on('end', () => {
  const m = raw.match(/\[[\s\S]*\]/);
  let all = [];
  try {
    all = m ? JSON.parse(m[0]) : [];
  } catch {
    all = [];
  }
  // severity 0 = error, 1 = warn: the two the gate treats as blocking.
  const findings = all
    .filter((f) => f.severity <= 1)
    .map((f) => ({
      code: f.code,
      severity: f.severity === 0 ? 'error' : 'warn',
      message: f.message,
      location: (f.path || []).join('.'),
    }));
  console.log(JSON.stringify({ spec, issues: findings.length, waived, findings }));
});
