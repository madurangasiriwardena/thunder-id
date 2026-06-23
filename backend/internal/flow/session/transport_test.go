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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCookieName(t *testing.T) {
	a := CookieName("flow-1")
	b := CookieName("flow-2")

	assert.True(t, strings.HasPrefix(a, cookieNamePrefix))
	assert.Equal(t, a, CookieName("flow-1"), "name must be stable for a flow")
	assert.NotEqual(t, a, b, "different flows must get different cookie names")
}

func TestInboundHandle_HandleFor(t *testing.T) {
	ih := InboundHandle{Cookies: map[string]string{CookieName("flow-1"): "handle-1"}}

	assert.Equal(t, "handle-1", ih.HandleFor("flow-1"))
	assert.Equal(t, "", ih.HandleFor("flow-2"))
	assert.Equal(t, "", InboundHandle{}.HandleFor("flow-1"))
}

func TestInbound_ContextRoundTrip(t *testing.T) {
	ih := InboundHandle{Cookies: map[string]string{"a": "b"}, Binding: "bind"}

	got, ok := InboundFrom(WithInbound(context.Background(), ih))
	assert.True(t, ok)
	assert.Equal(t, ih, got)

	_, ok = InboundFrom(context.Background())
	assert.False(t, ok)
}

func TestCookieCarrier_Read(t *testing.T) {
	carrier := NewCookieCarrier(false)

	r := httptest.NewRequest(http.MethodPost, "/flow/execute", nil)
	r.Header.Set("User-Agent", "test-agent")
	r.AddCookie(&http.Cookie{Name: CookieName("flow-1"), Value: "handle-1"})
	r.AddCookie(&http.Cookie{Name: "unrelated", Value: "x"})

	ih := carrier.Read(r)

	assert.Equal(t, "handle-1", ih.HandleFor("flow-1"))
	assert.Equal(t, "x", ih.Cookies["unrelated"])
	assert.NotEmpty(t, ih.Binding, "binding derived from User-Agent")
}

func TestCookieCarrier_Read_NoUserAgent(t *testing.T) {
	carrier := NewCookieCarrier(false)
	r := httptest.NewRequest(http.MethodPost, "/flow/execute", nil)
	r.Header.Del("User-Agent")

	ih := carrier.Read(r)

	assert.Empty(t, ih.Binding)
}

func TestCookieCarrier_Write(t *testing.T) {
	carrier := NewCookieCarrier(true)
	w := httptest.NewRecorder()

	carrier.Write(w, CookieName("flow-1"), "handle-1", time.Hour)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	ck := cookies[0]
	assert.Equal(t, CookieName("flow-1"), ck.Name)
	assert.Equal(t, "handle-1", ck.Value)
	assert.True(t, ck.HttpOnly)
	assert.True(t, ck.Secure)
	assert.Equal(t, http.SameSiteLaxMode, ck.SameSite)
	assert.Equal(t, 3600, ck.MaxAge)
}

func TestCookieCarrier_Clear(t *testing.T) {
	carrier := NewCookieCarrier(false)
	w := httptest.NewRecorder()

	carrier.Clear(w, CookieName("flow-1"))

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "", cookies[0].Value)
	assert.True(t, cookies[0].MaxAge < 0)
}

// TestCookieCarrier_RoundTrip writes a handle then reads it back through a second request,
// proving the carrier's write and read agree on the cookie name.
func TestCookieCarrier_RoundTrip(t *testing.T) {
	carrier := NewCookieCarrier(false)
	w := httptest.NewRecorder()
	carrier.Write(w, CookieName("flow-1"), "handle-xyz", time.Hour)

	r := httptest.NewRequest(http.MethodPost, "/flow/execute", nil)
	for _, ck := range w.Result().Cookies() {
		r.AddCookie(ck)
	}

	ih := carrier.Read(r)
	assert.Equal(t, "handle-xyz", ih.HandleFor("flow-1"))
}
