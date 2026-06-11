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

	"github.com/thunder-id/thunderid/internal/sessiongroup"
	dbprovider "github.com/thunder-id/thunderid/internal/system/database/provider"
	"github.com/thunder-id/thunderid/internal/system/log"
)

// Initialize creates the session store and service.
//
// This phase unconditionally uses the DB-backed store. When a Redis runtime is
// configured, session creation is disabled with a warning log so Redis deployments
// are not silently broken.
// TODO Phase B: add Redis-backed store + dual selection in init (mirror authz/init.go).
func Initialize(_ *http.ServeMux, sgSvc sessiongroup.SessionGroupServiceInterface) SessionServiceInterface {
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, "SessionInit"))

	if dbprovider.GetDBProvider() == nil {
		logger.Warn("No runtime DB provider available; session service disabled")
		return nil
	}

	store := newSessionRecordStore(dbprovider.GetDBProvider())
	csStore := newClientSessionStore(dbprovider.GetDBProvider())
	svc := newSessionService(store, csStore, sgSvc)

	logger.Debug("Session service initialized (DB-backed store)")
	return svc
}

// ResolveMiddleware stashes the resolved SessionRecord in the request context.
// Provided for later phases; not wired into any route in Phase A since no
// endpoint consumes it yet. groupID identifies which per-group cookie to read.
func ResolveMiddleware(svc SessionServiceInterface, groupID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if svc != nil {
				rec, err := svc.ResolveSession(r.Context(), r, groupID)
				if err == nil && rec != nil {
					r = r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, rec))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// sessionContextKey is the unexported context key used by ResolveMiddleware.
type sessionContextKey struct{}

// FromContext retrieves the SessionRecord stored by ResolveMiddleware, or nil.
func FromContext(ctx context.Context) *SessionRecord {
	rec, _ := ctx.Value(sessionContextKey{}).(*SessionRecord)
	return rec
}
