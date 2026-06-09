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

package session

import "time"

const (
	// ClientSessionStateActive is the only active client session state in Phase B.
	ClientSessionStateActive = "ACTIVE"
)

// ClientSession is the per-application session record that maps one SessionRecord to one
// OAuth client. It carries the OIDC sid claim and the granted scopes for that client.
// TODO token phase: add token_cnf, current_refresh_token_jti fields.
type ClientSession struct {
	ClientSessionID string
	SessionID       string
	ClientID        string
	// OIDCSID is the value emitted as the OIDC sid claim in the ID token.
	// Distinct from SessionID and HandleID; never written to cookies or response bodies.
	OIDCSID       string
	CreatedAt     time.Time
	LastUsedAt    time.Time
	Status        string
	GrantedScopes string
	Version       int
}
