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
	"time"

	"github.com/thunder-id/thunderid/internal/flow/common"
	"github.com/thunder-id/thunderid/internal/flow/core"
	"github.com/thunder-id/thunderid/internal/flow/session"
	"github.com/thunder-id/thunderid/internal/system/log"
)

const ssoCheckLoggerComponentName = "SSOCheckExecutor"

// ssoCheckExecutor resolves whether a live, compatible SSO session exists for the current
// flow and records the decision. It is the task behind the SSO-Check node and routes the
// Available/Unavailable outcomes.
//
// It depends only on the resolver (which reads the lean SESSION row); it holds no auth-context
// store, so the SSO-check hot path never loads SESSION_AUTH_CONTEXT.
type ssoCheckExecutor struct {
	core.ExecutorInterface
	resolver session.Resolver
	logger   *log.Logger
}

var _ core.ExecutorInterface = (*ssoCheckExecutor)(nil)

// newSSOCheckExecutor creates a new SSO-Check executor.
func newSSOCheckExecutor(flowFactory core.FlowFactoryInterface, resolver session.Resolver) *ssoCheckExecutor {
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, ssoCheckLoggerComponentName),
		log.String(log.LoggerKeyExecutorName, ExecutorNameSSOCheck))

	base := flowFactory.CreateExecutor(ExecutorNameSSOCheck, common.ExecutorTypeUtility,
		[]common.Input{}, []common.Input{})

	return &ssoCheckExecutor{
		ExecutorInterface: base,
		resolver:          resolver,
		logger:            logger,
	}
}

// Execute routes the SSO-Check node's two outcomes:
//   - Available (a live, compatible session): COMPLETE → onSuccess; stashes the resolved handle
//     so the Session node can load the saved flow state.
//   - Unavailable (no live session): FAILURE → onFailure, sending the flow down the
//     full-authentication path. This mirrors how identifying-style executors route a
//     "couldn't resolve" outcome; it is not a hard error.
func (e *ssoCheckExecutor) Execute(ctx *core.NodeContext) (*common.ExecutorResponse, error) {
	logger := e.logger.With(log.String(log.LoggerKeyExecutionID, ctx.ExecutionID))

	execResp := &common.ExecutorResponse{
		RuntimeData: make(map[string]string),
	}

	resolved := e.resolveSession(ctx, logger)
	if resolved != nil {
		execResp.Status = common.ExecComplete
		execResp.RuntimeData[common.RuntimeKeySSOSessionPresent] = "true"
		execResp.RuntimeData[common.RuntimeKeySSOResolvedHandle] = resolved.HandleID
		logger.Debug(ctx.Context, "Live SSO session present; routing to the Available outcome",
			log.String("flowId", ctx.SSO.FlowID))
	} else {
		execResp.Status = common.ExecFailure
		execResp.Error = &ErrNoLiveSSOSession
		execResp.RuntimeData[common.RuntimeKeySSOSessionPresent] = "false"
		logger.Debug(ctx.Context, "No live SSO session; routing to the Unavailable (full authentication) outcome")
	}

	return execResp, nil
}

// resolveSession returns the live session for the current flow, or nil when none applies:
// absent handle, no live/binding-valid session, a session from a different flow, or a
// session established at an incompatible flow version (which forces full authentication).
func (e *ssoCheckExecutor) resolveSession(ctx *core.NodeContext, logger *log.Logger) *session.Session {
	if e.resolver == nil || ctx.SSO.Handle == "" {
		return nil
	}

	s, err := e.resolver.Resolve(ctx.Context, ctx.SSO.Handle, ctx.SSO.Binding, time.Now().UTC())
	if err != nil {
		logger.Error(ctx.Context, "Failed to resolve SSO session", log.Error(err))
		return nil
	}
	if s == nil {
		return nil
	}
	if s.FlowID != ctx.SSO.FlowID {
		logger.Debug(ctx.Context, "Resolved session belongs to a different flow; ignoring")
		return nil
	}
	if s.FlowVersion != ctx.SSO.FlowVersion {
		logger.Debug(ctx.Context, "Resolved session has an incompatible flow version; forcing full authentication")
		return nil
	}

	return s
}
