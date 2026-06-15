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

import {useMutation, useQueryClient, type UseMutationResult} from '@tanstack/react-query';
import {useConfig, useToast} from '@thunderid/contexts';
import {useThunderID} from '@thunderid/react';
import {useTranslation} from 'react-i18next';
import {SESSION_GROUPS_QUERY_KEY, sessionGroupQueryKey} from './queryKeys';
import type {SessionGroup, UpdateSessionGroupRequest} from '../models/session-group';

export interface UpdateSessionGroupVariables {
  sessionGroupId: string;
  data: UpdateSessionGroupRequest;
}

/**
 * Custom React hook to update an existing session group.
 *
 * @returns TanStack Query mutation object for updating session groups
 *
 * @public
 */
export default function useUpdateSessionGroup(): UseMutationResult<SessionGroup, Error, UpdateSessionGroupVariables> {
  const {http} = useThunderID();
  const {getServerUrl} = useConfig();
  const queryClient: ReturnType<typeof useQueryClient> = useQueryClient();
  const {t} = useTranslation('sessionGroups');
  const {showToast} = useToast();

  return useMutation<SessionGroup, Error, UpdateSessionGroupVariables>({
    mutationFn: async ({sessionGroupId, data}: UpdateSessionGroupVariables): Promise<SessionGroup> => {
      const serverUrl: string = getServerUrl();
      const response: {data: SessionGroup} = await http.request({
        url: `${serverUrl}/session-groups/${sessionGroupId}`,
        method: 'PUT',
        headers: {'Content-Type': 'application/json'},
        data,
      } as unknown as Parameters<typeof http.request>[0]);

      return response.data;
    },
    onSuccess: (_data, {sessionGroupId}) => {
      queryClient.invalidateQueries({queryKey: sessionGroupQueryKey(sessionGroupId)}).catch(() => {
        /* noop */
      });
      queryClient.invalidateQueries({queryKey: SESSION_GROUPS_QUERY_KEY}).catch(() => {
        /* noop */
      });
      showToast(t('update.success'), 'success');
    },
    onError: () => {
      showToast(t('update.error'), 'error');
    },
  });
}
