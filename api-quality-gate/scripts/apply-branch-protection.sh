#!/usr/bin/env bash
# apply-branch-protection.sh
#
# Adds the gate's required checks to the protected branch WITHOUT discarding the
# protection already in place.
#
# GitHub's branch-protection PUT replaces the whole configuration: anything absent
# from the payload is cleared. Sending governance/branch-protection.json directly
# would drop every status check the branch already requires (integration tests,
# lint, builds) and leave only the gate's six. So this reads the current
# protection, layers the policy on top, and UNIONS the required check contexts.
#
# Requires: gh (authenticated as an admin), jq.
# Usage: REPO=org/repo [BRANCH=main] [DRY_RUN=1] [FORCE=1] ./apply-branch-protection.sh
set -euo pipefail

REPO="${REPO:?set REPO=org/repo}"
BRANCH="${BRANCH:-main}"
DRY_RUN="${DRY_RUN:-0}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
POLICY="${DIR}/governance/branch-protection.json"

command -v jq >/dev/null || { echo "jq is required." >&2; exit 1; }
[[ -f "$POLICY" ]] || { echo "Policy not found: ${POLICY}" >&2; exit 1; }

api="repos/${REPO}/branches/${BRANCH}/protection"

# Current protection, normalised from the GET shape (nested {enabled:bool}) into
# the PUT shape (flat bools). An unprotected branch (404) starts from empty.
if current_raw="$(gh api "$api" 2>/dev/null)"; then
  current="$(jq '{
    required_status_checks: (
      if .required_status_checks then
        { strict: .required_status_checks.strict,
          contexts: (.required_status_checks.contexts // []) }
      else null end
    ),
    enforce_admins: (.enforce_admins.enabled // false),
    required_pull_request_reviews: (
      if .required_pull_request_reviews then
        (.required_pull_request_reviews
         | { dismiss_stale_reviews,
             require_code_owner_reviews,
             required_approving_review_count }
         + (if .require_last_push_approval == null then {}
            else { require_last_push_approval } end))
      else null end
    ),
    restrictions: (
      if .restrictions then
        { users: [.restrictions.users[]?.login],
          teams: [.restrictions.teams[]?.slug],
          apps:  [.restrictions.apps[]?.slug] }
      else null end
    ),
    required_linear_history: (.required_linear_history.enabled // false),
    allow_force_pushes: (.allow_force_pushes.enabled // false),
    allow_deletions: (.allow_deletions.enabled // false),
    block_creations: (.block_creations.enabled // false),
    required_conversation_resolution: (.required_conversation_resolution.enabled // false)
  }' <<<"$current_raw")"
  echo "Existing protection found on ${REPO}@${BRANCH}."
else
  current='{}'
  echo "No existing protection on ${REPO}@${BRANCH}; applying the policy as-is."
fi

# The policy is a FLOOR, never a ceiling: it can only tighten what the branch
# already enforces. Contexts are unioned, hardening flags OR-ed, permissive flags
# AND-ed, and the review count takes the higher of the two, so running this can
# never weaken protection that is already stricter than the policy.
merged="$(jq -n --argjson cur "$current" --argjson pol "$(cat "$POLICY")" '
  def harden($a; $b): (($a // false) or ($b // false));
  def loosen($a; $b): (($a // false) and ($b // false));
  ($cur.required_pull_request_reviews // {}) as $cr |
  ($pol.required_pull_request_reviews // {}) as $pr |
  ($cur * $pol)
  | .required_status_checks.contexts =
      ((($cur.required_status_checks.contexts // [])
        + ($pol.required_status_checks.contexts // [])) | unique)
  | .required_status_checks.strict =
      harden($cur.required_status_checks.strict; $pol.required_status_checks.strict)
  | .enforce_admins = harden($cur.enforce_admins; $pol.enforce_admins)
  | .required_linear_history = harden($cur.required_linear_history; $pol.required_linear_history)
  | .required_conversation_resolution =
      harden($cur.required_conversation_resolution; $pol.required_conversation_resolution)
  | .block_creations = harden($cur.block_creations; $pol.block_creations)
  | .allow_force_pushes = loosen($cur.allow_force_pushes; $pol.allow_force_pushes)
  | .allow_deletions = loosen($cur.allow_deletions; $pol.allow_deletions)
  # Existing push restrictions are kept when the policy does not set its own.
  | .restrictions = (if $pol.restrictions == null then $cur.restrictions else $pol.restrictions end)
  | .required_pull_request_reviews =
      (if (.required_pull_request_reviews // null) == null then null
       else .required_pull_request_reviews
         | .required_approving_review_count =
             ([($cr.required_approving_review_count // 0),
               ($pr.required_approving_review_count // 0)] | max)
         | .dismiss_stale_reviews = harden($cr.dismiss_stale_reviews; $pr.dismiss_stale_reviews)
         | .require_code_owner_reviews =
             harden($cr.require_code_owner_reviews; $pr.require_code_owner_reviews)
       end)
')"

added="$(jq -n --argjson cur "$current" --argjson m "$merged" \
  '($m.required_status_checks.contexts // []) - ($cur.required_status_checks.contexts // [])')"

echo
echo "Required status checks after apply:"
jq -r '.required_status_checks.contexts[]? | "    " + .' <<<"$merged"
echo "Newly added by this run:"
jq -r 'if length == 0 then "    (none, already required)" else .[] | "  + " + . end' <<<"$added"

if [[ "$DRY_RUN" != "0" ]]; then
  echo
  echo "DRY_RUN set; nothing written. Payload that would be sent:"
  jq . <<<"$merged"
  exit 0
fi

if [[ -t 0 && "${FORCE:-0}" == "0" ]]; then
  echo
  read -r -p "Apply to ${REPO}@${BRANCH}? [y/N] " reply
  [[ "$reply" == [yY]* ]] || { echo "Aborted."; exit 1; }
fi

jq . <<<"$merged" | gh api -X PUT "$api" --input -
printf '✓ Applied branch protection to %s@%s.\n' "$REPO" "$BRANCH"
