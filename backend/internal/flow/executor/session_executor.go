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
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	authnprovidercm "github.com/thunder-id/thunderid/internal/authnprovider/common"
	authnprovidermgr "github.com/thunder-id/thunderid/internal/authnprovider/manager"
	"github.com/thunder-id/thunderid/internal/flow/common"
	"github.com/thunder-id/thunderid/internal/flow/core"
	"github.com/thunder-id/thunderid/internal/flow/session"
	"github.com/thunder-id/thunderid/internal/system/cryptolib"
	"github.com/thunder-id/thunderid/internal/system/log"
	"github.com/thunder-id/thunderid/internal/system/transaction"
	sysutils "github.com/thunder-id/thunderid/internal/system/utils"
)

const sessionLoggerComponentName = "SessionExecutor"

// sessionExecutor is the task behind the Session node, which sits at the join where the SSO
// and fresh-authentication branches converge. On the fresh path it saves the lean session plus
// its 1:1 auth context and emits the handle; on the SSO path it loads the saved auth context
// into the execution context so downstream nodes continue authenticated.
//
// It is an authentication-type executor: on the SSO path the engine only adopts the loaded
// authenticated user from an authentication executor.
type sessionExecutor struct {
	core.ExecutorInterface
	store            session.StoreInterface
	authContextStore session.AuthContextStoreInterface
	transactioner    transaction.Transactioner
	authnProvider    authnprovidermgr.AuthnProviderManagerInterface
	logger           *log.Logger
}

var _ core.ExecutorInterface = (*sessionExecutor)(nil)

// newSessionExecutor creates a new Session executor. The transactioner is used to write the
// session and its auth context atomically on the fresh path. The authn provider resolves the
// subject's entity reference and attributes when saving, and is the contract downstream nodes
// use to read the subject reconstructed on the SSO load path.
func newSessionExecutor(flowFactory core.FlowFactoryInterface, store session.StoreInterface,
	authContextStore session.AuthContextStoreInterface, transactioner transaction.Transactioner,
	authnProvider authnprovidermgr.AuthnProviderManagerInterface) *sessionExecutor {
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, sessionLoggerComponentName),
		log.String(log.LoggerKeyExecutorName, ExecutorNameSession))

	base := flowFactory.CreateExecutor(ExecutorNameSession, common.ExecutorTypeAuthentication,
		[]common.Input{}, []common.Input{})

	return &sessionExecutor{
		ExecutorInterface: base,
		store:             store,
		authContextStore:  authContextStore,
		transactioner:     transactioner,
		authnProvider:     authnProvider,
		logger:            logger,
	}
}

// Execute saves or loads the session depending on the SSO-Check decision. It always completes:
// a session save/load failure degrades SSO but must not fail authentication.
func (e *sessionExecutor) Execute(ctx *core.NodeContext) (*common.ExecutorResponse, error) {
	logger := e.logger.With(log.String(log.LoggerKeyExecutionID, ctx.ExecutionID))

	execResp := &common.ExecutorResponse{
		Status:      common.ExecComplete,
		RuntimeData: make(map[string]string),
	}

	if ctx.RuntimeData[common.RuntimeKeySSOSessionPresent] == "true" {
		if err := e.loadSession(ctx, execResp, logger); err != nil {
			logger.Error(ctx.Context, "Failed to load SSO session", log.Error(err))
		}
		return execResp, nil
	}

	if err := e.saveSession(ctx, execResp, logger); err != nil {
		logger.Error(ctx.Context, "Failed to save SSO session; continuing without SSO", log.Error(err))
	}
	return execResp, nil
}

// saveSession builds a bounded, sanitized flow state from the execution context, creates a new
// session, mints and binds a handle, and emits the handle for the transport layer to set.
func (e *sessionExecutor) saveSession(ctx *core.NodeContext, execResp *common.ExecutorResponse,
	logger *log.Logger) error {
	// Always preserve the already-authenticated user; this executor does not change it on the
	// fresh path, but it is an authentication-type executor so it must echo the AuthUser back to
	// keep the engine's authenticated subject.
	execResp.AuthUser = ctx.AuthUser

	if e.authnProvider == nil || !ctx.AuthUser.IsAuthenticated() {
		logger.Debug(ctx.Context, "No authenticated subject; skipping session save")
		return nil
	}

	// Idempotency: if a session was already saved in this flow execution, re-emit its handle
	// instead of creating a duplicate.
	if existing := ctx.RuntimeData[common.RuntimeKeySSOSessionSaved]; existing != "" {
		execResp.SSOHandleOut = existing
		return nil
	}

	// Resolve the subject's entity reference and attributes from the provider so they can be
	// snapshotted into the auth context and rehydrated on the SSO load path.
	authUser, entityRef, svcErr := e.authnProvider.GetEntityReference(ctx.Context, ctx.AuthUser)
	if svcErr != nil {
		return fmt.Errorf("failed to resolve subject entity reference: %s", svcErr.ErrorDescription.DefaultValue)
	}
	authUser, attrs, svcErr := e.authnProvider.GetUserAttributes(ctx.Context, nil, nil, authUser)
	if svcErr != nil {
		return fmt.Errorf("failed to resolve subject attributes: %s", svcErr.ErrorDescription.DefaultValue)
	}
	execResp.AuthUser = authUser
	if entityRef == nil || entityRef.EntityID == "" {
		logger.Debug(ctx.Context, "No resolved subject id; skipping session save")
		return nil
	}

	sessionID, err := sysutils.GenerateUUIDv7()
	if err != nil {
		return fmt.Errorf("failed to generate session id: %w", err)
	}
	handle, err := cryptolib.GenerateSecureToken()
	if err != nil {
		return fmt.Errorf("failed to generate session handle: %w", err)
	}

	now := time.Now().UTC()

	// The durable auth context holds the write-once auth-event facts (completed steps +
	// sanitized claim snapshot). Aggregate assurance/auth_time live on the lean session.
	authContext := session.AuthContext{
		SessionID: sessionID,
		Subject: session.SubjectSnapshot{
			OUID:     entityRef.OUID,
			UserType: entityRef.EntityType,
		},
		CompletedSteps: buildCompletedSteps(ctx.ExecutionHistory),
		SnapshotClaims: session.SanitizeClaims(attributesToClaims(attrs)),
		ContextVersion: 1,
	}

	newSession := session.Session{
		SessionID:       sessionID,
		SubjectID:       entityRef.EntityID,
		FlowID:          ctx.SSO.FlowID,
		FlowVersion:     ctx.SSO.FlowVersion,
		HandleID:        handle,
		HandleIssuedAt:  now,
		HandleExpiresAt: now.Add(session.DefaultHandleTTL),
		Binding:         ctx.SSO.Binding,
		AssuranceLevel:  ctx.RuntimeData[common.RuntimeKeySelectedAuthClass],
		AuthenticatedAt: now,
		CreatedAt:       now,
		LastActiveAt:    now,
		// Idle/absolute deadlines are kept on the model but not set here: timeout enforcement
		// is out of scope for this POC.
		State:   session.StateActive,
		Version: 1,
	}

	// Write the auth context and the session in one transaction so a session never exists
	// without its context (and neither persists if either write fails).
	saveErr := e.transactioner.Transact(ctx.Context, func(txCtx context.Context) error {
		if err := e.authContextStore.Create(txCtx, authContext); err != nil {
			return err
		}
		return e.store.Create(txCtx, newSession)
	})
	if saveErr != nil {
		return saveErr
	}

	execResp.RuntimeData[common.RuntimeKeySSOSessionSaved] = handle
	execResp.SSOHandleOut = handle
	logger.Debug(ctx.Context, "Saved SSO session", log.String("flowId", ctx.SSO.FlowID))
	return nil
}

// loadSession loads the saved flow state of the resolved session into the execution context so
// downstream nodes continue with the authenticated subject and claims, and refreshes the
// session's last-active timestamp.
func (e *sessionExecutor) loadSession(ctx *core.NodeContext, execResp *common.ExecutorResponse,
	logger *log.Logger) error {
	handle := ctx.RuntimeData[common.RuntimeKeySSOResolvedHandle]
	if handle == "" {
		return fmt.Errorf("no resolved session handle to load")
	}

	s, err := e.store.GetByHandle(ctx.Context, handle)
	if err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("resolved session no longer exists")
	}

	// Lazily load the durable auth context (only the load path reads it).
	authContext, err := e.authContextStore.GetBySessionID(ctx.Context, s.SessionID)
	if err != nil {
		return err
	}
	if authContext == nil {
		return fmt.Errorf("auth context for resolved session no longer exists")
	}

	// Rehydrate a fully-resolved AuthUser (entity reference + attributes, no provider tokens) from
	// the snapshot so downstream nodes read the subject through the normal provider contract
	// without performing a fresh authentication.
	authUser, err := buildResolvedAuthUser(
		authnprovidercm.EntityReference{
			EntityID:   s.SubjectID,
			EntityType: authContext.Subject.UserType,
			OUID:       authContext.Subject.OUID,
		},
		authContext.SnapshotClaims,
	)
	if err != nil {
		return err
	}
	execResp.AuthUser = authUser
	// Aggregate assurance/auth_time come from the lean session, not the context.
	if s.AssuranceLevel != "" {
		execResp.RuntimeData[common.RuntimeKeySelectedAuthClass] = s.AssuranceLevel
	}
	if !s.AuthenticatedAt.IsZero() {
		execResp.RuntimeData[common.RuntimeKeyAuthTime] = strconv.FormatInt(s.AuthenticatedAt.Unix(), 10)
	}

	// Refresh last-active timestamp under the optimistic-lock guard — touches SESSION only,
	// never the auth context. A conflict here is non-fatal: the session loaded successfully.
	s.LastActiveAt = time.Now().UTC()
	if updErr := e.store.Update(ctx.Context, s); updErr != nil {
		logger.Warn(ctx.Context, "Failed to refresh session last-active timestamp", log.Error(updErr))
	}

	logger.Debug(ctx.Context, "Loaded SSO session", log.String("flowId", ctx.SSO.FlowID))
	return nil
}

// buildCompletedSteps projects the execution history into the bounded per-node step facts
// kept in the auth context.
func buildCompletedSteps(history map[string]*common.NodeExecutionRecord) map[string]session.StepFact {
	if len(history) == 0 {
		return nil
	}
	steps := make(map[string]session.StepFact, len(history))
	for nodeID, record := range history {
		if record == nil {
			continue
		}
		steps[nodeID] = session.StepFact{
			Executor: record.ExecutorName,
			Status:   string(record.Status),
		}
	}
	return steps
}

// attributesToClaims flattens a provider attributes response into a claim map for snapshotting.
func attributesToClaims(attrs *authnprovidercm.AttributesResponse) map[string]interface{} {
	if attrs == nil {
		return nil
	}
	claims := make(map[string]interface{}, len(attrs.Attributes))
	for name, attr := range attrs.Attributes {
		if attr != nil {
			claims[name] = attr.Value
		}
	}
	return claims
}

// buildResolvedAuthUser constructs a fully-resolved AuthUser (entity reference + attributes,
// with no outstanding provider tokens) from a stored subject snapshot. Because the tokens are
// nil, the manager's GetEntityReference/GetUserAttributes return these values directly without a
// provider round-trip, which is exactly what the SSO load path needs. AuthUser's fields are
// unexported, so it is populated through its JSON contract.
func buildResolvedAuthUser(ref authnprovidercm.EntityReference,
	claims map[string]string) (authnprovidermgr.AuthUser, error) {
	attrs := authnprovidercm.AttributesResponse{
		Attributes: make(map[string]*authnprovidercm.AttributeResponse, len(claims)),
	}
	for name, value := range claims {
		attrs.Attributes[name] = &authnprovidercm.AttributeResponse{Value: value}
	}

	payload := struct {
		EntityReferenceToken any                                 `json:"entityReferenceToken"`
		EntityReference      *authnprovidercm.EntityReference    `json:"entityReference,omitempty"`
		AttributeToken       any                                 `json:"attributeToken"`
		Attributes           *authnprovidercm.AttributesResponse `json:"attributes,omitempty"`
	}{
		EntityReference: &ref,
		Attributes:      &attrs,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return authnprovidermgr.AuthUser{}, fmt.Errorf("failed to marshal resolved auth user: %w", err)
	}
	var authUser authnprovidermgr.AuthUser
	if err := json.Unmarshal(raw, &authUser); err != nil {
		return authnprovidermgr.AuthUser{}, fmt.Errorf("failed to build resolved auth user: %w", err)
	}
	return authUser, nil
}
