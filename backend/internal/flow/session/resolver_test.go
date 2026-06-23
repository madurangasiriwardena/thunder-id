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

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/thunder-id/thunderid/tests/mocks/database/providermock"
)

type ResolverTestSuite struct {
	suite.Suite
	mockDBProvider *providermock.DBProviderInterfaceMock
	mockDBClient   *providermock.DBClientInterfaceMock
	resolver       Resolver
	now            time.Time
}

func TestResolverTestSuite(t *testing.T) {
	suite.Run(t, new(ResolverTestSuite))
}

func (s *ResolverTestSuite) SetupTest() {
	s.mockDBProvider = &providermock.DBProviderInterfaceMock{}
	s.mockDBClient = &providermock.DBClientInterfaceMock{}
	s.resolver = NewResolver(NewStore(s.mockDBProvider, testDeploymentID))
	s.now = time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
}

// testBinding is the binding stored on every fixture session.
const testBinding = "bind-xyz"

// row builds a session result row with the given state and handle expiry against live defaults.
func (s *ResolverTestSuite) row(state string, handleExpires time.Time) map[string]interface{} {
	return map[string]interface{}{
		"session_id":          "sess-1",
		"subject_id":          "user-1",
		"flow_id":             "flow-1",
		"flow_version":        int64(1),
		"handle_id":           "handle-abc",
		"handle_issued_at":    s.now.Add(-time.Minute),
		"handle_expires_at":   handleExpires,
		"binding":             testBinding,
		"assurance_level":     "urn:thunder:acr:password",
		"authenticated_at":    s.now.Add(-time.Minute),
		"created_at":          s.now.Add(-time.Minute),
		"last_active_at":      s.now.Add(-time.Minute),
		"idle_expires_at":     nil,
		"absolute_expires_at": nil,
		"state":               state,
		"version":             int64(1),
	}
}

func (s *ResolverTestSuite) expectQuery(rows []map[string]interface{}, err error) {
	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("QueryContext", context.Background(), QueryGetSessionByHandle,
		"handle-abc", testDeploymentID).Return(rows, err)
}

func (s *ResolverTestSuite) TestResolve_Hit() {
	s.expectQuery([]map[string]interface{}{s.row("ACTIVE", s.now.Add(time.Hour))}, nil)

	got, err := s.resolver.Resolve(context.Background(), "handle-abc", "bind-xyz", s.now)

	s.NoError(err)
	s.Require().NotNil(got)
	s.Equal("sess-1", got.SessionID)
	s.Equal("flow-1", got.FlowID)
	// The resolve hot path reads SESSION only — it must never load the auth context.
	s.mockDBClient.AssertNotCalled(s.T(), "QueryContext", mock.Anything, QueryGetAuthContextBySessionID, mock.Anything)
}

func (s *ResolverTestSuite) TestResolve_EmptyHandle() {
	got, err := s.resolver.Resolve(context.Background(), "", "bind-xyz", s.now)

	s.NoError(err)
	s.Nil(got)
	s.mockDBProvider.AssertNotCalled(s.T(), "GetRuntimeDBClient")
}

func (s *ResolverTestSuite) TestResolve_AbsentNoRow() {
	s.expectQuery([]map[string]interface{}{}, nil)

	got, err := s.resolver.Resolve(context.Background(), "handle-abc", "bind-xyz", s.now)

	s.NoError(err)
	s.Nil(got)
}

func (s *ResolverTestSuite) TestResolve_Ended() {
	s.expectQuery([]map[string]interface{}{s.row("ENDED", s.now.Add(time.Hour))}, nil)

	got, err := s.resolver.Resolve(context.Background(), "handle-abc", "bind-xyz", s.now)

	s.NoError(err)
	s.Nil(got)
}

func (s *ResolverTestSuite) TestResolve_Expired() {
	s.expectQuery([]map[string]interface{}{s.row("ACTIVE", s.now.Add(-time.Minute))}, nil)

	got, err := s.resolver.Resolve(context.Background(), "handle-abc", "bind-xyz", s.now)

	s.NoError(err)
	s.Nil(got)
}

func (s *ResolverTestSuite) TestResolve_BindingMismatch() {
	s.expectQuery([]map[string]interface{}{s.row("ACTIVE", s.now.Add(time.Hour))}, nil)

	got, err := s.resolver.Resolve(context.Background(), "handle-abc", "other-binding", s.now)

	s.NoError(err)
	s.Nil(got)
}

func (s *ResolverTestSuite) TestResolve_StoreError() {
	s.expectQuery(nil, errors.New("db down"))

	got, err := s.resolver.Resolve(context.Background(), "handle-abc", "bind-xyz", s.now)

	s.Error(err)
	s.Nil(got)
}
