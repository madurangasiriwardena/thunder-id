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

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thunder-id/thunderid/internal/system/error/serviceerror"
)

// stubSvcForHandler implements SessionGroupServiceInterface with configurable ListAll behavior.
type stubSvcForHandler struct {
	listAllResp *SessionGroupListResponse
	listAllErr  *serviceerror.ServiceError
}

func (s *stubSvcForHandler) CreateSessionGroup(_ context.Context, _ CreateSessionGroupRequest) (*SessionGroup, *serviceerror.ServiceError) {
	return nil, nil
}
func (s *stubSvcForHandler) GetSessionGroup(_ context.Context, _ string) (*SessionGroup, *serviceerror.ServiceError) {
	return nil, nil
}
func (s *stubSvcForHandler) ListSessionGroupsForOU(_ context.Context, _ string) (*SessionGroupListResponse, *serviceerror.ServiceError) {
	return nil, nil
}
func (s *stubSvcForHandler) ListAllSessionGroups(_ context.Context) (*SessionGroupListResponse, *serviceerror.ServiceError) {
	return s.listAllResp, s.listAllErr
}
func (s *stubSvcForHandler) UpdateSessionGroup(_ context.Context, _ string, _ UpdateSessionGroupRequest) (*SessionGroup, *serviceerror.ServiceError) {
	return nil, nil
}
func (s *stubSvcForHandler) DeleteSessionGroup(_ context.Context, _ string) *serviceerror.ServiceError {
	return nil
}
func (s *stubSvcForHandler) ResolveGroupForClient(_ context.Context, _, _ string) (*SessionGroup, error) {
	return nil, nil
}

func TestHandleListAllRequest_Success(t *testing.T) {
	now := time.Now().UTC()
	groups := []SessionGroup{
		{ID: "g1", OUID: "ou-1", Name: "Group1", Mode: SessionModeManaged, CreatedAt: now, UpdatedAt: now},
		{ID: "g2", OUID: "ou-2", Name: "Group2", Mode: SessionModeSessionless, CreatedAt: now, UpdatedAt: now},
	}
	svc := &stubSvcForHandler{
		listAllResp: &SessionGroupListResponse{TotalResults: 2, Groups: groups},
	}
	h := newSessionGroupHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/session-groups", nil)
	w := httptest.NewRecorder()
	h.HandleListAllRequest(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp SessionGroupListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 2, resp.TotalResults)
	assert.Len(t, resp.Groups, 2)
}

func TestHandleListAllRequest_ServiceError(t *testing.T) {
	svc := &stubSvcForHandler{
		listAllErr: &serviceerror.InternalServerError,
	}
	h := newSessionGroupHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/session-groups", nil)
	w := httptest.NewRecorder()
	h.HandleListAllRequest(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
