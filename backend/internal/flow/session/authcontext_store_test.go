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
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/thunder-id/thunderid/tests/mocks/database/providermock"
)

// spyEncryptor is a test Encryptor that transforms predictably and records calls, so tests can
// assert the encryption-at-rest seam is exercised.
type spyEncryptor struct {
	encryptCalls int
	decryptCalls int
}

func (s *spyEncryptor) Encrypt(plaintext string) (string, error) {
	s.encryptCalls++
	return "enc:" + plaintext, nil
}

func (s *spyEncryptor) Decrypt(ciphertext string) (string, error) {
	s.decryptCalls++
	return strings.TrimPrefix(ciphertext, "enc:"), nil
}

type AuthContextStoreTestSuite struct {
	suite.Suite
	mockDBProvider *providermock.DBProviderInterfaceMock
	mockDBClient   *providermock.DBClientInterfaceMock
	enc            *spyEncryptor
	store          *authContextStore
}

func TestAuthContextStoreTestSuite(t *testing.T) {
	suite.Run(t, new(AuthContextStoreTestSuite))
}

func (s *AuthContextStoreTestSuite) SetupTest() {
	s.mockDBProvider = &providermock.DBProviderInterfaceMock{}
	s.mockDBClient = &providermock.DBClientInterfaceMock{}
	s.enc = &spyEncryptor{}
	s.store = &authContextStore{
		dbProvider:   s.mockDBProvider,
		deploymentID: testDeploymentID,
		encryptor:    s.enc,
	}
}

func sampleAuthContext() AuthContext {
	return AuthContext{
		SessionID:      "sess-1",
		Subject:        SubjectSnapshot{OUID: "ou-1", UserType: "person"},
		CompletedSteps: map[string]StepFact{"basic_auth": {Executor: "BasicAuthExecutor", Status: "COMPLETE"}},
		SnapshotClaims: map[string]string{"email": "alice@example.com"},
		ContextVersion: 1,
	}
}

func (s *AuthContextStoreTestSuite) TestNewAuthContextStore() {
	st := NewAuthContextStore(s.mockDBProvider, testDeploymentID, NewPassthroughEncryptor())
	s.NotNil(st)
	s.Implements((*AuthContextStoreInterface)(nil), st)
}

func (s *AuthContextStoreTestSuite) TestCreate_EncryptsAndPersists() {
	c := sampleAuthContext()
	payload, err := c.serializePayload()
	s.Require().NoError(err)

	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("ExecuteContext", context.Background(), QueryCreateAuthContext,
		c.SessionID, testDeploymentID, "enc:"+payload, c.ContextVersion).
		Return(int64(1), nil)

	createErr := s.store.Create(context.Background(), c)

	s.NoError(createErr)
	s.Equal(1, s.enc.encryptCalls, "encryption-at-rest seam must be exercised on create")
	s.mockDBClient.AssertExpectations(s.T())
}

func (s *AuthContextStoreTestSuite) TestCreate_TooLarge() {
	c := sampleAuthContext()
	c.SnapshotClaims = map[string]string{"email": strings.Repeat("a", MaxAuthContextBytes+1)}

	createErr := s.store.Create(context.Background(), c)

	s.ErrorIs(createErr, ErrAuthContextTooLarge)
	// Oversized payloads are rejected before any DB call.
	s.mockDBProvider.AssertNotCalled(s.T(), "GetRuntimeDBClient")
	s.Equal(0, s.enc.encryptCalls, "must not encrypt an over-cap payload")
}

func (s *AuthContextStoreTestSuite) TestCreate_DBError() {
	c := sampleAuthContext()
	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("ExecuteContext", context.Background(), QueryCreateAuthContext,
		c.SessionID, testDeploymentID, "enc:"+mustSerialize(s.T(), c), c.ContextVersion).
		Return(int64(0), errors.New("db down"))

	createErr := s.store.Create(context.Background(), c)

	s.Error(createErr)
	s.Contains(createErr.Error(), "failed to create auth context")
}

func (s *AuthContextStoreTestSuite) TestGetBySessionID_Hit() {
	c := sampleAuthContext()
	payload, err := c.serializePayload()
	s.Require().NoError(err)

	row := map[string]interface{}{
		"session_id":      "sess-1",
		"context":         "enc:" + payload,
		"context_version": int64(1),
	}
	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("QueryContext", context.Background(), QueryGetAuthContextBySessionID,
		"sess-1", testDeploymentID).
		Return([]map[string]interface{}{row}, nil)

	got, getErr := s.store.GetBySessionID(context.Background(), "sess-1")

	s.NoError(getErr)
	s.Require().NotNil(got)
	s.Equal(1, s.enc.decryptCalls, "decryption seam must be exercised on read")
	s.Equal("sess-1", got.SessionID)
	s.Equal(1, got.ContextVersion)
	s.Equal(SubjectSnapshot{OUID: "ou-1", UserType: "person"}, got.Subject)
	s.Equal("BasicAuthExecutor", got.CompletedSteps["basic_auth"].Executor)
	s.Equal("alice@example.com", got.SnapshotClaims["email"])
}

func (s *AuthContextStoreTestSuite) TestGetBySessionID_Miss() {
	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("QueryContext", context.Background(), QueryGetAuthContextBySessionID,
		"missing", testDeploymentID).
		Return([]map[string]interface{}{}, nil)

	got, getErr := s.store.GetBySessionID(context.Background(), "missing")

	s.NoError(getErr)
	s.Nil(got)
}

func (s *AuthContextStoreTestSuite) TestDelete() {
	s.mockDBProvider.On("GetRuntimeDBClient").Return(s.mockDBClient, nil)
	s.mockDBClient.On("ExecuteContext", context.Background(), QueryDeleteAuthContext,
		"sess-1", testDeploymentID).
		Return(int64(1), nil)

	delErr := s.store.Delete(context.Background(), "sess-1")

	s.NoError(delErr)
	s.mockDBClient.AssertExpectations(s.T())
}

func mustSerialize(t *testing.T, c AuthContext) string {
	t.Helper()
	payload, err := c.serializePayload()
	if err != nil {
		t.Fatalf("serialize payload: %v", err)
	}
	return payload
}
