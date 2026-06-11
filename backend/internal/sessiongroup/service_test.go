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

package sessiongroup

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thunder-id/thunderid/internal/system/config"
	"github.com/thunder-id/thunderid/internal/system/log"
)

// initTestConfig sets up a minimal server config for tests that call syntheticDefault().
func initTestConfig(t *testing.T) {
	t.Helper()
	_ = config.InitializeServerRuntime("/tmp/test-sg-svc", &config.Config{})
	t.Cleanup(config.ResetServerRuntime)
	config.GetServerRuntime().Config.Session = config.SessionConfig{
		DefaultMode: string(SessionModeManaged),
	}
}

// stubGroupStore is an in-memory stub for sessionGroupStoreInterface.
type stubGroupStore struct {
	groups     map[string]*SessionGroup
	createErr  error
	listAllErr error
	listByOUErr error
}

func newStubGroupStore() *stubGroupStore {
	return &stubGroupStore{groups: make(map[string]*SessionGroup)}
}

func (s *stubGroupStore) Create(_ context.Context, g SessionGroup) error {
	if s.createErr != nil {
		return s.createErr
	}
	cp := g
	s.groups[g.ID] = &cp
	return nil
}

func (s *stubGroupStore) GetByID(_ context.Context, id string) (*SessionGroup, error) {
	g, ok := s.groups[id]
	if !ok {
		return nil, ErrSessionGroupNotFound
	}
	cp := *g
	return &cp, nil
}

func (s *stubGroupStore) ListByOU(_ context.Context, ouID string) ([]SessionGroup, error) {
	if s.listByOUErr != nil {
		return nil, s.listByOUErr
	}
	var out []SessionGroup
	for _, g := range s.groups {
		if g.OUID == ouID {
			out = append(out, *g)
		}
	}
	return out, nil
}

func (s *stubGroupStore) ListAll(_ context.Context) ([]SessionGroup, error) {
	if s.listAllErr != nil {
		return nil, s.listAllErr
	}
	out := make([]SessionGroup, 0, len(s.groups))
	for _, g := range s.groups {
		out = append(out, *g)
	}
	return out, nil
}

func (s *stubGroupStore) Update(_ context.Context, g SessionGroup) error {
	if _, ok := s.groups[g.ID]; !ok {
		return ErrSessionGroupNotFound
	}
	cp := g
	s.groups[g.ID] = &cp
	return nil
}

func (s *stubGroupStore) Delete(_ context.Context, id string) error {
	if _, ok := s.groups[id]; !ok {
		return ErrSessionGroupNotFound
	}
	delete(s.groups, id)
	return nil
}

func newSvc(store sessionGroupStoreInterface) *sessionGroupService {
	return &sessionGroupService{
		store: store,
		log:   log.GetLogger().With(log.String(log.LoggerKeyComponentName, "SessionGroupServiceTest")),
	}
}

// ---- ResolveGroupForClient ----

func TestResolveGroupForClient_ExplicitIDFound_ReturnedAsIs(t *testing.T) {
	initTestConfig(t)
	store := newStubGroupStore()
	now := time.Now().UTC()
	g := SessionGroup{
		ID: "group-a", OUID: "ou-1", Name: "GroupA",
		Mode: SessionModeManaged, CreatedAt: now, UpdatedAt: now,
	}
	store.groups["group-a"] = &g

	svc := newSvc(store)
	// ouID is from a different OU — no OU-match check in new logic, should still return the group.
	resolved, err := svc.ResolveGroupForClient(context.Background(), "group-a", "ou-9999")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, "group-a", resolved.ID)
	assert.Equal(t, "ou-1", resolved.OUID, "OUID from DB is preserved, no cross-OU rejection")
}

func TestResolveGroupForClient_ExplicitIDNotFound_ReturnsSyntheticDefault(t *testing.T) {
	initTestConfig(t)
	svc := newSvc(newStubGroupStore())

	resolved, err := svc.ResolveGroupForClient(context.Background(), "does-not-exist", "ou-1")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, DeploymentDefaultGroupID, resolved.ID)
	assert.True(t, resolved.IsDefault)
}

func TestResolveGroupForClient_EmptyGroupID_ReturnsSyntheticDefault(t *testing.T) {
	initTestConfig(t)
	svc := newSvc(newStubGroupStore())

	resolved, err := svc.ResolveGroupForClient(context.Background(), "", "ou-1")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, DeploymentDefaultGroupID, resolved.ID)
	assert.Equal(t, SessionModeManaged, resolved.Mode, "synthetic default uses config DefaultMode")
	assert.True(t, resolved.IsDefault)
}

func TestResolveGroupForClient_EmptyGroupID_EmptyOUID_ReturnsSyntheticDefault(t *testing.T) {
	initTestConfig(t)
	svc := newSvc(newStubGroupStore())

	// No sessionGroupID and no ouID — should still return synthetic default (no longer errors).
	resolved, err := svc.ResolveGroupForClient(context.Background(), "", "")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, DeploymentDefaultGroupID, resolved.ID)
}

func TestResolveGroupForClient_SessionlessModeDefault(t *testing.T) {
	initTestConfig(t)
	config.GetServerRuntime().Config.Session.DefaultMode = string(SessionModeSessionless)

	svc := newSvc(newStubGroupStore())
	resolved, err := svc.ResolveGroupForClient(context.Background(), "", "ou-1")
	require.NoError(t, err)
	assert.Equal(t, SessionModeSessionless, resolved.Mode)
}

// ---- CreateSessionGroup ----

func TestCreateSessionGroup_MissingOUID(t *testing.T) {
	initTestConfig(t)
	svc := newSvc(newStubGroupStore())
	_, svcErr := svc.CreateSessionGroup(context.Background(), CreateSessionGroupRequest{
		Name: "Group A", Mode: SessionModeManaged,
	})
	require.NotNil(t, svcErr)
	assert.Equal(t, ErrorMissingOUID.Code, svcErr.Code)
}

func TestCreateSessionGroup_MissingName(t *testing.T) {
	initTestConfig(t)
	svc := newSvc(newStubGroupStore())
	_, svcErr := svc.CreateSessionGroup(context.Background(), CreateSessionGroupRequest{
		OUID: "ou-1", Mode: SessionModeManaged,
	})
	require.NotNil(t, svcErr)
	assert.Equal(t, ErrorInvalidRequestFormat.Code, svcErr.Code)
}

func TestCreateSessionGroup_InvalidMode(t *testing.T) {
	initTestConfig(t)
	svc := newSvc(newStubGroupStore())
	_, svcErr := svc.CreateSessionGroup(context.Background(), CreateSessionGroupRequest{
		OUID: "ou-1", Name: "Group A", Mode: "invalid-mode",
	})
	require.NotNil(t, svcErr)
	assert.Equal(t, ErrorInvalidSessionMode.Code, svcErr.Code)
}

func TestCreateSessionGroup_Success_IsDefaultAlwaysFalse(t *testing.T) {
	initTestConfig(t)
	store := newStubGroupStore()
	svc := newSvc(store)

	// Even if caller passes IsDefault=true it should be silently set to false.
	g, svcErr := svc.CreateSessionGroup(context.Background(), CreateSessionGroupRequest{
		OUID: "ou-1", Name: "My Group", Mode: SessionModeManaged, IsDefault: true,
	})
	require.Nil(t, svcErr)
	require.NotNil(t, g)
	assert.False(t, g.IsDefault, "IsDefault must be false regardless of request")
	assert.Equal(t, "ou-1", g.OUID)
	assert.Equal(t, "My Group", g.Name)
	assert.NotEmpty(t, g.ID)

	// Group was persisted.
	assert.Len(t, store.groups, 1)
}

// ---- ListAllSessionGroups ----

func TestListAllSessionGroups_ReturnsAllGroups(t *testing.T) {
	initTestConfig(t)
	store := newStubGroupStore()
	now := time.Now().UTC()
	store.groups["g1"] = &SessionGroup{ID: "g1", OUID: "ou-1", Name: "G1", Mode: SessionModeManaged, CreatedAt: now, UpdatedAt: now}
	store.groups["g2"] = &SessionGroup{ID: "g2", OUID: "ou-2", Name: "G2", Mode: SessionModeSessionless, CreatedAt: now, UpdatedAt: now}

	svc := newSvc(store)
	resp, svcErr := svc.ListAllSessionGroups(context.Background())
	require.Nil(t, svcErr)
	require.NotNil(t, resp)
	assert.Equal(t, 2, resp.TotalResults)
	assert.Len(t, resp.Groups, 2)
}

func TestListAllSessionGroups_Empty(t *testing.T) {
	initTestConfig(t)
	svc := newSvc(newStubGroupStore())
	resp, svcErr := svc.ListAllSessionGroups(context.Background())
	require.Nil(t, svcErr)
	assert.Equal(t, 0, resp.TotalResults)
}
