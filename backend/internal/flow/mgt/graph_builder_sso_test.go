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

package flowmgt

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thunder-id/thunderid/internal/flow/core"
	"github.com/thunder-id/thunderid/internal/flow/executor"
	"github.com/thunder-id/thunderid/internal/flow/interceptor"
	"github.com/thunder-id/thunderid/internal/system/cache"
	"github.com/thunder-id/thunderid/internal/system/config"
)

// TestSSOFlowDefinitionBuilds validates that the bootstrap SSO sample flow parses, references
// only registered executors, and builds into a graph with all expected nodes. It exercises the
// real executor registry so a typo'd executor name or dangling node reference fails here.
func TestSSOFlowDefinitionBuilds(t *testing.T) {
	raw, err := os.ReadFile("../../../cmd/server/bootstrap/flows/authentication/auth_flow_sso.json")
	require.NoError(t, err)

	var def CompleteFlowDefinition
	require.NoError(t, json.Unmarshal(raw, &def))
	def.ID = "auth-sso-flow-test"

	require.NoError(t, config.InitializeServerRuntime("/tmp/test-sso-flow", &config.Config{}))
	flowFactory, graphCache := core.Initialize(
		cache.Initialize(config.GetServerRuntime().Config.Cache, "test-deployment"))
	// Register only the executors this flow uses; their constructors tolerate nil services,
	// whereas some others dereference dependencies at construction time.
	registry, err := executor.Initialize(
		executor.ExecutorDependencies{FlowFactory: flowFactory},
		config.FlowConfig{Executors: []string{
			executor.ExecutorNameSSOCheck,
			executor.ExecutorNameSession,
			executor.ExecutorNameCredentialsAuth,
			executor.ExecutorNameAuthorization,
			executor.ExecutorNameAuthAssert,
		}})
	require.NoError(t, err)

	interceptorRegistry, err := interceptor.Initialize(
		interceptor.InterceptorDependencies{FlowFactory: flowFactory},
		config.FlowConfig{})
	require.NoError(t, err)

	builder := newGraphBuilder(flowFactory, registry, interceptorRegistry, graphCache)
	graph, svcErr := builder.GetGraph(context.Background(), &def)

	require.Nil(t, svcErr)
	require.NotNil(t, graph)

	for _, nodeID := range []string{
		"start", "sso_check", "prompt_credentials", "basic_auth",
		"session", "authorization_check", "auth_assert", "end",
	} {
		_, ok := graph.GetNode(nodeID)
		require.True(t, ok, "expected node %q in built graph", nodeID)
	}
}
