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
	// UpdateSessionGroup updates name and mode for the given session group.
	UpdateSessionGroup(ctx context.Context, id string, req UpdateSessionGroupRequest) (*SessionGroup, *serviceerror.ServiceError)
	// DeleteSessionGroup deletes a non-default session group.
	DeleteSessionGroup(ctx context.Context, id string) *serviceerror.ServiceError
	// EnsureDefaultForOU returns the existing default group for the OU, or creates one.
	// This is idempotent; concurrent calls are safe because the DB unique index prevents duplicates.
	EnsureDefaultForOU(ctx context.Context, ouID string) (*SessionGroup, error)
	// ResolveGroupForClient resolves the effective session group for a client.
	// When sessionGroupID is non-empty, it is validated (must belong to ouID) and returned.
	// When sessionGroupID is empty, the OU's default group is returned (creating it on demand).
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
	if req.IsDefault {
		exists, err := s.store.DefaultExistsForOU(ctx, req.OUID)
		if err != nil {
			s.log.ErrorWithContext(ctx, "Failed to check default group", log.Error(err))
			return nil, &serviceerror.InternalServerError
		}
		if exists {
			return nil, &ErrorDuplicateDefault
		}
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
		IsDefault: req.IsDefault,
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

func (s *sessionGroupService) EnsureDefaultForOU(ctx context.Context, ouID string) (*SessionGroup, error) {
	if ouID == "" {
		return nil, fmt.Errorf("ouID is required")
	}
	existing, err := s.store.GetDefaultForOU(ctx, ouID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrSessionGroupNotFound) {
		return nil, fmt.Errorf("failed to look up default session group: %w", err)
	}

	// Create a new default group.
	defaultMode := SessionMode(config.GetServerRuntime().Config.Session.DefaultMode)
	if !isValidMode(defaultMode) {
		defaultMode = SessionModeManaged
	}

	id, err := sysutils.GenerateUUIDv7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session group ID: %w", err)
	}
	now := time.Now().UTC()
	g := SessionGroup{
		ID:        id,
		OUID:      ouID,
		Name:      "Default",
		Mode:      defaultMode,
		IsDefault: true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if createErr := s.store.Create(ctx, g); createErr != nil {
		// Race: another process may have created the default. Try to read it.
		if existing2, readErr := s.store.GetDefaultForOU(ctx, ouID); readErr == nil {
			return existing2, nil
		}
		return nil, fmt.Errorf("failed to create default session group: %w", createErr)
	}
	s.log.DebugWithContext(ctx, "Created default session group",
		log.String("ouID", ouID), log.String("groupID", g.ID))
	return &g, nil
}

func (s *sessionGroupService) ResolveGroupForClient(
	ctx context.Context, sessionGroupID, ouID string,
) (*SessionGroup, error) {
	if sessionGroupID != "" {
		g, err := s.store.GetByID(ctx, sessionGroupID)
		if err != nil {
			if errors.Is(err, ErrSessionGroupNotFound) {
				// Explicit group not found → fall through to default.
				s.log.WarnWithContext(ctx, "Explicit session group not found; falling back to OU default",
					log.String("sessionGroupID", sessionGroupID))
			} else {
				return nil, fmt.Errorf("failed to look up session group: %w", err)
			}
		} else {
			if ouID != "" && g.OUID != ouID {
				// Cross-tenant attempt: group belongs to a different OU.
				s.log.WarnWithContext(ctx, "Session group OU mismatch; falling back to OU default",
					log.String("sessionGroupID", sessionGroupID),
					log.String("groupOUID", g.OUID),
					log.String("clientOUID", ouID))
			} else {
				return g, nil
			}
		}
	}
	if ouID == "" {
		return nil, fmt.Errorf("cannot resolve session group: no sessionGroupID and no ouID")
	}
	return s.EnsureDefaultForOU(ctx, ouID)
}

func isValidMode(m SessionMode) bool {
	return m == SessionModeManaged || m == SessionModeSessionless
}
