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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/thunder-id/thunderid/tests/mocks/database/providermock"
)

const testDeploymentID = "test-deployment"

type SessionStoreTestSuite struct {
	suite.Suite
	dbProvider *providermock.DBProviderInterfaceMock
	dbClient   *providermock.DBClientInterfaceMock
	store      *sessionRecordStore
}

func TestSessionStoreTestSuite(t *testing.T) {
	suite.Run(t, new(SessionStoreTestSuite))
}

func (s *SessionStoreTestSuite) SetupTest() {
	s.dbProvider = providermock.NewDBProviderInterfaceMock(s.T())
	s.dbClient = providermock.NewDBClientInterfaceMock(s.T())
	s.store = &sessionRecordStore{dbProvider: s.dbProvider, deploymentID: testDeploymentID}
}

func (s *SessionStoreTestSuite) buildRecord() SessionRecord {
	now := time.Now().UTC().Truncate(time.Second)
	return SessionRecord{
		SessionID:         "session-1",
		SubjectID:         "user-1",
		SessionGroupID:    "",
		AuthenticatedAt:   now,
		AssuranceLevel:    AssuranceLevelPlaceholder,
		CreatedAt:         now,
		LastActiveAt:      now,
		IdleExpiresAt:     now.Add(30 * time.Minute),
		AbsoluteExpiresAt: now.Add(12 * time.Hour),
		HandleID:          "handle-1",
		HandleIssuedAt:    now,
		HandleExpiresAt:   now.Add(12 * time.Hour),
		Binding:           BindingContext{Type: BindingTypeCookieStrict},
		State:             SessionStateActive,
		Version:           0,
	}
}

func (s *SessionStoreTestSuite) TestCreateSession_Success() {
	rec := s.buildRecord()
	s.dbProvider.On("GetRuntimeDBClient").Return(s.dbClient, nil)
	s.dbClient.On("ExecuteContext", mock.Anything, queryCreateSession,
		testDeploymentID,
		rec.SessionID,
		rec.SubjectID,
		rec.SessionGroupID,
		mock.MatchedBy(func(t time.Time) bool { return t.Location() == time.UTC }),
		rec.AssuranceLevel,
		mock.MatchedBy(func(t time.Time) bool { return t.Location() == time.UTC }),
		mock.MatchedBy(func(t time.Time) bool { return t.Location() == time.UTC }),
		mock.MatchedBy(func(t time.Time) bool { return t.Location() == time.UTC }),
		mock.MatchedBy(func(t time.Time) bool { return t.Location() == time.UTC }),
		rec.HandleID,
		mock.MatchedBy(func(t time.Time) bool { return t.Location() == time.UTC }),
		mock.MatchedBy(func(t time.Time) bool { return t.Location() == time.UTC }),
		rec.Binding.Type,
		string(rec.State),
		mock.Anything, // AUTH_FACTORS (nil for records with no factors)
		rec.Version,
	).Return(int64(1), nil)

	err := s.store.CreateSession(context.Background(), rec)
	require.NoError(s.T(), err)
}

func (s *SessionStoreTestSuite) TestCreateSession_DBClientError() {
	s.dbProvider.On("GetRuntimeDBClient").Return(nil, errors.New("conn failed"))

	err := s.store.CreateSession(context.Background(), s.buildRecord())
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "failed to get database client")
}

func (s *SessionStoreTestSuite) TestCreateSession_ExecuteError() {
	s.dbProvider.On("GetRuntimeDBClient").Return(s.dbClient, nil)
	s.dbClient.On("ExecuteContext", mock.Anything, queryCreateSession,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything,
	).Return(int64(0), errors.New("insert failed"))

	err := s.store.CreateSession(context.Background(), s.buildRecord())
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "failed to insert session record")
}

func (s *SessionStoreTestSuite) TestGetSessionByHandle_Found() {
	now := time.Now().UTC().Truncate(time.Second)
	row := map[string]interface{}{
		"session_id":          "session-1",
		"subject_id":          "user-1",
		"session_group_id":    "",
		"authenticated_at":    now,
		"assurance_level":     AssuranceLevelPlaceholder,
		"created_at":          now,
		"last_active_at":      now,
		"idle_expires_at":     now.Add(30 * time.Minute),
		"absolute_expires_at": now.Add(12 * time.Hour),
		"handle_id":           "handle-1",
		"handle_issued_at":    now,
		"handle_expires_at":   now.Add(12 * time.Hour),
		"binding_type":        BindingTypeCookieStrict,
		"session_state":       string(SessionStateActive),
		"version":             int64(0),
	}

	s.dbProvider.On("GetRuntimeDBClient").Return(s.dbClient, nil)
	s.dbClient.On("QueryContext", mock.Anything, queryGetSessionByHandle,
		"handle-1", testDeploymentID,
	).Return([]map[string]interface{}{row}, nil)

	rec, err := s.store.GetSessionByHandle(context.Background(), "handle-1")
	require.NoError(s.T(), err)
	require.NotNil(s.T(), rec)
	assert.Equal(s.T(), "session-1", rec.SessionID)
	assert.Equal(s.T(), "handle-1", rec.HandleID)
	assert.Equal(s.T(), SessionStateActive, rec.State)
	assert.Equal(s.T(), BindingTypeCookieStrict, rec.Binding.Type)
	assert.Equal(s.T(), 0, rec.Version)
}

func (s *SessionStoreTestSuite) TestGetSessionByHandle_NotFound() {
	s.dbProvider.On("GetRuntimeDBClient").Return(s.dbClient, nil)
	s.dbClient.On("QueryContext", mock.Anything, queryGetSessionByHandle,
		"unknown-handle", testDeploymentID,
	).Return([]map[string]interface{}{}, nil)

	rec, err := s.store.GetSessionByHandle(context.Background(), "unknown-handle")
	require.Error(s.T(), err)
	assert.Nil(s.T(), rec)
	assert.ErrorIs(s.T(), err, errSessionNotFound)
}

func (s *SessionStoreTestSuite) TestGetSessionByHandle_DBClientError() {
	s.dbProvider.On("GetRuntimeDBClient").Return(nil, errors.New("conn failed"))

	rec, err := s.store.GetSessionByHandle(context.Background(), "handle-1")
	require.Error(s.T(), err)
	assert.Nil(s.T(), rec)
	assert.Contains(s.T(), err.Error(), "failed to get database client")
}

func (s *SessionStoreTestSuite) TestTouchSession_Updated() {
	now := time.Now().UTC()
	s.dbProvider.On("GetRuntimeDBClient").Return(s.dbClient, nil)
	s.dbClient.On("ExecuteContext", mock.Anything, queryTouchSession,
		"session-1",
		mock.MatchedBy(func(t time.Time) bool { return t.Location() == time.UTC }),
		0,
		testDeploymentID,
	).Return(int64(1), nil)

	updated, err := s.store.TouchSession(context.Background(), "session-1", now, 0)
	require.NoError(s.T(), err)
	assert.True(s.T(), updated, "rowsAffected > 0 must be reported as updated")
}

func (s *SessionStoreTestSuite) TestTouchSession_VersionMismatch() {
	s.dbProvider.On("GetRuntimeDBClient").Return(s.dbClient, nil)
	s.dbClient.On("ExecuteContext", mock.Anything, queryTouchSession,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(int64(0), nil)

	updated, err := s.store.TouchSession(context.Background(), "session-1", time.Now().UTC(), 5)
	require.NoError(s.T(), err)
	assert.False(s.T(), updated, "rowsAffected == 0 means version mismatch")
}

func (s *SessionStoreTestSuite) TestTouchSession_DBClientError() {
	s.dbProvider.On("GetRuntimeDBClient").Return(nil, errors.New("conn failed"))

	updated, err := s.store.TouchSession(context.Background(), "session-1", time.Now().UTC(), 0)
	require.Error(s.T(), err)
	assert.False(s.T(), updated)
	assert.Contains(s.T(), err.Error(), "failed to get database client")
}
