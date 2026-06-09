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
	"time"

	"github.com/thunder-id/thunderid/internal/system/config"
)

// SessionCookieName is the __Host- prefixed cookie that carries the session handle.
// The __Host- prefix enforces Secure, Path=/, and no Domain — providing strict
// first-party scoping per RFC 6265bis.
const SessionCookieName = "__Host-tid_session"

// NewSessionCookie builds the Set-Cookie value for a new session handle.
// Lifetime is derived from the configured AbsoluteTimeout so the cookie and
// the server-side record share the same expiry boundary.
func NewSessionCookie(handleID string) *http.Cookie {
	absoluteTimeout := config.GetServerRuntime().Config.Session.AbsoluteTimeout
	expires := time.Now().UTC().Add(time.Duration(absoluteTimeout) * time.Second)
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    handleID,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   absoluteTimeout,
	}
}

// ClearSessionCookie returns a cookie that instructs the browser to delete the session cookie.
func ClearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}
