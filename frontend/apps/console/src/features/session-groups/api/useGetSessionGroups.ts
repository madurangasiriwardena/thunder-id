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

import {useQuery, type UseQueryResult} from '@tanstack/react-query';
import {useConfig} from '@thunderid/contexts';
import {useThunderID} from '@thunderid/react';
import {SESSION_GROUPS_QUERY_KEY} from './queryKeys';
import type {SessionGroupListResponse} from '../models/session-group';

export type {SessionGroup, SessionGroupListResponse} from '../models/session-group';

/**
 * Custom React hook to fetch all SSO session groups across the deployment.
 *
 * Used by the session-group management page and by the application advanced settings to let an
 * admin assign the application to a session group (the SSO boundary). Groups are deployment-wide;
 * apps in different OUs can share the same group for cross-OU SSO.
 *
 * @returns TanStack Query result containing the session group list
 *
 * @public
 */
export default function useGetSessionGroups(): UseQueryResult<SessionGroupListResponse> {
  const {http} = useThunderID();
  const {getServerUrl} = useConfig();

  return useQuery<SessionGroupListResponse>({
    queryKey: SESSION_GROUPS_QUERY_KEY,
    queryFn: async (): Promise<SessionGroupListResponse> => {
      const serverUrl: string = getServerUrl();

      const response: {
        data: SessionGroupListResponse;
      } = await http.request({
        url: `${serverUrl}/session-groups`,
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
        },
      } as unknown as Parameters<typeof http.request>[0]);

      return response.data;
    },
  });
}
