/*
 * Copyright (c) 2025-2026, WSO2 LLC. (https://www.wso2.com).
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

package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thunder-id/thunderid/internal/flow/common"
	"github.com/thunder-id/thunderid/internal/flow/core"
	"github.com/thunder-id/thunderid/internal/flow/session"
	"github.com/thunder-id/thunderid/internal/system/cache"
	"github.com/thunder-id/thunderid/internal/system/config"
)

// fakeResolver is a hand-written test double for session.Resolver.
type fakeResolver struct {
	sess *session.Session
	err  error
}

func (f *fakeResolver) Resolve(_ context.Context, handleID, _ string, _ time.Time) (*session.Session, error) {
	if handleID == "" {
		return nil, nil
	}
	return f.sess, f.err
}

func newTestSSOCheckExecutor(t *testing.T, resolver session.Resolver) *ssoCheckExecutor {
	t.Helper()
	_ = config.InitializeServerRuntime("/tmp/test-sso-exec", &config.Config{})
	flowFactory, _ := core.Initialize(cache.Initialize(config.GetServerRuntime().Config.Cache, "test-deployment"))
	return newSSOCheckExecutor(flowFactory, resolver)
}

func ssoNodeContext(handle string) *core.NodeContext {
	return &core.NodeContext{
		Context:     context.Background(),
		ExecutionID: "exec-1",
		SSO: core.SSOInputs{
			Handle:      handle,
			Binding:     "bind-xyz",
			FlowID:      "flow-1",
			FlowVersion: 3,
		},
	}
}

func liveSession() *session.Session {
	return &session.Session{
		SessionID:   "sess-1",
		HandleID:    "handle-abc",
		FlowID:      "flow-1",
		FlowVersion: 3,
		State:       session.StateActive,
	}
}

// assertAbsent asserts the Unavailable outcome: the node fails (routing to onFailure) with the
// no-live-session error, records the decision, and stashes no handle.
func assertAbsent(t *testing.T, resp *common.ExecutorResponse) {
	t.Helper()
	assert.Equal(t, common.ExecFailure, resp.Status)
	assert.NotNil(t, resp.Error)
	if resp.Error != nil {
		assert.Equal(t, ErrNoLiveSSOSession.Code, resp.Error.Code)
	}
	assert.Equal(t, "false", resp.RuntimeData[common.RuntimeKeySSOSessionPresent])
	assert.Empty(t, resp.RuntimeData[common.RuntimeKeySSOResolvedHandle])
}

func TestSSOCheck_Present(t *testing.T) {
	exec := newTestSSOCheckExecutor(t, &fakeResolver{sess: liveSession()})

	resp, err := exec.Execute(ssoNodeContext("handle-abc"))

	require.NoError(t, err)
	assert.Equal(t, common.ExecComplete, resp.Status)
	assert.Equal(t, "true", resp.RuntimeData[common.RuntimeKeySSOSessionPresent])
	assert.Equal(t, "handle-abc", resp.RuntimeData[common.RuntimeKeySSOResolvedHandle])
}

func TestSSOCheck_AbsentNoHandle(t *testing.T) {
	exec := newTestSSOCheckExecutor(t, &fakeResolver{sess: liveSession()})

	resp, err := exec.Execute(ssoNodeContext(""))

	require.NoError(t, err)
	assertAbsent(t, resp)
}

// TestSSOCheck_AbsentNoLiveSession covers resolver misses (expired / ended / binding mismatch),
// which all surface as a nil session from the resolver.
func TestSSOCheck_AbsentNoLiveSession(t *testing.T) {
	exec := newTestSSOCheckExecutor(t, &fakeResolver{sess: nil})

	resp, err := exec.Execute(ssoNodeContext("handle-abc"))

	require.NoError(t, err)
	assertAbsent(t, resp)
}

func TestSSOCheck_AbsentDifferentFlow(t *testing.T) {
	s := liveSession()
	s.FlowID = "other-flow"
	exec := newTestSSOCheckExecutor(t, &fakeResolver{sess: s})

	resp, err := exec.Execute(ssoNodeContext("handle-abc"))

	require.NoError(t, err)
	assertAbsent(t, resp)
}

func TestSSOCheck_AbsentVersionMismatch(t *testing.T) {
	s := liveSession()
	s.FlowVersion = 2 // node context expects version 3
	exec := newTestSSOCheckExecutor(t, &fakeResolver{sess: s})

	resp, err := exec.Execute(ssoNodeContext("handle-abc"))

	require.NoError(t, err)
	assertAbsent(t, resp)
}

func TestSSOCheck_AbsentOnResolverError(t *testing.T) {
	exec := newTestSSOCheckExecutor(t, &fakeResolver{err: errors.New("store down")})

	resp, err := exec.Execute(ssoNodeContext("handle-abc"))

	require.NoError(t, err)
	assertAbsent(t, resp)
}
