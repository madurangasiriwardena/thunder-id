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

package flowexec

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/thunder-id/thunderid/internal/flow/common"
	"github.com/thunder-id/thunderid/internal/session"
	"github.com/thunder-id/thunderid/internal/system/config"
)

func initHandlerTestConfig(t *testing.T) {
	t.Helper()
	_ = config.InitializeServerRuntime("/tmp/test-handler", &config.Config{})
	t.Cleanup(config.ResetServerRuntime)
	config.GetServerRuntime().Config.Session = config.SessionConfig{
		DefaultMode:     "managed",
		IdleTimeout:     1800,
		AbsoluteTimeout: 43200,
	}
}

func buildFlowRequest(appID, flowType string) *http.Request {
	body := FlowRequest{
		ApplicationID: appID,
		FlowType:      flowType,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/flow/execute", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestHandleFlowExecutionRequest_CompletedFlow_SetsCookie(t *testing.T) {
	initHandlerTestConfig(t)

	const handleID = "test-handle-abc"
	const sessionID = "test-session-id-should-not-appear"

	mockSvc := NewFlowExecServiceInterfaceMock(t)
	const testGroup = "test-group"
	mockSvc.On("Execute",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(&FlowStep{
		Status:        common.FlowStatusComplete,
		SessionHandle: handleID,
		SessionGroupID: testGroup,
	}, nil)

	handler := newFlowExecutionHandler(mockSvc)
	w := httptest.NewRecorder()
	handler.HandleFlowExecutionRequest(w, buildFlowRequest("app-1", "AUTHENTICATION"))

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Acceptance criterion 4: the handle must appear in Set-Cookie
	setCookie := resp.Header.Get("Set-Cookie")
	require.NotEmpty(t, setCookie, "Set-Cookie must be present for a completed flow")
	assert.Contains(t, setCookie, session.SessionCookieName(testGroup)+"="+handleID,
		"Set-Cookie must carry the session handle")

	// Acceptance criterion 4: session_id must never appear in Set-Cookie
	assert.NotContains(t, setCookie, sessionID, "session_id must never appear in Set-Cookie")

	// Acceptance criterion 4: session_id must never appear in the JSON body
	var respBody map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&respBody)
	require.NoError(t, err)
	bodyJSON, _ := json.Marshal(respBody)
	assert.NotContains(t, string(bodyJSON), sessionID, "session_id must not appear in the response body")
	assert.NotContains(t, strings.ToLower(string(bodyJSON)), "session_id",
		"sessionId / session_id field must not appear in the response JSON")
}

func TestHandleFlowExecutionRequest_CompletedFlow_NoSession(t *testing.T) {
	initHandlerTestConfig(t)

	mockSvc := NewFlowExecServiceInterfaceMock(t)
	mockSvc.On("Execute",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(&FlowStep{
		Status:        common.FlowStatusComplete,
		SessionHandle: "", // no session (sessionless group or no session service)
	}, nil)

	handler := newFlowExecutionHandler(mockSvc)
	w := httptest.NewRecorder()
	handler.HandleFlowExecutionRequest(w, buildFlowRequest("app-1", "AUTHENTICATION"))

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Set-Cookie"), "no Set-Cookie when SessionHandle is empty")
}

func TestHandleFlowExecutionRequest_IncompleteFlow_NoCookie(t *testing.T) {
	initHandlerTestConfig(t)

	mockSvc := NewFlowExecServiceInterfaceMock(t)
	mockSvc.On("Execute",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(&FlowStep{
		Status:        common.FlowStatusIncomplete,
		ExecutionID:   "exec-1",
		SessionHandle: "", // in-progress flows never have a session handle
	}, nil)

	handler := newFlowExecutionHandler(mockSvc)
	w := httptest.NewRecorder()
	handler.HandleFlowExecutionRequest(w, buildFlowRequest("app-1", "AUTHENTICATION"))

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Set-Cookie"))
}
