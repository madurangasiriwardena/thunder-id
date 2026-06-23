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
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"
)

// cookieNamePrefix prefixes every per-flow SSO cookie name.
const cookieNamePrefix = "tid_sso_"

// CookieName derives the per-flow SSO cookie name from the flow ID. Each flow gets its
// own cookie so sessions from different flows do not clobber each other's handle. The
// flow ID is hashed so the raw ID is not exposed in the cookie name and the name stays
// within the cookie-token character set.
func CookieName(flowID string) string {
	sum := sha256.Sum256([]byte(flowID))
	return cookieNamePrefix + hex.EncodeToString(sum[:])[:16]
}

// InboundHandle holds the request-scoped SSO transport inputs read from a carrier. It is
// transient: it must never be persisted with the flow context.
type InboundHandle struct {
	// Cookies maps every inbound cookie name to its value. The per-flow handle is selected
	// from this set by name, because the flow ID is not known when the carrier reads the request.
	Cookies map[string]string
	// Binding is the request-derived value a session is bound to (see HandleCarrier).
	Binding string
}

// HandleFor returns the SSO handle carried for the given flow, or "" when none is present.
func (ih InboundHandle) HandleFor(flowID string) string {
	if ih.Cookies == nil {
		return ""
	}
	return ih.Cookies[CookieName(flowID)]
}

type inboundCtxKey struct{}

// WithInbound stores the inbound SSO transport inputs on the context for the flow service
// to consume once it has resolved the flow ID.
func WithInbound(ctx context.Context, ih InboundHandle) context.Context {
	return context.WithValue(ctx, inboundCtxKey{}, ih)
}

// InboundFrom retrieves the inbound SSO transport inputs from the context.
func InboundFrom(ctx context.Context) (InboundHandle, bool) {
	ih, ok := ctx.Value(inboundCtxKey{}).(InboundHandle)
	return ih, ok
}

// HandleCarrier abstracts the transport that carries the session handle. A cookie is one
// carrier; keeping this behind an interface lets a non-cookie carrier plug in later.
type HandleCarrier interface {
	// Read extracts the inbound SSO transport inputs from a request.
	Read(r *http.Request) InboundHandle
	// Write emits the handle to the response under the given (per-flow) cookie name, valid
	// for ttl.
	Write(w http.ResponseWriter, cookieName, handle string, ttl time.Duration)
	// Clear removes the handle from the response. Seam for logout / session end.
	// TODO(sso): wire this to logout / back-channel handling (out of scope here).
	Clear(w http.ResponseWriter, cookieName string)
}

// cookieCarrier carries the handle as an HTTP cookie.
type cookieCarrier struct {
	secure bool
}

// NewCookieCarrier creates a cookie-backed HandleCarrier. secure controls the Secure
// attribute; it should be true behind TLS.
func NewCookieCarrier(secure bool) HandleCarrier {
	return &cookieCarrier{secure: secure}
}

// Read collects all inbound cookies and derives the binding from the request.
func (c *cookieCarrier) Read(r *http.Request) InboundHandle {
	cookies := make(map[string]string)
	for _, ck := range r.Cookies() {
		cookies[ck.Name] = ck.Value
	}
	return InboundHandle{
		Cookies: cookies,
		Binding: deriveBinding(r),
	}
}

// Write sets the per-flow handle cookie on the response.
func (c *cookieCarrier) Write(w http.ResponseWriter, cookieName, handle string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    handle,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   c.secure,
		// SameSite=Lax suffices for same-site SSO. Cross-site SSO would require
		// SameSite=None with Secure.
		// TODO(sso): make SameSite configurable for cross-site deployments.
		SameSite: http.SameSiteLaxMode,
	})
}

// Clear expires the per-flow handle cookie on the response.
func (c *cookieCarrier) Clear(w http.ResponseWriter, cookieName string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// deriveBinding computes the value a session handle is bound to for a given request.
//
// For this POC the handle is bound to the User-Agent: a stable signal within one browser
// that differs across clients, so a stolen handle replayed from a different client fails
// the binding check. It is intentionally lightweight.
// TODO(sso): strengthen binding (e.g. token-bound / device-bound) before production.
func deriveBinding(r *http.Request) string {
	ua := r.UserAgent()
	if ua == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(ua))
	return hex.EncodeToString(sum[:])
}
