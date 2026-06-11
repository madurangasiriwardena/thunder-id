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
	"errors"
	"fmt"
	"time"

	"github.com/thunder-id/thunderid/internal/system/config"
	"github.com/thunder-id/thunderid/internal/system/error/serviceerror"
	"github.com/thunder-id/thunderid/internal/system/log"
	sysutils "github.com/thunder-id/thunderid/internal/system/utils"
)

// SessionGroupServiceInterface is the public API for session group management.
type SessionGroupServiceInterface interface {
	// CreateSessionGroup creates a new session group for the given OU.
	CreateSessionGroup(ctx context.Context, req CreateSessionGroupRequest) (*SessionGroup, *serviceerror.ServiceError)
	// GetSessionGroup returns the session group with the given ID.
	GetSessionGroup(ctx context.Context, id string) (*SessionGroup, *serviceerror.ServiceError)
	// ListSessionGroupsForOU returns all session groups for the given OU.
	ListSessionGroupsForOU(ctx context.Context, ouID string) (*SessionGroupListResponse, *serviceerror.ServiceError)
	// ListAllSessionGroups returns all session groups across the deployment.
	ListAllSessionGroups(ctx context.Context) (*SessionGroupListResponse, *serviceerror.ServiceError)
	// UpdateSessionGroup updates name and mode for the given session group.
	UpdateSessionGroup(ctx context.Context, id string, req UpdateSessionGroupRequest) (*SessionGroup, *serviceerror.ServiceError)
	// DeleteSessionGroup deletes a session group.
	DeleteSessionGroup(ctx context.Context, id string) *serviceerror.ServiceError
	// ResolveGroupForClient resolves the effective session group for a client.
	// When sessionGroupID is non-empty, it is looked up by ID and returned as-is (no OU check).
	// When sessionGroupID is empty or the ID is not found, a synthetic deployment-level default
	// group (DeploymentDefaultGroupID) is returned — no DB write is performed.
	ResolveGroupForClient(ctx context.Context, sessionGroupID, ouID string) (*SessionGroup, error)
}

type sessionGroupService struct {
	store sessionGroupStoreInterface
	log   *log.Logger
}

func newSessionGroupService(store sessionGroupStoreInterface) SessionGroupServiceInterface {
	return &sessionGroupService{
		store: store,
		log:   log.GetLogger().With(log.String(log.LoggerKeyComponentName, "SessionGroupService")),
	}
}

func (s *sessionGroupService) CreateSessionGroup(
	ctx context.Context, req CreateSessionGroupRequest,
) (*SessionGroup, *serviceerror.ServiceError) {
	if req.OUID == "" {
		return nil, &ErrorMissingOUID
	}
	if req.Name == "" {
		return nil, &ErrorInvalidRequestFormat
	}
	if !isValidMode(req.Mode) {
		return nil, &ErrorInvalidSessionMode
	}

	id, err := sysutils.GenerateUUIDv7()
	if err != nil {
		s.log.ErrorWithContext(ctx, "Failed to generate session group ID", log.Error(err))
		return nil, &serviceerror.InternalServerError
	}
	now := time.Now().UTC()
	g := SessionGroup{
		ID:        id,
		OUID:      req.OUID,
		Name:      req.Name,
		Mode:      req.Mode,
		IsDefault: false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.Create(ctx, g); err != nil {
		s.log.ErrorWithContext(ctx, "Failed to persist session group", log.Error(err))
		return nil, &serviceerror.InternalServerError
	}
	return &g, nil
}

func (s *sessionGroupService) GetSessionGroup(
	ctx context.Context, id string,
) (*SessionGroup, *serviceerror.ServiceError) {
	if id == "" {
		return nil, &ErrorMissingSessionGroupID
	}
	g, err := s.store.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrSessionGroupNotFound) {
			return nil, &ErrorSessionGroupNotFound
		}
		s.log.ErrorWithContext(ctx, "Failed to get session group", log.Error(err))
		return nil, &serviceerror.InternalServerError
	}
	return g, nil
}

func (s *sessionGroupService) ListSessionGroupsForOU(
	ctx context.Context, ouID string,
) (*SessionGroupListResponse, *serviceerror.ServiceError) {
	if ouID == "" {
		return nil, &ErrorMissingOUID
	}
	groups, err := s.store.ListByOU(ctx, ouID)
	if err != nil {
		s.log.ErrorWithContext(ctx, "Failed to list session groups", log.Error(err))
		return nil, &serviceerror.InternalServerError
	}
	return &SessionGroupListResponse{TotalResults: len(groups), Groups: groups}, nil
}

func (s *sessionGroupService) ListAllSessionGroups(
	ctx context.Context,
) (*SessionGroupListResponse, *serviceerror.ServiceError) {
	groups, err := s.store.ListAll(ctx)
	if err != nil {
		s.log.ErrorWithContext(ctx, "Failed to list all session groups", log.Error(err))
		return nil, &serviceerror.InternalServerError
	}
	return &SessionGroupListResponse{TotalResults: len(groups), Groups: groups}, nil
}

func (s *sessionGroupService) UpdateSessionGroup(
	ctx context.Context, id string, req UpdateSessionGroupRequest,
) (*SessionGroup, *serviceerror.ServiceError) {
	if id == "" {
		return nil, &ErrorMissingSessionGroupID
	}
	if req.Name == "" {
		return nil, &ErrorInvalidRequestFormat
	}
	if !isValidMode(req.Mode) {
		return nil, &ErrorInvalidSessionMode
	}
	g, err := s.store.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrSessionGroupNotFound) {
			return nil, &ErrorSessionGroupNotFound
		}
		s.log.ErrorWithContext(ctx, "Failed to get session group for update", log.Error(err))
		return nil, &serviceerror.InternalServerError
	}
	g.Name = req.Name
	g.Mode = req.Mode
	g.UpdatedAt = time.Now().UTC()
	if err := s.store.Update(ctx, *g); err != nil {
		s.log.ErrorWithContext(ctx, "Failed to update session group", log.Error(err))
		return nil, &serviceerror.InternalServerError
	}
	return g, nil
}

func (s *sessionGroupService) DeleteSessionGroup(ctx context.Context, id string) *serviceerror.ServiceError {
	if id == "" {
		return &ErrorMissingSessionGroupID
	}
	g, err := s.store.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrSessionGroupNotFound) {
			return &ErrorSessionGroupNotFound
		}
		s.log.ErrorWithContext(ctx, "Failed to get session group for delete", log.Error(err))
		return &serviceerror.InternalServerError
	}
	if g.IsDefault {
		return &ErrorCannotDeleteDefault
	}
	if err := s.store.Delete(ctx, id); err != nil {
		s.log.ErrorWithContext(ctx, "Failed to delete session group", log.Error(err))
		return &serviceerror.InternalServerError
	}
	return nil
}

func (s *sessionGroupService) ResolveGroupForClient(
	ctx context.Context, sessionGroupID, ouID string,
) (*SessionGroup, error) {
	if sessionGroupID != "" {
		g, err := s.store.GetByID(ctx, sessionGroupID)
		if err != nil {
			if errors.Is(err, ErrSessionGroupNotFound) {
				s.log.WarnWithContext(ctx, "Explicit session group not found; falling back to deployment default",
					log.String("sessionGroupID", sessionGroupID))
			} else {
				return nil, fmt.Errorf("failed to look up session group: %w", err)
			}
		} else {
			return g, nil
		}
	}
	return s.syntheticDefault(), nil
}

func (s *sessionGroupService) syntheticDefault() *SessionGroup {
	mode := SessionMode(config.GetServerRuntime().Config.Session.DefaultMode)
	if !isValidMode(mode) {
		mode = SessionModeManaged
	}
	return &SessionGroup{
		ID:        DeploymentDefaultGroupID,
		Name:      "Default",
		Mode:      mode,
		IsDefault: true,
	}
}

func isValidMode(m SessionMode) bool {
	return m == SessionModeManaged || m == SessionModeSessionless
}
