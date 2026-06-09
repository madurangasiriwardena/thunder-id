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
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thunder-id/thunderid/internal/system/config"
)

func initCookieTestConfig(t *testing.T) {
	t.Helper()
	_ = config.InitializeServerRuntime("/tmp/test-cookie", &config.Config{})
	t.Cleanup(config.ResetServerRuntime)
	config.GetServerRuntime().Config.Session = config.SessionConfig{
		DefaultMode:     string(SessionModeManaged),
		IdleTimeout:     1800,
		AbsoluteTimeout: 43200,
	}
}

func TestNewSessionCookie(t *testing.T) {
	initCookieTestConfig(t)

	cookie := NewSessionCookie("test-handle-id")
	require.NotNil(t, cookie)

	assert.Equal(t, SessionCookieName, cookie.Name, "cookie name must be __Host-tid_session")
	assert.Equal(t, "test-handle-id", cookie.Value, "cookie value must be the handle, not the session ID")
	assert.Equal(t, "/", cookie.Path)
	assert.Empty(t, cookie.Domain, "Domain must be empty for __Host- prefix semantics")
	assert.True(t, cookie.Secure)
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite, "SameSite must be Lax for top-level nav SSO")
	assert.Equal(t, 43200, cookie.MaxAge)
	assert.False(t, cookie.Expires.IsZero())
}

func TestNewSessionCookie_ValueIsHandle(t *testing.T) {
	initCookieTestConfig(t)

	const handleID = "handle-abc-123"
	const sessionID = "session-xyz-456"

	cookie := NewSessionCookie(handleID)
	assert.Equal(t, handleID, cookie.Value)
	assert.NotEqual(t, sessionID, cookie.Value, "session_id must never appear in the cookie value")
}
