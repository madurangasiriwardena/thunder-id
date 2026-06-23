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

package flowexec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/thunder-id/thunderid/internal/flow/common"
	"github.com/thunder-id/thunderid/internal/flow/core"
	"github.com/thunder-id/thunderid/internal/flow/executor"
	flowmgt "github.com/thunder-id/thunderid/internal/flow/mgt"
	"github.com/thunder-id/thunderid/internal/flow/session"
	"github.com/thunder-id/thunderid/internal/system/cache"
	"github.com/thunder-id/thunderid/internal/system/config"
	"github.com/thunder-id/thunderid/internal/system/error/serviceerror"
	"github.com/thunder-id/thunderid/internal/system/log"
)

// fakeFlowProvider implements FlowProviderInterface plus GetFlow, so it satisfies the
// flowVersionLookup capability used by resolveActiveFlowVersion.
type fakeFlowProvider struct {
	def    *flowmgt.CompleteFlowDefinition
	svcErr *serviceerror.ServiceError
}

func (f *fakeFlowProvider) GetFlowByHandle(context.Context, string, common.FlowType) (
	*flowmgt.CompleteFlowDefinition, *serviceerror.ServiceError) {
	return nil, nil
}

func (f *fakeFlowProvider) GetGraph(context.Context, string) (core.GraphInterface, *serviceerror.ServiceError) {
	return nil, nil
}

func (f *fakeFlowProvider) GetFlow(context.Context, string) (
	*flowmgt.CompleteFlowDefinition, *serviceerror.ServiceError) {
	return f.def, f.svcErr
}

const testFlowID = "auth-graph-1"

func newTestGraph(t *testing.T) core.GraphInterface {
	t.Helper()
	_ = config.InitializeServerRuntime("/tmp/test-sso", &config.Config{})
	flowFactory, _ := core.Initialize(cache.Initialize(config.GetServerRuntime().Config.Cache, "test-deployment"))
	return flowFactory.CreateGraph(testFlowID, common.FlowTypeAuthentication)
}

func TestApplyInboundSSO_SelectsHandleForFlow(t *testing.T) {
	engineCtx := &EngineContext{Graph: newTestGraph(t)}

	ih := session.InboundHandle{
		Cookies: map[string]string{session.CookieName(testFlowID): "handle-1"},
		Binding: "bind-xyz",
	}
	ctx := session.WithInbound(context.Background(), ih)

	applyInboundSSO(engineCtx, ctx)

	assert.Equal(t, "handle-1", engineCtx.SSOHandleIn)
	assert.Equal(t, "bind-xyz", engineCtx.SSOBinding)
}

func TestApplyInboundSSO_NoInbound(t *testing.T) {
	engineCtx := &EngineContext{Graph: newTestGraph(t)}

	applyInboundSSO(engineCtx, context.Background())

	assert.Empty(t, engineCtx.SSOHandleIn)
	assert.Empty(t, engineCtx.SSOBinding)
}

func TestApplyInboundSSO_NilGraph(t *testing.T) {
	engineCtx := &EngineContext{}
	ctx := session.WithInbound(context.Background(),
		session.InboundHandle{Cookies: map[string]string{}, Binding: "bind"})

	applyInboundSSO(engineCtx, ctx)

	assert.Empty(t, engineCtx.SSOHandleIn)
	assert.Empty(t, engineCtx.SSOBinding)
}

func TestResolveActiveFlowVersion_FromProvider(t *testing.T) {
	svc := &flowExecService{flowProvider: &fakeFlowProvider{
		def: &flowmgt.CompleteFlowDefinition{ID: "auth-graph-1", ActiveVersion: 5},
	}}
	engineCtx := &EngineContext{Graph: newTestGraph(t)}

	version := svc.resolveActiveFlowVersion(context.Background(), engineCtx, log.GetLogger())

	assert.Equal(t, 5, version)
}

func TestResolveActiveFlowVersion_ProviderWithoutGetFlow(t *testing.T) {
	// The generated FlowProviderInterface mock does not implement GetFlow, so the version
	// lookup capability is absent and the active version resolves to 0 (unknown).
	svc := &flowExecService{flowProvider: &FlowProviderInterfaceMock{}}
	engineCtx := &EngineContext{Graph: newTestGraph(t)}

	version := svc.resolveActiveFlowVersion(context.Background(), engineCtx, log.GetLogger())

	assert.Equal(t, 0, version)
}

func TestResolveActiveFlowVersion_NilGraph(t *testing.T) {
	svc := &flowExecService{flowProvider: &fakeFlowProvider{
		def: &flowmgt.CompleteFlowDefinition{ActiveVersion: 5},
	}}

	version := svc.resolveActiveFlowVersion(context.Background(), &EngineContext{}, log.GetLogger())

	assert.Equal(t, 0, version)
}

// newGraphWithExecutor builds a single-node graph whose node is backed by the given executor.
func newGraphWithExecutor(t *testing.T, executorName string) core.GraphInterface {
	t.Helper()
	_ = config.InitializeServerRuntime("/tmp/test-sso", &config.Config{})
	flowFactory, _ := core.Initialize(cache.Initialize(config.GetServerRuntime().Config.Cache, "test-deployment"))
	graph := flowFactory.CreateGraph(testFlowID, common.FlowTypeAuthentication)
	node, err := flowFactory.CreateNode("n1", string(common.NodeTypeTaskExecution), nil, false, false)
	assert.NoError(t, err)
	node.(core.ExecutorBackedNodeInterface).SetExecutorName(executorName)
	assert.NoError(t, graph.AddNode(node))
	return graph
}

// A flow that establishes (Session) or consults (SSO-Check) a session must have its active
// version resolved on every path — including the fresh-login save path, which carries no inbound
// handle. Gating the version lookup on an inbound handle would persist sessions at version 0 and
// fail the version check on the next login.
func TestFlowUsesSSOSession(t *testing.T) {
	assert.True(t, flowUsesSSOSession(newGraphWithExecutor(t, executor.ExecutorNameSession)))
	assert.True(t, flowUsesSSOSession(newGraphWithExecutor(t, executor.ExecutorNameSSOCheck)))
	assert.False(t, flowUsesSSOSession(newGraphWithExecutor(t, executor.ExecutorNameCredentialsAuth)))
	assert.False(t, flowUsesSSOSession(nil))
}
