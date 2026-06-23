/*
 * Copyright (c) 2025-2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package session

import (
	"encoding/json"
	"fmt"
)

// MaxAuthContextBytes bounds the serialized (pre-encryption) auth context payload, keeping the
// sibling row small and preventing unbounded growth from accumulated step facts and claims.
const MaxAuthContextBytes = 16 * 1024

// AuthContext is the durable authenticated-context sibling of a session, stored 1:1 by
// session_id in SESSION_AUTH_CONTEXT. It holds the write-once auth-event facts the SSO skip
// relies on: the completed steps keyed by node id and a minimal, safe-to-snapshot view of the
// subject and its claims. It is encrypted at rest and read only on the SSO load path — never
// touched by activity (last_active_at) updates.
//
// Aggregate facts used by hot-path policy checks (assurance_level, authenticated_at) live on
// SESSION, not here, so those checks never load this context.
type AuthContext struct {
	// SessionID is the owning session's internal id (the 1:1 key).
	SessionID string
	// Subject is the minimal subject snapshot. The subject's id lives on SESSION.subject_id;
	// only attributes needed to rehydrate the authenticated user are snapshotted here.
	Subject SubjectSnapshot
	// CompletedSteps records the completed authentication steps keyed by node id.
	CompletedSteps map[string]StepFact
	// SnapshotClaims is the sanitized, allow-listed claim set safe to persist. Mutable
	// identity attributes are intentionally NOT copied here.
	SnapshotClaims map[string]string
	// ContextVersion versions the context payload schema/content independently of the session.
	ContextVersion int
}

// SubjectSnapshot is the minimal, safe-to-snapshot subject view kept in the auth context.
type SubjectSnapshot struct {
	OUID     string `json:"ouId,omitempty"`
	UserType string `json:"userType,omitempty"`
}

// StepFact is a per-node completed authentication-step fact.
type StepFact struct {
	Executor string `json:"executor,omitempty"`
	Status   string `json:"status,omitempty"`
	// TODO(sso): enrich with per-node auth-event facts (amr, completed_at).
}

// authContextPayload is the JSON form of the auth context that is encrypted into the CONTEXT
// column. The context version is stored as its own column, not in the payload.
type authContextPayload struct {
	Subject        SubjectSnapshot     `json:"subject,omitempty"`
	CompletedSteps map[string]StepFact `json:"completedSteps,omitempty"`
	SnapshotClaims map[string]string   `json:"snapshotClaims,omitempty"`
}

// serializePayload renders the encryptable portion of the auth context to JSON.
func (c AuthContext) serializePayload() (string, error) {
	data, err := json.Marshal(authContextPayload{
		Subject:        c.Subject,
		CompletedSteps: c.CompletedSteps,
		SnapshotClaims: c.SnapshotClaims,
	})
	if err != nil {
		return "", fmt.Errorf("failed to serialize auth context: %w", err)
	}
	return string(data), nil
}

// parseAuthContextPayload parses the JSON payload of an auth context.
func parseAuthContextPayload(raw string) (authContextPayload, error) {
	var payload authContextPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return authContextPayload{}, fmt.Errorf("failed to parse auth context: %w", err)
	}
	return payload, nil
}
