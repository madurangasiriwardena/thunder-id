/*
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

// Package session provides protocol-free durable browser session management for ThunderID.
package session

import "time"

const (
	// DefaultSessionGroupID is the ID of the default session group. Every app resolves
	// here until per-group config is introduced in Phase B.
	DefaultSessionGroupID = "default"

	// AssuranceLevelPlaceholder is used until real ACR derivation from the flow is added.
	// TODO Phase B: derive real assurance level from flow ACR.
	AssuranceLevelPlaceholder = "placeholder"

	// BindingTypeCookieStrict marks a session whose handle is bound to a strict first-party cookie.
	// The struct is shaped to accommodate future key-bound binding types.
	// TODO later phases: add key-bound binding types.
	BindingTypeCookieStrict = "cookie-strict"
)

// SessionState represents the lifecycle state of a session.
type SessionState string

const (
	// SessionStateActive is the only active session state in Phase A.
	SessionStateActive SessionState = "ACTIVE"
)

// SessionMode controls whether a session is created for apps in a group.
type SessionMode string

const (
	// SessionModeManaged causes a durable SessionRecord to be created on login.
	SessionModeManaged SessionMode = "managed"
	// SessionModeSessionless disables session creation for the group.
	SessionModeSessionless SessionMode = "sessionless"
)

// BindingContext describes how a session handle is bound to the client.
// Modeled as a struct so other binding types can be added later.
type BindingContext struct {
	Type string
}

// SessionGroup is a minimal placeholder until per-group config is introduced (Phase B).
// TODO Phase B: real per-group config; for now every app maps to the default group.
type SessionGroup struct {
	ID   string
	Mode SessionMode
}

// SessionRecord is the protocol-free durable entity that represents a browser SSO session.
// SessionID is internal only; only HandleID is exposed to the client via cookie.
type SessionRecord struct {
	SessionID         string
	SubjectID         string
	SessionGroupID    string
	AuthenticatedAt   time.Time
	AssuranceLevel    string
	CreatedAt         time.Time
	LastActiveAt      time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
	HandleID          string
	HandleIssuedAt    time.Time
	HandleExpiresAt   time.Time
	Binding           BindingContext
	State             SessionState
	Version           int
}

// IsLive reports whether the session is currently active and not expired.
func (r *SessionRecord) IsLive(now time.Time) bool {
	return r.State == SessionStateActive &&
		now.Before(r.IdleExpiresAt) &&
		now.Before(r.AbsoluteExpiresAt)
}
