// Spectral custom function (tested against @stoplight/spectral 6.x).
//
// `given` MUST target a path-item object, i.e. $.paths[*], so that we can read
// BOTH path-item-level and operation-level parameters (OpenAPI does not merge
// these automatically; the effective parameter set is the union of the two).
//
// Options:
//   requireAll    : string[]  - every name must be present as a query param
//   requireOneOf  : string[]  - at least one of these must be present
//   collectionOnly: boolean   - default true; skip "item" endpoints
//
// A collection endpoint is decided by what the GET RETURNS, not by how its path
// is spelled. Path spelling is a bad proxy: `/groups/tree/{path}` ends in a
// template variable but is a paginated collection (listGroupsByPath), so a
// spelling-based rule silently skips it. The 200 response schema is authoritative;
// the path shape is only a fallback when that schema cannot be read.

const ITEM_SUFFIX = /\}\/?$/;

// Envelope fields that distinguish a list payload from an item that merely has an
// array property (a Group has `members`; only a list has `totalResults`/`links`).
// Matched by shape, not an exact allow-list, so `totalVersions` counts too.
const LIST_MARKER = /^(total|count|start|next|prev|page|offset|limit|links|cursor|has(More|Next))/i;

const jsonSchemaOf = (get) => {
  const content = ((get.responses || {})['200'] || {}).content || {};
  const key = Object.keys(content).find((k) => /json/i.test(k));
  return key ? content[key].schema : undefined;
};

// true = collection, false = item, null = undeterminable (caller falls back).
const returnsList = (get) => {
  const schema = jsonSchemaOf(get);
  if (!schema || typeof schema !== 'object') return null;
  if (schema.type === 'array' || schema.items) return true;

  const props = schema.properties;
  if (!props || typeof props !== 'object') return null;

  const entries = Object.entries(props);
  const isArray = ([, p]) => p && (p.type === 'array' || p.items);
  if (!entries.some(isArray)) return false;

  // A list either carries an envelope marker (totalResults, links, totalVersions)
  // or is a bare wrapper around the array itself ({ "languages": [...] }). An item
  // that merely owns an array (a Group with `members`) has neither.
  const hasMarker = entries.some(([name]) => LIST_MARKER.test(name));
  return hasMarker || entries.every(isArray);
};

export default (pathItem, opts = {}, context) => {
  if (!pathItem || typeof pathItem !== 'object') return;

  const url = String(context.path[context.path.length - 1]);
  const get = pathItem.get;
  if (!get) return; // only list/GET collections are subject to this rule

  const collectionOnly = opts.collectionOnly !== false;
  if (collectionOnly) {
    const isList = returnsList(get);
    if (isList === false) return;
    if (isList === null && ITEM_SUFFIX.test(url)) return;
  }

  const queryNames = [
    ...(Array.isArray(pathItem.parameters) ? pathItem.parameters : []),
    ...(Array.isArray(get.parameters) ? get.parameters : []),
  ]
    .filter((p) => p && p.in === 'query' && typeof p.name === 'string')
    .map((p) => p.name);

  const missingAll = (opts.requireAll || []).filter((n) => !queryNames.includes(n));

  let oneOfMissing = false;
  if (Array.isArray(opts.requireOneOf) && opts.requireOneOf.length > 0) {
    oneOfMissing = !opts.requireOneOf.some((n) => queryNames.includes(n));
  }

  if (missingAll.length === 0 && !oneOfMissing) return;

  const parts = [];
  if (missingAll.length) parts.push(`required: ${missingAll.join(', ')}`);
  if (oneOfMissing) parts.push(`one of: ${opts.requireOneOf.join(' | ')}`);

  return [
    {
      message: `Collection GET ${url} is missing query params (${parts.join('; ')}).`,
      path: ['paths', url, 'get'],
    },
  ];
};
