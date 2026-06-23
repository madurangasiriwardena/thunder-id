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

// Package session provides the persistent SSO session model and relational store.
//
// A session is the unit that carries authenticated state across separate flow
// executions. It is grouped by flow: the flow ID is the group key, so only
// applications configured with the same flow can share a session (SSO). The
// session is referenced by an opaque handle, decoupled from the transport that
// carries it (a cookie is one such carrier; see the resolver).
package session

import "time"

// State represents the lifecycle state of a session.
type State string

const (
	// StateActive indicates the session is live and may back an SSO decision.
	StateActive State = "ACTIVE"
	// StateRevoked indicates the session was explicitly revoked and must not be resumed.
	StateRevoked State = "REVOKED"
	// StateEnded indicates the session ended (e.g. logout) and must not be resumed.
	StateEnded State = "ENDED"
)

// Session is the lean, hot-path SSO session entity. It carries only operational fields plus
// the aggregate assurance_level/authenticated_at used by policy checks (acr/max_age), so the
// resolve, SSO-check, and activity-touch paths never load the durable auth context.
//
// The write-once auth-event facts (completed steps + sanitized claim snapshot) live in the
// sibling AuthContext (SESSION_AUTH_CONTEXT, 1:1 by session id), loaded only on the SSO path.
type Session struct {
	// SessionID is the internal primary key, never exposed to clients.
	SessionID string
	// SubjectID is the authenticated subject (user) the session belongs to.
	SubjectID string
	// FlowID is the flow this session is grouped under (the SSO group key).
	FlowID string
	// FlowVersion is the flow definition version the session was established at.
	FlowVersion int

	// HandleID is the opaque handle that references this session. Unique per deployment.
	HandleID string
	// HandleIssuedAt is when the current handle was minted.
	HandleIssuedAt time.Time
	// HandleExpiresAt is when the current handle stops being accepted.
	HandleExpiresAt time.Time

	// Binding captures the value the handle is bound to (carrier-defined). A session is
	// only honored when the presented binding matches.
	Binding string

	// AssuranceLevel is the aggregate assurance reached when the session was established.
	AssuranceLevel string

	// AuthenticatedAt is when the subject most recently authenticated for this session.
	AuthenticatedAt time.Time
	// CreatedAt is when the session row was created.
	CreatedAt time.Time
	// LastActiveAt is refreshed each time the session backs a flow execution.
	LastActiveAt time.Time

	// IdleExpiresAt / AbsoluteExpiresAt are retained for later timeout enforcement.
	// TODO(sso): enforce idle/absolute timeouts; fields are persisted but not enforced.
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time

	// State is the lifecycle state of the session.
	State State
	// Version is the optimistic-lock token, incremented on every successful update.
	Version int
}
