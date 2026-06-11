/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import {useQuery, type UseQueryResult} from '@tanstack/react-query';
import {useConfig} from '@thunderid/contexts';
import {useThunderID} from '@thunderid/react';

/**
 * A session group as returned by the server.
 *
 * @public
 */
export interface SessionGroup {
  id: string;
  ouId: string;
  name: string;
  sessionMode: string;
  isDefault: boolean;
}

/**
 * Response shape for listing the session groups of an organization unit.
 *
 * @public
 */
export interface SessionGroupListResponse {
  totalResults?: number;
  groups: SessionGroup[];
}

/**
 * Custom React hook to fetch the SSO session groups defined within an organization unit.
 *
 * Used by the application advanced settings to let an admin assign the application to a
 * session group (the SSO boundary). The query is disabled until `ouId` is provided.
 *
 * @param ouId - The organization unit whose session groups should be listed
 * @returns TanStack Query result containing the session group list
 *
 * @public
 */
export default function useGetSessionGroups(ouId?: string): UseQueryResult<SessionGroupListResponse> {
  const {http} = useThunderID();
  const {getServerUrl} = useConfig();

  return useQuery<SessionGroupListResponse>({
    queryKey: ['session-groups', ouId],
    enabled: Boolean(ouId),
    queryFn: async (): Promise<SessionGroupListResponse> => {
      const serverUrl: string = getServerUrl();

      const response: {
        data: SessionGroupListResponse;
      } = await http.request({
        url: `${serverUrl}/organization-units/${ouId}/session-groups`,
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
        },
      } as unknown as Parameters<typeof http.request>[0]);

      return response.data;
    },
  });
}
