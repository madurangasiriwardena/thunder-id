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
	"github.com/thunder-id/thunderid/internal/system/database/model"
)

const sessionColumns = `SESSION_ID, SUBJECT_ID, FLOW_ID, FLOW_VERSION, HANDLE_ID, HANDLE_ISSUED_AT, ` +
	`HANDLE_EXPIRES_AT, BINDING, ASSURANCE_LEVEL, AUTHENTICATED_AT, CREATED_AT, ` +
	`LAST_ACTIVE_AT, IDLE_EXPIRES_AT, ABSOLUTE_EXPIRES_AT, STATE, VERSION`

var (
	// QueryCreateSession inserts a new SSO session.
	QueryCreateSession = model.DBQuery{
		ID: "SSO-SESS-01",
		Query: `INSERT INTO "SSO_SESSION" (SESSION_ID, DEPLOYMENT_ID, SUBJECT_ID, FLOW_ID, FLOW_VERSION, ` +
			`HANDLE_ID, HANDLE_ISSUED_AT, HANDLE_EXPIRES_AT, BINDING, ASSURANCE_LEVEL, ` +
			`AUTHENTICATED_AT, CREATED_AT, LAST_ACTIVE_AT, IDLE_EXPIRES_AT, ABSOLUTE_EXPIRES_AT, STATE, VERSION) ` +
			`VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
	}

	// QueryGetSessionByHandle fetches a session by its opaque handle ID. Liveness checks
	// (state, deadlines, binding) are applied by the resolver, not here.
	QueryGetSessionByHandle = model.DBQuery{
		ID: "SSO-SESS-02",
		Query: `SELECT ` + sessionColumns + ` FROM "SSO_SESSION" ` +
			`WHERE HANDLE_ID = $1 AND DEPLOYMENT_ID = $2`,
	}

	// QueryUpdateSession updates the mutable fields of a session under an optimistic-lock
	// guard: it only matches when the stored VERSION equals the expected version, and it
	// bumps VERSION on success. It never touches the auth context.
	QueryUpdateSession = model.DBQuery{
		ID: "SSO-SESS-03",
		Query: `UPDATE "SSO_SESSION" SET FLOW_VERSION = $1, HANDLE_ID = $2, HANDLE_ISSUED_AT = $3, ` +
			`HANDLE_EXPIRES_AT = $4, BINDING = $5, ASSURANCE_LEVEL = $6, ` +
			`LAST_ACTIVE_AT = $7, IDLE_EXPIRES_AT = $8, ABSOLUTE_EXPIRES_AT = $9, STATE = $10, ` +
			`VERSION = VERSION + 1, UPDATED_AT = CURRENT_TIMESTAMP ` +
			`WHERE SESSION_ID = $11 AND DEPLOYMENT_ID = $12 AND VERSION = $13`,
	}

	// QueryCreateAuthContext inserts the 1:1 auth context for a session.
	QueryCreateAuthContext = model.DBQuery{
		ID: "SSO-SESS-AC-01",
		Query: `INSERT INTO "SSO_SESSION_AUTH_CONTEXT" (SESSION_ID, DEPLOYMENT_ID, CONTEXT, CONTEXT_VERSION) ` +
			`VALUES ($1, $2, $3, $4)`,
	}

	// QueryGetAuthContextBySessionID fetches a session's auth context by session id.
	QueryGetAuthContextBySessionID = model.DBQuery{
		ID: "SSO-SESS-AC-02",
		Query: `SELECT SESSION_ID, CONTEXT, CONTEXT_VERSION FROM "SSO_SESSION_AUTH_CONTEXT" ` +
			`WHERE SESSION_ID = $1 AND DEPLOYMENT_ID = $2`,
	}

	// QueryDeleteAuthContext removes a session's auth context.
	QueryDeleteAuthContext = model.DBQuery{
		ID:    "SSO-SESS-AC-03",
		Query: `DELETE FROM "SSO_SESSION_AUTH_CONTEXT" WHERE SESSION_ID = $1 AND DEPLOYMENT_ID = $2`,
	}
)
