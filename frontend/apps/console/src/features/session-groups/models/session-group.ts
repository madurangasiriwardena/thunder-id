/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

/**
 * SSO session mode of a session group.
 *
 * - `managed`: the deployment maintains a browser SSO session for apps in this group.
 * - `sessionless`: no SSO session is kept; every authorization runs a fresh flow.
 *
 * @public
 */
export type SessionMode = 'managed' | 'sessionless';

/** The set of selectable session modes, in display order. */
export const SESSION_MODES: readonly SessionMode[] = ['managed', 'sessionless'] as const;

/**
 * A session group as returned by the server.
 *
 * @public
 */
export interface SessionGroup {
  id: string;
  ouId: string;
  name: string;
  sessionMode: SessionMode;
  isDefault: boolean;
  createdAt?: string;
  updatedAt?: string;
}

/**
 * Response shape for listing the session groups across the deployment.
 *
 * @public
 */
export interface SessionGroupListResponse {
  totalResults?: number;
  groups: SessionGroup[];
}

/**
 * Request body for creating a session group. The OU is supplied in the body; groups are
 * deployment-wide so apps in different OUs can share one group for cross-OU SSO.
 *
 * @public
 */
export interface CreateSessionGroupRequest {
  ouId: string;
  name: string;
  sessionMode: SessionMode;
}

/**
 * Request body for updating a session group. Only the name and mode are mutable; the OU and
 * default flag are fixed at creation.
 *
 * @public
 */
export interface UpdateSessionGroupRequest {
  name: string;
  sessionMode: SessionMode;
}
