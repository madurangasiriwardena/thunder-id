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

package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/thunder-id/thunderid/tests/mocks/database/providermock"
)

const testDeploymentID = "test-deployment-id"

type StoreTestSuite struct {
	suite.Suite
	mockDBProvider *providermock.DBProviderInterfaceMock
	mockDBClient   *providermock.DBClientInterfaceMock
	store          *store
}

func TestStoreTestSuite(t *testing.T) {
	suite.Run(t, new(StoreTestSuite))
}

func (s *StoreTestSuite) SetupTest() {
	s.mockDBProvider = &providermock.DBProviderInterfaceMock{}
	s.mockDBClient = &providermock.DBClientInterfaceMock{}
	s.store = &store{
		dbProvider:   s.mockDBProvider,
		deploymentID: testDeploymentID,
	}
}

func (s *StoreTestSuite) sampleSession() Session {
	base := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	return Session{
		SessionID:       "sess-1",
		SubjectID:       "user-1",
		FlowID:          "flow-1",
		FlowVersion:     2,
		HandleID:        "handle-abc",
		HandleIssuedAt:  base,
		HandleExpiresAt: base.Add(time.Hour),
		Binding:         "bind-xyz",
		AssuranceLevel:  "urn:thunder:acr:password",
		AuthenticatedAt: base,
		CreatedAt:       base,
		LastActiveAt:    base,
		// IdleExpiresAt left zero on purpose to exercise the nullable path.
		AbsoluteExpiresAt: base.Add(8 * time.Hour),
		State:             StateActive,
		Version:           1,
	}
}

func (s *StoreTestSuite) TestNewStore() {
	st := NewStore(s.mockDBProvider, testDeploymentID)
	s.NotNil(st)
	s.Implements((*StoreInterface)(nil), st)
}

func (s *StoreTestSuite) TestCreate_Success() {
	sess := s.sampleSession()

	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("ExecuteContext", context.Background(), QueryCreateSession,
		sess.SessionID, testDeploymentID, sess.SubjectID, sess.FlowID, sess.FlowVersion,
		sess.HandleID, sess.HandleIssuedAt, sess.HandleExpiresAt, sess.Binding,
		sess.AssuranceLevel, sess.AuthenticatedAt, sess.CreatedAt, sess.LastActiveAt,
		nil, sess.AbsoluteExpiresAt, string(sess.State), sess.Version).
		Return(int64(1), nil)

	err := s.store.Create(context.Background(), sess)

	s.NoError(err)
	s.mockDBProvider.AssertExpectations(s.T())
	s.mockDBClient.AssertExpectations(s.T())
}

func (s *StoreTestSuite) TestCreate_DBError() {
	sess := s.sampleSession()

	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("ExecuteContext", context.Background(), QueryCreateSession,
		sess.SessionID, testDeploymentID, sess.SubjectID, sess.FlowID, sess.FlowVersion,
		sess.HandleID, sess.HandleIssuedAt, sess.HandleExpiresAt, sess.Binding,
		sess.AssuranceLevel, sess.AuthenticatedAt, sess.CreatedAt, sess.LastActiveAt,
		nil, sess.AbsoluteExpiresAt, string(sess.State), sess.Version).
		Return(int64(0), errors.New("db down"))

	err := s.store.Create(context.Background(), sess)

	s.Error(err)
	s.Contains(err.Error(), "failed to create session")
}

func (s *StoreTestSuite) TestGetByHandle_Hit() {
	base := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	row := map[string]interface{}{
		"session_id":          "sess-1",
		"subject_id":          "user-1",
		"flow_id":             "flow-1",
		"flow_version":        int64(2),
		"handle_id":           "handle-abc",
		"handle_issued_at":    base,
		"handle_expires_at":   base.Add(time.Hour),
		"binding":             "bind-xyz",
		"assurance_level":     "urn:thunder:acr:password",
		"authenticated_at":    base,
		"created_at":          base,
		"last_active_at":      base,
		"idle_expires_at":     nil,
		"absolute_expires_at": base.Add(8 * time.Hour),
		"state":               "ACTIVE",
		"version":             int64(3),
	}

	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("QueryContext", context.Background(), QueryGetSessionByHandle,
		"handle-abc", testDeploymentID).
		Return([]map[string]interface{}{row}, nil)

	got, err := s.store.GetByHandle(context.Background(), "handle-abc")

	s.NoError(err)
	s.Require().NotNil(got)
	s.Equal("sess-1", got.SessionID)
	s.Equal("user-1", got.SubjectID)
	s.Equal("flow-1", got.FlowID)
	s.Equal(2, got.FlowVersion)
	s.Equal("handle-abc", got.HandleID)
	s.Equal("bind-xyz", got.Binding)
	s.Equal("urn:thunder:acr:password", got.AssuranceLevel)
	s.Equal(StateActive, got.State)
	s.Equal(3, got.Version)
	s.True(got.HandleExpiresAt.Equal(base.Add(time.Hour)))
	s.True(got.AbsoluteExpiresAt.Equal(base.Add(8 * time.Hour)))
	s.True(got.IdleExpiresAt.IsZero())
}

func (s *StoreTestSuite) TestGetByHandle_Miss() {
	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("QueryContext", context.Background(), QueryGetSessionByHandle,
		"missing", testDeploymentID).
		Return([]map[string]interface{}{}, nil)

	got, err := s.store.GetByHandle(context.Background(), "missing")

	s.NoError(err)
	s.Nil(got)
}

func (s *StoreTestSuite) TestUpdate_Success() {
	sess := s.sampleSession()

	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("ExecuteContext", context.Background(), QueryUpdateSession,
		sess.FlowVersion, sess.HandleID, sess.HandleIssuedAt, sess.HandleExpiresAt, sess.Binding,
		sess.AssuranceLevel, sess.LastActiveAt, nil, sess.AbsoluteExpiresAt,
		string(sess.State), sess.SessionID, testDeploymentID, sess.Version).
		Return(int64(1), nil)

	err := s.store.Update(context.Background(), &sess)

	s.NoError(err)
	s.Equal(2, sess.Version) // optimistic version bumped in memory
	s.mockDBClient.AssertExpectations(s.T())
}

func (s *StoreTestSuite) TestUpdate_VersionConflict() {
	sess := s.sampleSession()

	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("ExecuteContext", context.Background(), QueryUpdateSession,
		sess.FlowVersion, sess.HandleID, sess.HandleIssuedAt, sess.HandleExpiresAt, sess.Binding,
		sess.AssuranceLevel, sess.LastActiveAt, nil, sess.AbsoluteExpiresAt,
		string(sess.State), sess.SessionID, testDeploymentID, sess.Version).
		Return(int64(0), nil)

	err := s.store.Update(context.Background(), &sess)

	s.ErrorIs(err, ErrVersionConflict)
	s.Equal(1, sess.Version) // version unchanged on conflict
}

func (s *StoreTestSuite) TestGetByHandle_ClientError() {
	s.mockDBProvider.On("GetRuntimeDBClient").Return(nil, errors.New("no client"))

	got, err := s.store.GetByHandle(context.Background(), "handle-abc")

	s.Error(err)
	s.Nil(got)
}

func (s *StoreTestSuite) TestGetByHandle_QueryError() {
	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("QueryContext", context.Background(), QueryGetSessionByHandle,
		"handle-abc", testDeploymentID).
		Return(nil, errors.New("query failed"))

	got, err := s.store.GetByHandle(context.Background(), "handle-abc")

	s.Error(err)
	s.Nil(got)
}

// TestBuildSessionFromRow_DriverVariants exercises the []byte / string-time / integer
// forms different drivers return for the same logical columns.
func (s *StoreTestSuite) TestBuildSessionFromRow_DriverVariants() {
	row := map[string]interface{}{
		"session_id":          []byte("sess-1"),
		"subject_id":          "user-1",
		"flow_id":             "flow-1",
		"flow_version":        int32(2),
		"handle_id":           "handle-abc",
		"handle_issued_at":    "2026-06-16 10:00:00",
		"handle_expires_at":   "2026-06-16T11:00:00Z",
		"binding":             []byte("bind-xyz"),
		"assurance_level":     "urn:thunder:acr:password",
		"authenticated_at":    "2026-06-16 10:00:00",
		"created_at":          "2026-06-16 10:00:00",
		"last_active_at":      "2026-06-16 10:00:00",
		"idle_expires_at":     "2026-06-16 10:30:00",
		"absolute_expires_at": "2026-06-16 18:00:00",
		"state":               "ACTIVE",
		"version":             3,
	}

	got, err := buildSessionFromRow(row)

	s.NoError(err)
	s.Require().NotNil(got)
	s.Equal("sess-1", got.SessionID)
	s.Equal(2, got.FlowVersion)
	s.Equal("bind-xyz", got.Binding)
	s.Equal(3, got.Version)
	s.False(got.IdleExpiresAt.IsZero())
}

func (s *StoreTestSuite) TestBuildSessionFromRow_BadField() {
	row := map[string]interface{}{"session_id": 42}

	got, err := buildSessionFromRow(row)

	s.Error(err)
	s.Nil(got)
}

func (s *StoreTestSuite) TestBuildSessionFromRow_BadIntField() {
	row := map[string]interface{}{
		"session_id":   "sess-1",
		"subject_id":   "user-1",
		"flow_id":      "flow-1",
		"flow_version": "not-an-int",
	}

	got, err := buildSessionFromRow(row)

	s.Error(err)
	s.Nil(got)
}
