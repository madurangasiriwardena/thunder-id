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

package sessiongroup

import (
	"net/http"

	"github.com/thunder-id/thunderid/internal/system/middleware"
)

// Initialize initializes the session group service and registers its HTTP routes.
// The returned SessionGroupServiceInterface is injected into the session service,
// the OAuth authz service, and the OU service.
func Initialize(mux *http.ServeMux) (SessionGroupServiceInterface, error) {
	store, _, err := newSessionGroupStore()
	if err != nil {
		return nil, err
	}

	svc := newSessionGroupService(store)
	h := newSessionGroupHandler(svc)
	registerRoutes(mux, h)
	return svc, nil
}

func registerRoutes(mux *http.ServeMux, h *sessionGroupHandler) {
	collectionCORS := middleware.CORSOptions{
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   middleware.DefaultAllowedHeaders,
		AllowCredentials: true,
		MaxAge:           600,
	}
	itemCORS := middleware.CORSOptions{
		AllowedMethods:   []string{"GET", "PUT", "DELETE"},
		AllowedHeaders:   middleware.DefaultAllowedHeaders,
		AllowCredentials: true,
		MaxAge:           600,
	}

	// Collection: deployment-wide list + create (ouId supplied in request body for POST)
	mux.HandleFunc(middleware.WithCORS(
		"GET /session-groups",
		h.HandleListAllRequest, collectionCORS))
	mux.HandleFunc(middleware.WithCORS(
		"POST /session-groups",
		h.HandlePostRequest, collectionCORS))
	mux.HandleFunc(middleware.WithCORS(
		"OPTIONS /session-groups",
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) },
		collectionCORS))

	// By group ID: get, update, delete
	mux.HandleFunc(middleware.WithCORS(
		"GET /session-groups/{id}",
		h.HandleGetRequest, itemCORS))
	mux.HandleFunc(middleware.WithCORS(
		"PUT /session-groups/{id}",
		h.HandlePutRequest, itemCORS))
	mux.HandleFunc(middleware.WithCORS(
		"DELETE /session-groups/{id}",
		h.HandleDeleteRequest, itemCORS))
	mux.HandleFunc(middleware.WithCORS(
		"OPTIONS /session-groups/{id}",
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) },
		itemCORS))
}
