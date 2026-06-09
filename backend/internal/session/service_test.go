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
	assert.Equal(t, DefaultSessionGroupID, rec.SessionGroupID)
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
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "handle-1"})

	resolved, err := svc.ResolveSession(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, "sess-1", resolved.SessionID)
	assert.Equal(t, 1, store.touchCalls, "TouchSession must be called once for a live session")
}

func TestResolveSession_NoCookie(t *testing.T) {
	initTestConfig(t)
	svc := &sessionService{store: newStubStore()}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resolved, err := svc.ResolveSession(context.Background(), req)
	require.NoError(t, err)
	assert.Nil(t, resolved)
}

func TestResolveSession_UnknownHandle(t *testing.T) {
	initTestConfig(t)
	svc := &sessionService{store: newStubStore()}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "no-such-handle"})

	resolved, err := svc.ResolveSession(context.Background(), req)
	require.NoError(t, err)
	assert.Nil(t, resolved)
}

func TestResolveSession_ExpiredIdle(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

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
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "handle-exp"})

	resolved, err := svc.ResolveSession(context.Background(), req)
	require.NoError(t, err)
	assert.Nil(t, resolved)
	assert.Equal(t, 0, store.touchCalls, "TouchSession must not be called for an expired session")
}

func TestResolveSession_ExpiredAbsolute(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

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
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "handle-abs"})

	resolved, err := svc.ResolveSession(context.Background(), req)
	require.NoError(t, err)
	assert.Nil(t, resolved)
}

func TestResolveSession_NonActiveState(t *testing.T) {
	initTestConfig(t)
	store := newStubStore()
	svc := &sessionService{store: store}

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
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "handle-inactive"})

	resolved, err := svc.ResolveSession(context.Background(), req)
	require.NoError(t, err)
	assert.Nil(t, resolved)
}
