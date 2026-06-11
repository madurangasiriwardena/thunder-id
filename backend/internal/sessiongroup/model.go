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

package sessiongroup

import "time"

// SessionMode controls whether a session is created for apps in a group.
type SessionMode string

const (
	// SessionModeManaged causes a durable session record to be created on login.
	SessionModeManaged SessionMode = "managed"
	// SessionModeSessionless disables session creation for the group.
	SessionModeSessionless SessionMode = "sessionless"
)

// DeploymentDefaultGroupID is the sentinel ID for the implicit deployment-level default session group.
// It is not a UUIDv7 so it can never collide with a real group ID.
// Apps that have no explicit session group assigned are bucketed into this group.
const DeploymentDefaultGroupID = "deployment-default"

// SessionGroup is the SSO boundary entity.
// Apps assigned to the same group share a session; apps in different groups do not,
// even within the same OU. Cross-OU SSO is possible by assigning apps in different
// OUs to the same group, or by leaving them both unassigned (both resolve to the
// deployment-level default).
type SessionGroup struct {
	ID        string      `json:"id"`
	OUID      string      `json:"ouId"`
	Name      string      `json:"name"`
	Mode      SessionMode `json:"sessionMode"`
	IsDefault bool        `json:"isDefault"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

// CreateSessionGroupRequest is the request body for creating a session group.
type CreateSessionGroupRequest struct {
	OUID      string      `json:"ouId"`
	Name      string      `json:"name"`
	Mode      SessionMode `json:"sessionMode"`
	IsDefault bool        `json:"isDefault,omitempty"`
}

// UpdateSessionGroupRequest is the request body for updating a session group.
type UpdateSessionGroupRequest struct {
	Name string      `json:"name"`
	Mode SessionMode `json:"sessionMode"`
}

// SessionGroupListResponse is the response for listing session groups.
type SessionGroupListResponse struct {
	TotalResults int            `json:"totalResults"`
	Groups       []SessionGroup `json:"groups"`
}
