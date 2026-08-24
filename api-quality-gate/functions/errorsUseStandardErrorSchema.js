// Spectral custom function (tested against @stoplight/spectral 6.x).
//
// `given` MUST target an operation: $.paths[*][get,post,put,patch,delete], and the
// rule MUST set `resolved: false` so this sees literal `$ref`s and can identify an
// error schema by NAME. Refs are followed manually against the unresolved document
// (43% of error responses in this repo are response-level $refs such as
// #/components/responses/Unauthorized, and those must not escape the rule).
//
// Every 4xx / 5xx / default response that declares a body must serve it as JSON
// using one of the product's agreed error schemas.
//
// RFC 9457 `application/problem+json` is deliberately NOT required. ThunderID's
// error model is a structured i18n envelope (code / message / description) served
// as `application/json`, used by 305 responses across the API surface;
// problem+json appears nowhere. Requiring it produced 556 unfixable findings that
// had to be waived on every spec, which is a rule that does no work. This rule
// enforces the convention the product actually has, so it catches the real defect
// instead: error responses that fall back to `text/plain` and a bare string, which
// clients cannot parse for an error code.
//
// Options:
//   mediaTypes      : string[] - accepted on every error response
//                                (default ['application/json'])
//   mediaTypesByCode: object   - extra media types allowed for specific status
//                                codes, e.g. { "500": ["text/plain"] }
//   schemaMediaType : string   - the media type held to `schemas`
//                                (default 'application/json')
//   schemas         : string[] - permitted schema names (default ['Error',
//                                'OAuthError']; OAuthError is RFC 6749 section 5.2)
//
// mediaTypes is a list, not a constant, because the right value is per-surface.
// ThunderID's own endpoints serve `application/json`. It is NOT a SCIM API: it
// borrows only SCIM's filter grammar (RFC 7644 section 3.4.2), and its error body
// is the bespoke i18n envelope, not SCIM's
// urn:ietf:params:scim:api:messages:2.0:Error. So SCIM's `application/scim+json`
// (RFC 7644 section 8.1) is deliberately not required here, since claiming it
// would assert protocol conformance this API does not have. A genuine SCIM
// surface should set mediaTypes: ["application/scim+json"] for its own paths.

const isErrorCode = (code) => code === 'default' || /^[45]/.test(code);

// Resolve a local JSON pointer ('#/components/responses/Unauthorized') against the
// document root. Returns undefined for external or unresolvable refs.
const deref = (node, root, seen = new Set()) => {
  let cur = node;
  while (cur && typeof cur.$ref === 'string') {
    const ref = cur.$ref;
    if (!ref.startsWith('#/') || seen.has(ref)) return undefined;
    seen.add(ref);
    cur = ref
      .slice(2)
      .split('/')
      .map((s) => s.replace(/~1/g, '/').replace(/~0/g, '~'))
      .reduce((acc, key) => (acc == null ? undefined : acc[key]), root);
  }
  return cur;
};

export default (op, opts = {}, context) => {
  if (!op || typeof op !== 'object') return;

  const root = context.document?.data;
  const url = String(context.path[1]);
  const method = String(context.path[2]).toLowerCase();
  const mediaTypes = opts.mediaTypes || ['application/json'];
  const perCode = opts.mediaTypesByCode || {};
  const schemaMediaType = opts.schemaMediaType || 'application/json';
  const allowed = opts.schemas || ['Error', 'OAuthError'];

  const problems = [];
  for (const [code, rawResp] of Object.entries(op.responses || {})) {
    if (!isErrorCode(code)) continue;

    const resp = rawResp && rawResp.$ref ? deref(rawResp, root) : rawResp;
    // An unresolvable ref is not this rule's business; oas3-valid-schema reports it.
    if (!resp) continue;

    const content = resp.content || {};
    const types = Object.keys(content);
    // A response with no body is out of scope here; declaring the standard error
    // codes at all is operation-has-standard-errors' job.
    if (types.length === 0) continue;

    // Point at the operation's own response entry so the finding lands on the
    // offending operation, not on the shared component it borrowed.
    const at = ['paths', url, method, 'responses', code];

    // Some codes accept more than the general set: a 500 may be text/plain, since
    // an unreachable service can have its response produced by infrastructure
    // rather than by the application.
    const acceptable = [...mediaTypes, ...(perCode[code] || [])];

    const served = acceptable.find((t) => content[t]);
    if (!served) {
      problems.push({
        message:
          `${method.toUpperCase()} ${url} response ${code} serves ${types.join(', ')}; ` +
          `error responses must use ${acceptable.join(' or ')} so clients can parse an error code.`,
        path: at,
      });
      continue;
    }

    // Only the structured JSON body is held to the agreed schema. problem+json is
    // RFC 9457's own shape and text/plain has no schema to speak of, so neither is
    // checked against the allow-list.
    if (served !== schemaMediaType) continue;

    const schema = deref(content[served].schema, root, new Set());
    const rawSchema = content[served].schema;
    const name =
      rawSchema && typeof rawSchema.$ref === 'string' ? rawSchema.$ref.split('/').pop() : null;

    if (!name || !allowed.includes(name)) {
      const shown = name || (schema && schema.type ? `an inline ${schema.type} schema` : 'an inline schema');
      problems.push({
        message:
          `${method.toUpperCase()} ${url} response ${code} must reference one of the ` +
          `agreed error schemas (${allowed.join(', ')}), not ${shown}.`,
        path: at,
      });
    }
  }
  return problems;
};
