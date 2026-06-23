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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeClaims_AllowListAndStringify(t *testing.T) {
	claims := SanitizeClaims(map[string]interface{}{
		"email":          "alice@example.com",
		"email_verified": true,
		"name":           "Alice",
		"locale":         "en-US",
		// disallowed / non-scalar values are dropped:
		"password": "secret",
		"token":    "abc.def.ghi",
		"groups":   []string{"admin"},
		"profile":  map[string]interface{}{"x": 1},
	})

	assert.Equal(t, "alice@example.com", claims["email"])
	assert.Equal(t, "true", claims["email_verified"])
	assert.Equal(t, "Alice", claims["name"])
	assert.Equal(t, "en-US", claims["locale"])
	assert.NotContains(t, claims, "password")
	assert.NotContains(t, claims, "token")
	assert.NotContains(t, claims, "groups")
	assert.NotContains(t, claims, "profile")
}

func TestSanitizeClaims_Empty(t *testing.T) {
	assert.Nil(t, SanitizeClaims(nil))
	assert.Nil(t, SanitizeClaims(map[string]interface{}{"password": "secret"}))
}

func TestAuthContext_PayloadRoundTrip(t *testing.T) {
	c := AuthContext{
		SessionID:      "sess-1",
		Subject:        SubjectSnapshot{OUID: "ou-1", UserType: "person"},
		CompletedSteps: map[string]StepFact{"basic_auth": {Executor: "BasicAuthExecutor", Status: "COMPLETE"}},
		SnapshotClaims: map[string]string{"email": "a@b.com"},
		ContextVersion: 1,
	}

	raw, err := c.serializePayload()
	require.NoError(t, err)

	parsed, err := parseAuthContextPayload(raw)
	require.NoError(t, err)
	assert.Equal(t, c.Subject, parsed.Subject)
	assert.Equal(t, c.CompletedSteps, parsed.CompletedSteps)
	assert.Equal(t, c.SnapshotClaims, parsed.SnapshotClaims)
}

func TestParseAuthContextPayload_Invalid(t *testing.T) {
	_, err := parseAuthContextPayload("{not json")
	assert.Error(t, err)
}
