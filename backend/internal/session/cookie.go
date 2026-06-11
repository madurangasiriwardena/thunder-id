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
	"time"

	"github.com/thunder-id/thunderid/internal/system/config"
)

type sessionHandleKey struct{}

// WithSessionHandle stores an incoming session handle in the context so downstream
// service code can reuse the caller's browser session.
func WithSessionHandle(ctx context.Context, handle string) context.Context {
	return context.WithValue(ctx, sessionHandleKey{}, handle)
}

// GetSessionHandleFromContext retrieves the session handle threaded through the context,
// returning an empty string when none is present.
func GetSessionHandleFromContext(ctx context.Context) string {
	if h, ok := ctx.Value(sessionHandleKey{}).(string); ok {
		return h
	}
	return ""
}

// SessionCookieName returns the __Host- prefixed cookie name for the given session group.
// Each group gets a distinct cookie so that apps in different groups never share handles.
// The __Host- prefix enforces Secure, Path=/, and no Domain — providing strict
// first-party scoping per RFC 6265bis.
func SessionCookieName(groupID string) string {
	return "__Host-tid_session_" + groupID
}

// NewSessionCookie builds the Set-Cookie value for a new session handle scoped to groupID.
// Lifetime is derived from the configured AbsoluteTimeout so the cookie and
// the server-side record share the same expiry boundary.
func NewSessionCookie(handleID, groupID string) *http.Cookie {
	absoluteTimeout := config.GetServerRuntime().Config.Session.AbsoluteTimeout
	expires := time.Now().UTC().Add(time.Duration(absoluteTimeout) * time.Second)
	return &http.Cookie{
		Name:     SessionCookieName(groupID),
		Value:    handleID,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   absoluteTimeout,
	}
}

// ClearSessionCookie returns a cookie that instructs the browser to delete the session cookie
// for the given group.
func ClearSessionCookie(groupID string) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName(groupID),
		Value:    "",
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}
