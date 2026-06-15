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

import {waitFor, renderHook} from '@thunderid/test-utils';
import {describe, it, expect, beforeEach, afterEach, vi} from 'vitest';
import type {CreateSessionGroupRequest, SessionGroup} from '../../models/session-group';
import {SESSION_GROUPS_QUERY_KEY} from '../queryKeys';
import useCreateSessionGroup from '../useCreateSessionGroup';

vi.mock('@thunderid/react', () => ({
  useThunderID: vi.fn(),
}));

vi.mock('@thunderid/contexts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@thunderid/contexts')>();
  return {
    ...actual,
    useConfig: vi.fn(),
    useToast: vi.fn(),
  };
});

const {useThunderID} = await import('@thunderid/react');
const {useConfig, useToast} = await import('@thunderid/contexts');

describe('useCreateSessionGroup', () => {
  let mockHttpRequest: ReturnType<typeof vi.fn>;
  let mockGetServerUrl: ReturnType<typeof vi.fn>;
  let mockShowToast: ReturnType<typeof vi.fn>;

  const mockCreated: SessionGroup = {
    id: 'sg-1',
    name: 'My Group',
    ouId: 'ou-1',
    sessionMode: 'managed',
    isDefault: false,
  };

  const mockRequest: CreateSessionGroupRequest = {
    name: 'My Group',
    ouId: 'ou-1',
    sessionMode: 'managed',
  };

  beforeEach(() => {
    mockHttpRequest = vi.fn();
    mockGetServerUrl = vi.fn().mockReturnValue('https://api.test.com');
    mockShowToast = vi.fn();

    vi.mocked(useThunderID).mockReturnValue({
      http: {request: mockHttpRequest},
    } as unknown as ReturnType<typeof useThunderID>);

    vi.mocked(useConfig).mockReturnValue({
      getServerUrl: mockGetServerUrl,
    } as unknown as ReturnType<typeof useConfig>);

    vi.mocked(useToast).mockReturnValue({
      showToast: mockShowToast,
    } as unknown as ReturnType<typeof useToast>);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('should successfully create a session group', async () => {
    mockHttpRequest.mockResolvedValueOnce({data: mockCreated});

    const {result} = renderHook(() => useCreateSessionGroup());

    result.current.mutate(mockRequest);

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockCreated);
  });

  it('should make correct API call', async () => {
    mockHttpRequest.mockResolvedValueOnce({data: mockCreated});

    const {result} = renderHook(() => useCreateSessionGroup());

    result.current.mutate(mockRequest);

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(mockHttpRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        url: 'https://api.test.com/session-groups',
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        data: mockRequest,
      }),
    );
  });

  it('should handle API error', async () => {
    const apiError = new Error('Failed to create session group');
    mockHttpRequest.mockRejectedValueOnce(apiError);

    const {result} = renderHook(() => useCreateSessionGroup());

    result.current.mutate(mockRequest);

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });

    expect(result.current.error).toEqual(apiError);
    expect(mockShowToast).toHaveBeenCalledWith(expect.any(String), 'error');
  });

  it('should invalidate the session groups cache on success', async () => {
    mockHttpRequest.mockResolvedValueOnce({data: mockCreated});

    const {result, queryClient} = renderHook(() => useCreateSessionGroup());
    const invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries');

    result.current.mutate(mockRequest);

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(invalidateQueriesSpy).toHaveBeenCalledWith(
      expect.objectContaining({queryKey: SESSION_GROUPS_QUERY_KEY}),
    );
  });
});
