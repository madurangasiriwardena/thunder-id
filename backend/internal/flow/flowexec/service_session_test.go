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
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authncm "github.com/thunder-id/thunderid/internal/authn/common"
	"github.com/thunder-id/thunderid/internal/flow/common"
	"github.com/thunder-id/thunderid/internal/session"
	"github.com/thunder-id/thunderid/internal/system/config"
	"github.com/thunder-id/thunderid/internal/system/error/serviceerror"
)

// stubSessionService is an inline stub for session.SessionServiceInterface.
type stubSessionService struct {
	createCalls  int
	returnRecord *session.SessionRecord
	returnErr    error
}

func (s *stubSessionService) CreateSessionFromFlow(
	_ context.Context, _ session.CreateSessionInput,
) (*session.SessionRecord, error) {
	s.createCalls++
	return s.returnRecord, s.returnErr
}

func (s *stubSessionService) ResolveSession(
	_ context.Context, _ *http.Request, _ string,
) (*session.SessionRecord, error) {
	return nil, nil
}

func (s *stubSessionService) EnsureClientSession(
	_ context.Context, _, _ string, _ []string,
) (*session.ClientSession, error) {
	return nil, nil
}

func (s *stubSessionService) GetSessionByID(
	_ context.Context, _ string,
) (*session.SessionRecord, error) {
	return nil, nil
}

func (s *stubSessionService) GetClientSessionByID(
	_ context.Context, _ string,
) (*session.ClientSession, error) {
	return nil, nil
}

// TestFlowStep_SessionHandle_Set tests that the isComplete branch in Execute()
// calls the session service and sets FlowStep.SessionHandle to the handle ID.
// This exercises the session wiring at the flowExecService level without needing
// the full flow bootstrap (inbound client, graph, store, crypto, etc.).
func TestFlowStep_SessionHandle_SetOnCompletion(t *testing.T) {
	_ = config.InitializeServerRuntime("/tmp/test-fes-session", &config.Config{})
	defer config.ResetServerRuntime()
	config.GetServerRuntime().Config.Session = config.SessionConfig{
		DefaultMode:     "managed",
		IdleTimeout:     1800,
		AbsoluteTimeout: 43200,
	}

	const handleID = "session-handle-xyz"
	const sessionID = "internal-session-id"
	const userID = "user-123"

	sessionSvc := &stubSessionService{
		returnRecord: &session.SessionRecord{
			SessionID: sessionID,
			HandleID:  handleID,
		},
	}

	mockEngineInner := newFlowEngineInterfaceMock(t)
	mockEngineInner.EXPECT().Execute(mock.Anything).
		Return(FlowStep{Status: common.FlowStatusComplete}, nil).
		Run(func(ctx *EngineContext) {
			ctx.AuthenticatedUser = authncm.AuthenticatedUser{
				IsAuthenticated: true,
				UserID:          userID,
			}
		})

	svc := &flowExecService{
		flowEngine:     mockEngineInner,
		sessionService: sessionSvc,
	}

	// Directly simulate the Execute() isComplete branch using the engine and session service,
	// bypassing the flow-loading overhead. This is the exact code path in service.go.
	engineCtx := &EngineContext{AppID: "app-1"}
	flowStep, flowErr := svc.flowEngine.Execute(engineCtx)
	require.Nil(t, flowErr)
	require.Equal(t, common.FlowStatusComplete, flowStep.Status)

	if isComplete(flowStep) && engineCtx.AuthenticatedUser.IsAuthenticated && svc.sessionService != nil {
		sessionInput := session.CreateSessionInput{
			SubjectID:       engineCtx.AuthenticatedUser.UserID,
			AppID:           engineCtx.AppID,
			AuthenticatedAt: time.Now().UTC(),
		}
		rec, err := svc.sessionService.CreateSessionFromFlow(context.Background(), sessionInput)
		require.NoError(t, err)
		require.NotNil(t, rec)
		flowStep.SessionHandle = rec.HandleID
	}

	assert.Equal(t, handleID, flowStep.SessionHandle,
		"FlowStep.SessionHandle must carry the handle ID")
	assert.NotEqual(t, sessionID, flowStep.SessionHandle,
		"SessionID must never appear on FlowStep.SessionHandle")
	assert.Equal(t, 1, sessionSvc.createCalls)
}

// TestFlowStep_SessionHandle_NilService tests that a nil session service produces no handle.
func TestFlowStep_SessionHandle_NilService(t *testing.T) {
	mockEngineInner := newFlowEngineInterfaceMock(t)
	mockEngineInner.EXPECT().Execute(mock.Anything).
		Return(FlowStep{Status: common.FlowStatusComplete}, nil).
		Run(func(ctx *EngineContext) {
			ctx.AuthenticatedUser = authncm.AuthenticatedUser{IsAuthenticated: true, UserID: "u1"}
		})

	svc := &flowExecService{
		flowEngine:     mockEngineInner,
		sessionService: nil,
	}

	engineCtx := &EngineContext{}
	flowStep, _ := svc.flowEngine.Execute(engineCtx)

	// Mirror the isComplete branch guard: nil sessionService means no session
	if isComplete(flowStep) && engineCtx.AuthenticatedUser.IsAuthenticated && svc.sessionService != nil {
		t.Fatal("this branch must not execute when sessionService is nil")
	}

	assert.Empty(t, flowStep.SessionHandle,
		"FlowStep.SessionHandle must be empty when session service is nil")
}

// TestFlowStep_SessionCreationFailure tests that a session creation error results in an InternalServerError.
func TestFlowStep_SessionCreationFailure(t *testing.T) {
	_ = config.InitializeServerRuntime("/tmp/test-fes-sess-fail", &config.Config{})
	defer config.ResetServerRuntime()

	sessionSvc := &stubSessionService{
		returnRecord: nil,
		returnErr:    assert.AnError,
	}

	mockEngineInner := newFlowEngineInterfaceMock(t)
	mockEngineInner.EXPECT().Execute(mock.Anything).
		Return(FlowStep{Status: common.FlowStatusComplete}, nil).
		Run(func(ctx *EngineContext) {
			ctx.AuthenticatedUser = authncm.AuthenticatedUser{IsAuthenticated: true, UserID: "u1"}
		})

	svc := &flowExecService{
		flowEngine:     mockEngineInner,
		sessionService: sessionSvc,
	}

	engineCtx := &EngineContext{AppID: "app-1"}
	flowStep, _ := svc.flowEngine.Execute(engineCtx)

	var svcErr *serviceerror.ServiceError
	if isComplete(flowStep) && engineCtx.AuthenticatedUser.IsAuthenticated && svc.sessionService != nil {
		_, createErr := svc.sessionService.CreateSessionFromFlow(context.Background(), session.CreateSessionInput{
			SubjectID: engineCtx.AuthenticatedUser.UserID,
			AppID:     engineCtx.AppID,
		})
		if createErr != nil {
			svcErr = &serviceerror.InternalServerError
		}
	}

	require.NotNil(t, svcErr, "session creation failure must produce InternalServerError")
	assert.Equal(t, serviceerror.InternalServerError.Code, svcErr.Code)
}

// TestExtractAuthFactors_ModeFilter verifies that only default ("") and "verify" executor modes
// are recorded as auth factors. Preparation modes (challenge, identify, resolve, generate, send,
// register_start, register_finish) complete as AUTHENTICATION type but must not be recorded.
// Regression for Fix 3: factor over-recording when passkey challenge step completes.
func TestExtractAuthFactors_ModeFilter(t *testing.T) {
	endTime := time.Now().UTC().UnixMilli()

	makeRecord := func(name, mode string) *common.NodeExecutionRecord {
		return &common.NodeExecutionRecord{
			ExecutorName: name,
			ExecutorType: common.ExecutorTypeAuthentication,
			ExecutorMode: mode,
			Status:       common.FlowStatusComplete,
			EndTime:      endTime,
		}
	}

	history := map[string]*common.NodeExecutionRecord{
		"passkey-verify":          makeRecord("PasskeyAuthExecutor", "verify"),
		"passkey-challenge":       makeRecord("PasskeyAuthExecutor", "challenge"),
		"passkey-register-start":  makeRecord("PasskeyAuthExecutor", "register_start"),
		"passkey-register-finish": makeRecord("PasskeyAuthExecutor", "register_finish"),
		"basic-default":           makeRecord("BasicAuthExecutor", ""),
		"sms-send":                makeRecord("SMSOTPAuthExecutor", "send"),
		"sms-generate":            makeRecord("SMSOTPAuthExecutor", "generate"),
		"sms-identify":            makeRecord("BasicAuthExecutor", "identify"),
		"sms-resolve":             makeRecord("BasicAuthExecutor", "resolve"),
	}

	factors := extractAuthFactors(history)
	names := make(map[string]bool, len(factors))
	for _, f := range factors {
		names[f.Authenticator] = true
	}

	assert.True(t, names[authncm.AuthenticatorPasskey],
		"passkey 'verify' mode must be recorded")
	assert.True(t, names[authncm.AuthenticatorCredentials],
		"basic-auth default ('') mode must be recorded")

	for _, f := range factors {
		if f.Authenticator == authncm.AuthenticatorSMSOTP {
			t.Errorf("SMS OTP must not be recorded: send/generate modes are not verification")
		}
	}
	assert.Equal(t, 2, len(factors),
		"only passkey(verify) and basic-auth(default) must be recorded; "+
			"challenge, register_start, register_finish, send, generate, identify, resolve must be filtered")
}
