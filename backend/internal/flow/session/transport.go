// Copyright 2026 The ThunderID Authors
// SPDX-License-Identifier: Apache-2.0

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

// cookieName derives the per-flow SSO cookie name from the flow ID. Each flow gets its
// own cookie so sessions from different flows do not clobber each other's handle. The
// flow ID is hashed so the raw ID is not exposed in the cookie name and the name stays
// within the cookie-token character set.
func cookieName(flowID string) string {
	sum := sha256.Sum256([]byte(flowID))
	return cookieNamePrefix + hex.EncodeToString(sum[:])[:16]
}

// InboundHandle carries the request-scoped SSO handles read from a transport. The source decides
// how a handle is keyed and carried; callers only select a handle by flow ID. It is transient: it
// must never be persisted with the flow context.
type InboundHandle interface {
	// HandleFor returns the SSO handle carried for the given flow, or "" when none is present.
	HandleFor(flowID string) string
}

// staticInbound carries a single handle for a single flow.
type staticInbound struct {
	flowID string
	handle string
}

// NewStaticInbound creates an InboundHandle that carries handle for flowID only. It lets a caller
// that already holds the handle supply it without going through a request transport.
func NewStaticInbound(flowID, handle string) InboundHandle {
	return staticInbound{flowID: flowID, handle: handle}
}

// HandleFor returns the handle when flowID matches the carried flow, or "" otherwise.
func (s staticInbound) HandleFor(flowID string) string {
	if flowID != s.flowID {
		return ""
	}
	return s.handle
}

// anyFlowInbound carries a single handle for whichever flow the request resolves to. Offering it for
// any flow is safe: Resolve rejects a handle whose session belongs to a different flow.
type anyFlowInbound struct {
	handle string
}

// HandleFor returns the carried handle regardless of the flow ID.
func (a anyFlowInbound) HandleFor(string) string {
	return a.handle
}

// chainInbound offers the handles of several sources in priority order.
type chainInbound []InboundHandle

// HandleFor returns the first non-empty handle any source carries for the flow.
func (c chainInbound) HandleFor(flowID string) string {
	for _, ih := range c {
		if handle := ih.HandleFor(flowID); handle != "" {
			return handle
		}
	}
	return ""
}

type inboundCtxKey struct{}

// WithInbound stores the inbound SSO transport inputs on the context for the flow service
// to consume once it has resolved the flow ID.
func WithInbound(ctx context.Context, ih InboundHandle) context.Context {
	return context.WithValue(ctx, inboundCtxKey{}, ih)
}

// InboundFrom retrieves the inbound SSO transport inputs from the context. It reports false when
// nothing was attached or a nil InboundHandle was attached.
func InboundFrom(ctx context.Context) (InboundHandle, bool) {
	ih, ok := ctx.Value(inboundCtxKey{}).(InboundHandle)
	return ih, ok
}

// Exchange is one request and its response as the handle transports see them. Each field is one
// place a handle can be carried; a transport uses the fields it needs and ignores the rest. A new
// kind of carrier adds a field here, so existing transports and the HandleTransport methods do not
// change.
type Exchange struct {
	// Request is the inbound HTTP request.
	Request *http.Request
	// Response is the HTTP response being built. It is nil for an endpoint that only reads the handle.
	Response http.ResponseWriter
	// Body is the SSO handle field of the endpoint's JSON body. It is nil for an endpoint without one.
	Body *BodyHandle
}

// BodyHandle is the SSO handle field of an endpoint's JSON body.
type BodyHandle struct {
	// In is the handle the client sent in the request body.
	In string
	// Out is the handle to return in the response body. Transports set it; the endpoint copies it
	// into the response before writing the body.
	Out string
}

// HandleTransport abstracts how the session handle is read from a request and emitted onto a
// response. The transport owns how a flow's handle is keyed and carried; callers pass only the
// exchange and the flow ID.
type HandleTransport interface {
	// Read extracts the inbound SSO transport inputs from the exchange.
	Read(x *Exchange) InboundHandle
	// Write emits the handle for the given flow onto the exchange, valid for ttl.
	Write(x *Exchange, flowID, handle string, ttl time.Duration)
	// Clear removes the handle for the given flow from the exchange. Seam for logout / session end.
	Clear(x *Exchange, flowID string)
}

// TransportConfig holds the deployment settings the handle transports need.
type TransportConfig struct {
	// SecureCookies marks the SSO cookie Secure; it should be true behind TLS.
	SecureCookies bool
}

// NewHandleTransport creates the HandleTransport every endpoint uses. It is the single place the
// supported transports are assembled. A handle sent in the JSON body takes precedence over the
// cookie, and an issued handle is emitted on every transport.
func NewHandleTransport(cfg TransportConfig) HandleTransport {
	return chainTransport{bodyTransport{}, newCookieTransport(cfg.SecureCookies)}
}

// chainTransport combines transports. Read prefers earlier transports; Write and Clear reach all.
type chainTransport []HandleTransport

// Read collects the inbound inputs of every transport, in priority order.
func (c chainTransport) Read(x *Exchange) InboundHandle {
	inbound := make(chainInbound, 0, len(c))
	for _, t := range c {
		if ih := t.Read(x); ih != nil {
			inbound = append(inbound, ih)
		}
	}
	return inbound
}

// Write emits the handle on every transport.
func (c chainTransport) Write(x *Exchange, flowID, handle string, ttl time.Duration) {
	for _, t := range c {
		t.Write(x, flowID, handle, ttl)
	}
}

// Clear removes the handle from every transport.
func (c chainTransport) Clear(x *Exchange, flowID string) {
	for _, t := range c {
		t.Clear(x, flowID)
	}
}

// bodyTransport carries the handle in the endpoint's JSON body.
type bodyTransport struct{}

// Read returns the handle sent in the request body, or nil when none was sent.
func (bodyTransport) Read(x *Exchange) InboundHandle {
	if x.Body == nil || x.Body.In == "" {
		return nil
	}
	return anyFlowInbound{handle: x.Body.In}
}

// Write sets the handle to return in the response body.
func (bodyTransport) Write(x *Exchange, _, handle string, _ time.Duration) {
	if x.Body != nil {
		x.Body.Out = handle
	}
}

// Clear drops any handle set for the response body. The client discards its copy on its own.
func (bodyTransport) Clear(x *Exchange, _ string) {
	if x.Body != nil {
		x.Body.Out = ""
	}
}

// cookieTransport carries the handle as an HTTP cookie.
type cookieTransport struct {
	secure bool
}

// newCookieTransport creates a cookie-backed HandleTransport. secure controls the Secure
// attribute; it should be true behind TLS.
func newCookieTransport(secure bool) HandleTransport {
	return &cookieTransport{secure: secure}
}

// cookieInbound maps every inbound cookie name to its value. The per-flow handle is selected
// from this set by name, because the flow ID is not known when the transport reads the request.
type cookieInbound map[string]string

// HandleFor returns the value of the per-flow SSO cookie, or "" when it is absent.
func (ci cookieInbound) HandleFor(flowID string) string {
	return ci[cookieName(flowID)]
}

// Read collects all inbound cookies from the request.
func (c *cookieTransport) Read(x *Exchange) InboundHandle {
	cookies := make(cookieInbound)
	if x.Request == nil {
		return cookies
	}
	for _, ck := range x.Request.Cookies() {
		cookies[ck.Name] = ck.Value
	}
	return cookies
}

// Write sets the per-flow handle cookie on the response.
func (c *cookieTransport) Write(x *Exchange, flowID, handle string, ttl time.Duration) {
	if x.Response == nil {
		return
	}
	http.SetCookie(x.Response, &http.Cookie{
		Name:     cookieName(flowID),
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
func (c *cookieTransport) Clear(x *Exchange, flowID string) {
	if x.Response == nil {
		return
	}
	http.SetCookie(x.Response, &http.Cookie{
		Name:     cookieName(flowID),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
	})
}
