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
	"context"
	"net/http"

	"github.com/thunder-id/thunderid/internal/system/error/apierror"
	"github.com/thunder-id/thunderid/internal/system/error/serviceerror"
	"github.com/thunder-id/thunderid/internal/system/log"
	sysutils "github.com/thunder-id/thunderid/internal/system/utils"
)

const handlerLoggerComponentName = "SessionGroupHandler"

type sessionGroupHandler struct {
	service SessionGroupServiceInterface
}

func newSessionGroupHandler(service SessionGroupServiceInterface) *sessionGroupHandler {
	return &sessionGroupHandler{service: service}
}

// HandleListAllRequest handles GET /session-groups (deployment-wide listing)
func (h *sessionGroupHandler) HandleListAllRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, handlerLoggerComponentName))

	resp, svcErr := h.service.ListAllSessionGroups(ctx)
	if svcErr != nil {
		h.handleError(ctx, w, svcErr)
		return
	}

	sysutils.WriteSuccessResponse(ctx, w, http.StatusOK, resp)
	logger.Debug(ctx, "Listed all session groups", log.Int("totalResults", resp.TotalResults))
}

// HandleListRequest handles GET /organization-units/{ouId}/session-groups
func (h *sessionGroupHandler) HandleListRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, handlerLoggerComponentName))

	ouID := r.PathValue("ouId")
	if ouID == "" {
		h.handleError(ctx, w, &ErrorMissingOUID)
		return
	}

	resp, svcErr := h.service.ListSessionGroupsForOU(ctx, ouID)
	if svcErr != nil {
		h.handleError(ctx, w, svcErr)
		return
	}

	sysutils.WriteSuccessResponse(ctx, w, http.StatusOK, resp)
	logger.Debug(ctx, "Listed session groups", log.String("ouId", ouID),
		log.Int("totalResults", resp.TotalResults))
}

// HandlePostRequest handles POST /organization-units/{ouId}/session-groups
func (h *sessionGroupHandler) HandlePostRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, handlerLoggerComponentName))

	ouID := r.PathValue("ouId")

	req, err := sysutils.DecodeJSONBody[CreateSessionGroupRequest](r)
	if err != nil {
		h.handleError(ctx, w, &ErrorInvalidRequestFormat)
		return
	}
	if ouID != "" {
		req.OUID = ouID
	}

	g, svcErr := h.service.CreateSessionGroup(ctx, *req)
	if svcErr != nil {
		h.handleError(ctx, w, svcErr)
		return
	}

	sysutils.WriteSuccessResponse(ctx, w, http.StatusCreated, g)
	logger.Debug(ctx, "Created session group", log.String("id", g.ID))
}

// HandleGetRequest handles GET /session-groups/{id}
func (h *sessionGroupHandler) HandleGetRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, handlerLoggerComponentName))

	id := r.PathValue("id")
	if id == "" {
		h.handleError(ctx, w, &ErrorMissingSessionGroupID)
		return
	}

	g, svcErr := h.service.GetSessionGroup(ctx, id)
	if svcErr != nil {
		h.handleError(ctx, w, svcErr)
		return
	}

	sysutils.WriteSuccessResponse(ctx, w, http.StatusOK, g)
	logger.Debug(ctx, "Retrieved session group", log.String("id", id))
}

// HandlePutRequest handles PUT /session-groups/{id}
func (h *sessionGroupHandler) HandlePutRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, handlerLoggerComponentName))

	id := r.PathValue("id")
	if id == "" {
		h.handleError(ctx, w, &ErrorMissingSessionGroupID)
		return
	}

	req, err := sysutils.DecodeJSONBody[UpdateSessionGroupRequest](r)
	if err != nil {
		h.handleError(ctx, w, &ErrorInvalidRequestFormat)
		return
	}

	g, svcErr := h.service.UpdateSessionGroup(ctx, id, *req)
	if svcErr != nil {
		h.handleError(ctx, w, svcErr)
		return
	}

	sysutils.WriteSuccessResponse(ctx, w, http.StatusOK, g)
	logger.Debug(ctx, "Updated session group", log.String("id", id))
}

// HandleDeleteRequest handles DELETE /session-groups/{id}
func (h *sessionGroupHandler) HandleDeleteRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, handlerLoggerComponentName))

	id := r.PathValue("id")
	if id == "" {
		h.handleError(ctx, w, &ErrorMissingSessionGroupID)
		return
	}

	svcErr := h.service.DeleteSessionGroup(ctx, id)
	if svcErr != nil {
		h.handleError(ctx, w, svcErr)
		return
	}

	sysutils.WriteSuccessResponse(ctx, w, http.StatusNoContent, nil)
	logger.Debug(ctx, "Deleted session group", log.String("id", id))
}

func (h *sessionGroupHandler) handleError(
	ctx context.Context, w http.ResponseWriter, svcErr *serviceerror.ServiceError,
) {
	var statusCode int
	switch svcErr.Type {
	case serviceerror.ClientErrorType:
		switch svcErr.Code {
		case ErrorSessionGroupNotFound.Code:
			statusCode = http.StatusNotFound
		case ErrorDuplicateDefault.Code, ErrorCannotDeleteDefault.Code:
			statusCode = http.StatusConflict
		default:
			statusCode = http.StatusBadRequest
		}
	default:
		statusCode = http.StatusInternalServerError
	}

	sysutils.WriteErrorResponse(ctx, w, statusCode, apierror.ErrorResponse{
		Code:        svcErr.Code,
		Message:     svcErr.Error,
		Description: svcErr.ErrorDescription,
	})
}
