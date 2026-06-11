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

package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thunder-id/thunderid/internal/system/config"
)

// initTestConfig wires in a minimal SessionConfig so service methods can read config.
func initTestConfig(t *testing.T) {
	t.Helper()
	_ = config.InitializeServerRuntime("/tmp/test-session-svc", &config.Config{})
	t.Cleanup(config.ResetServerRuntime)
	config.GetServerRuntime().Config.Session = config.SessionConfig{
		DefaultMode:     string(SessionModeManaged),
		IdleTimeout:     1800,
		AbsoluteTimeout: 43200,
	}
}

// stubStore is an in-memory stub for SessionRecordStoreInterface.
type stubStore struct {
	records     map[string]*SessionRecord // keyed by handleID
	createCalls int
	touchCalls  int
	touchReturn bool
}

func newStubStore() *stubStore {
	return &stubStore{
		records:     make(map[string]*SessionRecord),
		touchReturn: true,
	}
}

func (s *stubStore) CreateSession(_ context.Context, rec SessionRecord) error {
	s.createCalls++
	cp := rec
	s.records[rec.HandleID] = &cp
	return nil
}

func (s *stubStore) GetSessionByHandle(_ context.Context, handleID string) (*SessionRecord, error) {
	rec, ok := s.records[handleID]
	if !ok {
		return nil, errSessionNotFound
	}
	cp := *rec
	return &cp, nil
}

func (s *stubStore) GetSessionByID(_ context.Context, sessionID string) (*SessionRecord, error) {
	for _, rec := range s.records {
		if rec.SessionID == sessionID {
			cp := *rec
			return &cp, nil
		}
	}
	return nil, errSessionNotFound
}

func (s *stubStore) TouchSession(_ context.Context, _ string, _ time.Time, _ int) (bool, error) {
	s.touchCalls++
	return s.touchReturn, nil
}

func (s *stubStore) UpdateSessionAuth(
	_ context.Context, sessionID string, factors []AuthFactor, assuranceLevel string, authenticatedAt time.Time,
) error {
	for _, rec := range s.records {
		if rec.SessionID == sessionID {
			rec.AuthFactors = factors
			rec.AssuranceLevel = assuranceLevel
			rec.AuthenticatedAt = authenticatedAt
			return nil
		}
	}
	return errSessionNotFound
}

func TestCreateSessionFromFlow_ManagedGroup(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

	in := CreateSessionInput{
		SubjectID:       "user-abc",
		AppID:           "app-1",
		AuthenticatedAt: time.Now().UTC(),
	}

	rec, err := svc.CreateSessionFromFlow(context.Background(), in)
	require.NoError(t, err)
	require.NotNil(t, rec)

	assert.NotEmpty(t, rec.SessionID)
	assert.NotEmpty(t, rec.HandleID)
	assert.NotEqual(t, rec.SessionID, rec.HandleID, "SessionID and HandleID must be distinct UUIDs")
	assert.Equal(t, "user-abc", rec.SubjectID)
	assert.Equal(t, "", rec.SessionGroupID)
	assert.Equal(t, SessionStateActive, rec.State)
	assert.Equal(t, BindingTypeCookieStrict, rec.Binding.Type)
	assert.Equal(t, 0, rec.Version)

	cfg := config.GetServerRuntime().Config.Session
	expectedAbsolute := rec.CreatedAt.Add(time.Duration(cfg.AbsoluteTimeout) * time.Second)
	assert.WithinDuration(t, expectedAbsolute, rec.AbsoluteExpiresAt, time.Second,
		"AbsoluteExpiresAt must be CreatedAt + AbsoluteTimeout")
	assert.Equal(t, rec.AbsoluteExpiresAt, rec.HandleExpiresAt,
		"HandleExpiresAt must equal AbsoluteExpiresAt")

	assert.Equal(t, 1, store.createCalls)
}

func TestCreateSessionFromFlow_SessionlessGroup(t *testing.T) {
	initTestConfig(t)
	cfg := config.GetServerRuntime()
	cfg.Config.Session.DefaultMode = string(SessionModeSessionless)
	defer func() { cfg.Config.Session.DefaultMode = string(SessionModeManaged) }()

	store := newStubStore()
	svc := &sessionService{store: store}

	rec, err := svc.CreateSessionFromFlow(context.Background(), CreateSessionInput{
		SubjectID: "user-abc",
		AppID:     "app-1",
	})

	require.NoError(t, err)
	assert.Nil(t, rec, "sessionless group must not create a session record")
	assert.Equal(t, 0, store.createCalls, "store must not be called for sessionless group")
}

func TestResolveSession_ValidLiveCookie(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

	const testGroup = "test-group"
	now := time.Now().UTC()
	rec := &SessionRecord{
		SessionID:         "sess-1",
		HandleID:          "handle-1",
		State:             SessionStateActive,
		IdleExpiresAt:     now.Add(30 * time.Minute),
		AbsoluteExpiresAt: now.Add(12 * time.Hour),
		Version:           2,
	}
	store.records["handle-1"] = rec

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName(testGroup), Value: "handle-1"})

	resolved, err := svc.ResolveSession(context.Background(), req, testGroup)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, "sess-1", resolved.SessionID)
	assert.Equal(t, 1, store.touchCalls, "TouchSession must be called once for a live session")
}

func TestResolveSession_NoCookie(t *testing.T) {
	initTestConfig(t)
	svc := &sessionService{store: newStubStore()}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resolved, err := svc.ResolveSession(context.Background(), req, "test-group")
	require.NoError(t, err)
	assert.Nil(t, resolved)
}

func TestResolveSession_UnknownHandle(t *testing.T) {
	initTestConfig(t)
	svc := &sessionService{store: newStubStore()}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName("test-group"), Value: "no-such-handle"})

	resolved, err := svc.ResolveSession(context.Background(), req, "test-group")
	require.NoError(t, err)
	assert.Nil(t, resolved)
}

func TestResolveSession_ExpiredIdle(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

	const testGroup = "test-group"
	now := time.Now().UTC()
	rec := &SessionRecord{
		SessionID:         "sess-exp",
		HandleID:          "handle-exp",
		State:             SessionStateActive,
		IdleExpiresAt:     now.Add(-1 * time.Minute), // already expired
		AbsoluteExpiresAt: now.Add(12 * time.Hour),
	}
	store.records["handle-exp"] = rec

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName(testGroup), Value: "handle-exp"})

	resolved, err := svc.ResolveSession(context.Background(), req, testGroup)
	require.NoError(t, err)
	assert.Nil(t, resolved)
	assert.Equal(t, 0, store.touchCalls, "TouchSession must not be called for an expired session")
}

func TestResolveSession_ExpiredAbsolute(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

	const testGroup = "test-group"
	now := time.Now().UTC()
	rec := &SessionRecord{
		SessionID:         "sess-abs",
		HandleID:          "handle-abs",
		State:             SessionStateActive,
		IdleExpiresAt:     now.Add(30 * time.Minute),
		AbsoluteExpiresAt: now.Add(-1 * time.Second), // already expired
	}
	store.records["handle-abs"] = rec

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName(testGroup), Value: "handle-abs"})

	resolved, err := svc.ResolveSession(context.Background(), req, testGroup)
	require.NoError(t, err)
	assert.Nil(t, resolved)
}

func TestResolveSession_NonActiveState(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

	const testGroup = "test-group"
	now := time.Now().UTC()
	rec := &SessionRecord{
		SessionID:         "sess-inactive",
		HandleID:          "handle-inactive",
		State:             SessionState("REVOKED"),
		IdleExpiresAt:     now.Add(30 * time.Minute),
		AbsoluteExpiresAt: now.Add(12 * time.Hour),
	}
	store.records["handle-inactive"] = rec

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName(testGroup), Value: "handle-inactive"})

	resolved, err := svc.ResolveSession(context.Background(), req, testGroup)
	require.NoError(t, err)
	assert.Nil(t, resolved)
}

func TestCreateSessionFromFlow_FindOrCreate_ReturnsExistingByHandle(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

	now := time.Now().UTC()
	existing := &SessionRecord{
		SessionID:         "existing-session",
		HandleID:          "existing-handle",
		SubjectID:         "user-abc",
		SessionGroupID:    "",
		State:             SessionStateActive,
		IdleExpiresAt:     now.Add(30 * time.Minute),
		AbsoluteExpiresAt: now.Add(12 * time.Hour),
	}
	store.records["existing-handle"] = existing

	in := CreateSessionInput{
		SubjectID:      "user-abc",
		AppID:          "app-1",
		IncomingHandle: "existing-handle",
	}
	rec, err := svc.CreateSessionFromFlow(context.Background(), in)
	require.NoError(t, err)
	require.NotNil(t, rec)

	assert.Equal(t, "existing-session", rec.SessionID, "must reuse the session bound to the incoming handle")
	assert.Equal(t, 0, store.createCalls, "CreateSession must not be called when handle matches an active session")
}

func TestCreateSessionFromFlow_FindOrCreate_CreatesWhenNone(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

	in := CreateSessionInput{SubjectID: "user-new", AppID: "app-1"}
	rec, err := svc.CreateSessionFromFlow(context.Background(), in)
	require.NoError(t, err)
	require.NotNil(t, rec)

	assert.NotEmpty(t, rec.SessionID)
	assert.Equal(t, 1, store.createCalls)
}

// TestCreateSessionFromFlow_PerBrowser_NoHandleAlwaysCreates verifies that when no
// IncomingHandle is present a new session is always created, even if a session for the
// same (subject, group) exists — the (subject, group) index is no longer the reuse key.
func TestCreateSessionFromFlow_PerBrowser_NoHandleAlwaysCreates(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

	now := time.Now().UTC()
	existing := &SessionRecord{
		SessionID:         "old-session",
		HandleID:          "old-handle",
		SubjectID:         "user-abc",
		SessionGroupID:    "",
		State:             SessionStateActive,
		IdleExpiresAt:     now.Add(30 * time.Minute),
		AbsoluteExpiresAt: now.Add(12 * time.Hour),
	}
	store.records["old-handle"] = existing

	in := CreateSessionInput{
		SubjectID: "user-abc",
		AppID:     "app-1",
		// No IncomingHandle — simulates a second browser window (no cookie yet).
	}
	rec, err := svc.CreateSessionFromFlow(context.Background(), in)
	require.NoError(t, err)
	require.NotNil(t, rec)

	assert.NotEqual(t, "old-session", rec.SessionID, "second browser must get its own session record")
	assert.Equal(t, 1, store.createCalls)
}

// TestCreateSessionFromFlow_PerBrowser_WrongSubjectCreatesNew verifies that a stale
// cookie from a different user (handle found but SubjectID mismatch) does not reuse that session.
func TestCreateSessionFromFlow_PerBrowser_WrongSubjectCreatesNew(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

	now := time.Now().UTC()
	otherUser := &SessionRecord{
		SessionID:         "other-session",
		HandleID:          "stolen-handle",
		SubjectID:         "other-user",
		SessionGroupID:    "",
		State:             SessionStateActive,
		IdleExpiresAt:     now.Add(30 * time.Minute),
		AbsoluteExpiresAt: now.Add(12 * time.Hour),
	}
	store.records["stolen-handle"] = otherUser

	in := CreateSessionInput{
		SubjectID:      "user-abc",
		AppID:          "app-1",
		IncomingHandle: "stolen-handle",
	}
	rec, err := svc.CreateSessionFromFlow(context.Background(), in)
	require.NoError(t, err)
	require.NotNil(t, rec)

	assert.NotEqual(t, "other-session", rec.SessionID, "must not reuse a session belonging to a different user")
	assert.Equal(t, 1, store.createCalls)
}

