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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authnprovidercm "github.com/thunder-id/thunderid/internal/authnprovider/common"
	authnprovidermgr "github.com/thunder-id/thunderid/internal/authnprovider/manager"
	"github.com/thunder-id/thunderid/internal/flow/common"
	"github.com/thunder-id/thunderid/internal/flow/core"
	"github.com/thunder-id/thunderid/internal/flow/session"
	"github.com/thunder-id/thunderid/internal/system/cache"
	"github.com/thunder-id/thunderid/internal/system/config"
	"github.com/thunder-id/thunderid/internal/system/transaction"
	"github.com/thunder-id/thunderid/tests/mocks/authnprovider/managermock"
)

// txStub is a test Transactioner that runs the function and models commit/rollback: on error
// it invokes onRollback (which test code uses to discard the fakes' staged writes).
type txStub struct {
	calls      int
	committed  bool
	onRollback func()
}

func (s *txStub) Transact(ctx context.Context, fn func(context.Context) error) error {
	s.calls++
	if err := fn(ctx); err != nil {
		if s.onRollback != nil {
			s.onRollback()
		}
		return err
	}
	s.committed = true
	return nil
}

var _ transaction.Transactioner = (*txStub)(nil)

// fakeStore is a hand-written test double for session.StoreInterface.
type fakeStore struct {
	created   *session.Session
	createErr error
	getResult *session.Session
	getErr    error
	updated   *session.Session
	updateErr error
}

func (f *fakeStore) Create(_ context.Context, s session.Session) error {
	f.created = &s
	return f.createErr
}

func (f *fakeStore) GetByHandle(_ context.Context, _ string) (*session.Session, error) {
	return f.getResult, f.getErr
}

func (f *fakeStore) Update(_ context.Context, s *session.Session) error {
	f.updated = s
	return f.updateErr
}

// fakeAuthContextStore is a hand-written test double for session.AuthContextStoreInterface.
type fakeAuthContextStore struct {
	created      *session.AuthContext
	createErr    error
	getResult    *session.AuthContext
	getErr       error
	getCalls     int
	gotSessionID string
	deletedID    string
	deleteErr    error
}

func (f *fakeAuthContextStore) Create(_ context.Context, c session.AuthContext) error {
	f.created = &c
	return f.createErr
}

func (f *fakeAuthContextStore) GetBySessionID(_ context.Context, sessionID string) (*session.AuthContext, error) {
	f.getCalls++
	f.gotSessionID = sessionID
	return f.getResult, f.getErr
}

func (f *fakeAuthContextStore) Delete(_ context.Context, sessionID string) error {
	f.deletedID = sessionID
	return f.deleteErr
}

// authenticatedAuthUser returns an AuthUser that reports IsAuthenticated() == true. Its tokens
// are opaque; the resolved subject is supplied by the mocked provider in each test.
func authenticatedAuthUser() authnprovidermgr.AuthUser {
	var authUser authnprovidermgr.AuthUser
	_ = authUser.UnmarshalJSON([]byte(`{"entityReferenceToken":"tok","attributeToken":"tok"}`))
	return authUser
}

// saveAuthnMock returns a provider that resolves the fresh-save subject (user-1 / ou-1 / person)
// and its attributes. Expectations are optional so guard-short-circuit tests can reuse it.
func saveAuthnMock(t *testing.T) *managermock.AuthnProviderManagerInterfaceMock {
	t.Helper()
	m := managermock.NewAuthnProviderManagerInterfaceMock(t)
	resolved := authenticatedAuthUser()
	m.On("GetEntityReference", mock.Anything, mock.Anything).
		Return(resolved, &authnprovidercm.EntityReference{EntityID: "user-1", OUID: "ou-1", EntityType: "person"}, nil).
		Maybe()
	m.On("GetUserAttributes", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(resolved, &authnprovidercm.AttributesResponse{Attributes: map[string]*authnprovidercm.AttributeResponse{
			"email":    {Value: "alice@example.com"},
			"name":     {Value: "Alice"},
			"password": {Value: "super-secret"},
		}}, nil).
		Maybe()
	return m
}

func newTestSessionExecutor(t *testing.T, store session.StoreInterface,
	authCtxStore session.AuthContextStoreInterface,
	authn authnprovidermgr.AuthnProviderManagerInterface) *sessionExecutor {
	t.Helper()
	return newTestSessionExecutorWithTx(t, store, authCtxStore, &txStub{}, authn)
}

func newTestSessionExecutorWithTx(t *testing.T, store session.StoreInterface,
	authCtxStore session.AuthContextStoreInterface, tx transaction.Transactioner,
	authn authnprovidermgr.AuthnProviderManagerInterface) *sessionExecutor {
	t.Helper()
	_ = config.InitializeServerRuntime("/tmp/test-session-exec", &config.Config{})
	flowFactory, _ := core.Initialize(cache.Initialize(config.GetServerRuntime().Config.Cache, "test-deployment"))
	return newSessionExecutor(flowFactory, store, authCtxStore, tx, authn)
}

func freshCtx() *core.NodeContext {
	return &core.NodeContext{
		Context:     context.Background(),
		ExecutionID: "exec-1",
		RuntimeData: map[string]string{common.RuntimeKeySelectedAuthClass: "urn:thunder:acr:password"},
		AuthUser:    authenticatedAuthUser(),
		ExecutionHistory: map[string]*common.NodeExecutionRecord{
			"basic_auth": {NodeID: "basic_auth", ExecutorName: "CredentialsAuthExecutor", Status: common.FlowStatusComplete},
		},
		SSO: core.SSOInputs{Binding: "bind-xyz", FlowID: "flow-1", FlowVersion: 3},
	}
}

func TestSession_FreshSave(t *testing.T) {
	store := &fakeStore{}
	authCtx := &fakeAuthContextStore{}
	exec := newTestSessionExecutor(t, store, authCtx, saveAuthnMock(t))

	resp, err := exec.Execute(freshCtx())
	require.NoError(t, err)

	// Lean session row: operational fields + aggregate assurance; no embedded context.
	require.NotNil(t, store.created)
	created := store.created
	assert.Equal(t, "user-1", created.SubjectID)
	assert.Equal(t, "flow-1", created.FlowID)
	assert.Equal(t, 3, created.FlowVersion)
	assert.Equal(t, "bind-xyz", created.Binding)
	assert.Equal(t, "urn:thunder:acr:password", created.AssuranceLevel)
	assert.Equal(t, session.StateActive, created.State)
	assert.NotEmpty(t, created.HandleID)
	assert.True(t, created.HandleExpiresAt.After(created.HandleIssuedAt))

	// Sibling auth context, keyed by the same session id, holds the sanitized facts.
	require.NotNil(t, authCtx.created)
	ac := authCtx.created
	assert.Equal(t, created.SessionID, ac.SessionID)
	assert.Equal(t, session.SubjectSnapshot{OUID: "ou-1", UserType: "person"}, ac.Subject)
	assert.Equal(t, "alice@example.com", ac.SnapshotClaims["email"])
	assert.Equal(t, "Alice", ac.SnapshotClaims["name"])
	assert.NotContains(t, ac.SnapshotClaims, "password")
	assert.Contains(t, ac.CompletedSteps, "basic_auth")
	assert.Equal(t, 1, ac.ContextVersion)

	// Handle is emitted for the transport layer and recorded for idempotency.
	assert.Equal(t, created.HandleID, resp.SSOHandleOut)
	assert.Equal(t, created.HandleID, resp.RuntimeData[common.RuntimeKeySSOSessionSaved])
	// The already-authenticated subject is echoed back so the engine keeps it.
	assert.True(t, resp.AuthUser.IsAuthenticated())
}

// TestSession_FreshSave_Atomic asserts the session and its auth context are written within a
// single transaction that commits.
func TestSession_FreshSave_Atomic(t *testing.T) {
	store := &fakeStore{}
	authCtx := &fakeAuthContextStore{}
	tx := &txStub{}
	exec := newTestSessionExecutorWithTx(t, store, authCtx, tx, saveAuthnMock(t))

	_, err := exec.Execute(freshCtx())
	require.NoError(t, err)

	assert.Equal(t, 1, tx.calls, "both writes must occur in a single transaction")
	assert.True(t, tx.committed)
	assert.NotNil(t, store.created)
	assert.NotNil(t, authCtx.created)
}

// TestSession_FreshSave_RollsBackOnSessionError forces the session write to fail and asserts
// the transaction rolls back both rows (neither persists) and the failure is non-fatal.
func TestSession_FreshSave_RollsBackOnSessionError(t *testing.T) {
	store := &fakeStore{createErr: errors.New("insert session failed")}
	authCtx := &fakeAuthContextStore{}
	tx := &txStub{}
	// Model the transaction's rollback by discarding the staged writes.
	tx.onRollback = func() {
		store.created = nil
		authCtx.created = nil
	}
	exec := newTestSessionExecutorWithTx(t, store, authCtx, tx, saveAuthnMock(t))

	resp, err := exec.Execute(freshCtx())

	require.NoError(t, err) // save failure degrades SSO but does not fail auth
	assert.False(t, tx.committed)
	assert.Nil(t, store.created, "session insert must be rolled back")
	assert.Nil(t, authCtx.created, "auth context insert must be rolled back")
	assert.Empty(t, resp.SSOHandleOut)
	assert.Empty(t, resp.RuntimeData[common.RuntimeKeySSOSessionSaved])
}

func TestSession_FreshSave_Idempotent(t *testing.T) {
	store := &fakeStore{}
	authCtx := &fakeAuthContextStore{}
	exec := newTestSessionExecutor(t, store, authCtx, saveAuthnMock(t))
	ctx := freshCtx()
	ctx.RuntimeData[common.RuntimeKeySSOSessionSaved] = "existing-handle"

	resp, err := exec.Execute(ctx)
	require.NoError(t, err)

	assert.Nil(t, store.created, "must not create a duplicate session")
	assert.Nil(t, authCtx.created, "must not create a duplicate auth context")
	assert.Equal(t, "existing-handle", resp.SSOHandleOut)
}

func TestSession_FreshSave_Unauthenticated(t *testing.T) {
	store := &fakeStore{}
	authCtx := &fakeAuthContextStore{}
	exec := newTestSessionExecutor(t, store, authCtx, saveAuthnMock(t))
	ctx := freshCtx()
	ctx.AuthUser = authnprovidermgr.AuthUser{}

	resp, err := exec.Execute(ctx)
	require.NoError(t, err)

	assert.Nil(t, store.created)
	assert.Nil(t, authCtx.created)
	assert.Empty(t, resp.SSOHandleOut)
}

// TestSession_FreshSave_AuthContextErrorIsNonFatal verifies that an auth-context write failure
// degrades SSO without failing auth, and (since the context is written first) leaves no session.
func TestSession_FreshSave_AuthContextErrorIsNonFatal(t *testing.T) {
	store := &fakeStore{}
	authCtx := &fakeAuthContextStore{createErr: errors.New("db down")}
	exec := newTestSessionExecutor(t, store, authCtx, saveAuthnMock(t))

	resp, err := exec.Execute(freshCtx())

	require.NoError(t, err)
	assert.Equal(t, common.ExecComplete, resp.Status)
	assert.Empty(t, resp.SSOHandleOut)
	assert.Nil(t, store.created, "session must not be created when its context failed")
	assert.Empty(t, resp.RuntimeData[common.RuntimeKeySSOSessionSaved])
}

func ssoLoadCtx() *core.NodeContext {
	return &core.NodeContext{
		Context:     context.Background(),
		ExecutionID: "exec-2",
		RuntimeData: map[string]string{
			common.RuntimeKeySSOSessionPresent: "true",
			common.RuntimeKeySSOResolvedHandle: "handle-abc",
		},
		SSO: core.SSOInputs{Binding: "bind-xyz", FlowID: "flow-1", FlowVersion: 3},
	}
}

func TestSession_SSOLoad(t *testing.T) {
	store := &fakeStore{getResult: &session.Session{
		SessionID: "sess-1", SubjectID: "user-2", HandleID: "handle-abc", FlowID: "flow-1", FlowVersion: 3,
		AssuranceLevel: "urn:thunder:acr:password", AuthenticatedAt: time.Unix(1700000000, 0).UTC(),
		State: session.StateActive, Version: 1,
	}}
	authCtx := &fakeAuthContextStore{getResult: &session.AuthContext{
		SessionID:      "sess-1",
		Subject:        session.SubjectSnapshot{OUID: "ou-9", UserType: "person"},
		SnapshotClaims: map[string]string{"email": "bob@example.com"},
		ContextVersion: 1,
	}}
	// The load path rehydrates the subject from the snapshot and never calls the provider.
	exec := newTestSessionExecutor(t, store, authCtx, managermock.NewAuthnProviderManagerInterfaceMock(t))

	resp, err := exec.Execute(ssoLoadCtx())
	require.NoError(t, err)

	// A fully-resolved AuthUser is rehydrated so downstream nodes read the subject through the
	// normal provider contract. Its embedded entity reference + attributes mirror the snapshot.
	assert.True(t, resp.AuthUser.IsAuthenticated())
	au := resp.AuthUser
	raw, marshalErr := json.Marshal(&au)
	require.NoError(t, marshalErr)
	rendered := string(raw)
	assert.True(t, strings.Contains(rendered, `"entityId":"user-2"`), rendered)
	assert.True(t, strings.Contains(rendered, `"ouId":"ou-9"`), rendered)
	assert.True(t, strings.Contains(rendered, "bob@example.com"), rendered)
	assert.Equal(t, "urn:thunder:acr:password", resp.RuntimeData[common.RuntimeKeySelectedAuthClass])
	assert.Equal(t, "1700000000", resp.RuntimeData[common.RuntimeKeyAuthTime])

	// The context is read lazily exactly once (on load), keyed by the session's id.
	assert.Equal(t, 1, authCtx.getCalls)
	assert.Equal(t, "sess-1", authCtx.gotSessionID)
	// The activity touch refreshes SESSION only — it never rewrites the auth context.
	require.NotNil(t, store.updated)
	assert.False(t, store.updated.LastActiveAt.IsZero())
	assert.Nil(t, authCtx.created, "activity touch must not rewrite the auth context")
}

func TestSession_SSOLoad_MissingSessionIsNonFatal(t *testing.T) {
	store := &fakeStore{getResult: nil}
	authCtx := &fakeAuthContextStore{}
	exec := newTestSessionExecutor(t, store, authCtx, managermock.NewAuthnProviderManagerInterfaceMock(t))

	resp, err := exec.Execute(ssoLoadCtx())

	require.NoError(t, err)
	assert.Equal(t, common.ExecComplete, resp.Status)
	assert.False(t, resp.AuthUser.IsAuthenticated())
}

// TestSession_SSOLoad_MissingContextIsNonFatal covers a session whose auth context is gone:
// the load fails gracefully rather than authenticating from a partial record.
func TestSession_SSOLoad_MissingContextIsNonFatal(t *testing.T) {
	store := &fakeStore{getResult: &session.Session{SessionID: "sess-1", SubjectID: "user-2", HandleID: "handle-abc"}}
	authCtx := &fakeAuthContextStore{getResult: nil}
	exec := newTestSessionExecutor(t, store, authCtx, managermock.NewAuthnProviderManagerInterfaceMock(t))

	resp, err := exec.Execute(ssoLoadCtx())

	require.NoError(t, err)
	assert.Equal(t, common.ExecComplete, resp.Status)
	assert.False(t, resp.AuthUser.IsAuthenticated())
}
