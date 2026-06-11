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

import (
	"time"

	"github.com/thunder-id/thunderid/internal/sessiongroup"
)

const (
	// AssuranceLevelPlaceholder is used until real ACR derivation from the flow is added.
	AssuranceLevelPlaceholder = "placeholder"

	// BindingTypeCookieStrict marks a session whose handle is bound to a strict first-party cookie.
	BindingTypeCookieStrict = "cookie-strict"
)

// SessionMode is an alias for sessiongroup.SessionMode so callers can import just this package.
type SessionMode = sessiongroup.SessionMode

const (
	// SessionModeManaged re-exports sessiongroup.SessionModeManaged.
	SessionModeManaged = sessiongroup.SessionModeManaged
	// SessionModeSessionless re-exports sessiongroup.SessionModeSessionless.
	SessionModeSessionless = sessiongroup.SessionModeSessionless
)

// SessionState represents the lifecycle state of a session.
type SessionState string

const (
	// SessionStateActive is the only active session state in Phase A.
	SessionStateActive SessionState = "ACTIVE"
)

// BindingContext describes how a session handle is bound to the client.
type BindingContext struct {
	Type string
}

// AuthFactor records a completed authentication factor in a session.
type AuthFactor struct {
	Authenticator string `json:"authenticator"`
	AuthTime      int64  `json:"authTime"`
}

// SessionRecord is the protocol-free durable entity that represents a browser SSO session.
// SessionID is internal only; only HandleID is exposed to the client via cookie.
type SessionRecord struct {
	SessionID         string
	SubjectID         string
	SessionGroupID    string
	AuthenticatedAt   time.Time
	AssuranceLevel    string
	AuthFactors       []AuthFactor
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
