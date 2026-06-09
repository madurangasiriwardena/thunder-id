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
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/thunder-id/thunderid/internal/system/config"
	"github.com/thunder-id/thunderid/internal/system/log"
	sysutils "github.com/thunder-id/thunderid/internal/system/utils"
)

// SessionServiceInterface defines the public session management API.
type SessionServiceInterface interface {
	// CreateSessionFromFlow creates a SessionRecord from a completed authentication flow.
	// Returns (nil, nil) when the resolved session group is sessionless.
	CreateSessionFromFlow(ctx context.Context, in CreateSessionInput) (*SessionRecord, error)
	// ResolveSession reads the session handle cookie from r and returns the live session.
	// Returns (nil, nil) when no cookie is present, the handle is unknown, or the session
	// has expired.
	ResolveSession(ctx context.Context, r *http.Request) (*SessionRecord, error)
}

// CreateSessionInput carries the facts from a completed authentication flow.
type CreateSessionInput struct {
	SubjectID       string
	AppID           string
	AuthenticatedAt time.Time
}

// sessionService is the implementation of SessionServiceInterface.
type sessionService struct {
	store SessionRecordStoreInterface
}

func newSessionService(store SessionRecordStoreInterface) SessionServiceInterface {
	return &sessionService{store: store}
}

// CreateSessionFromFlow creates a durable SessionRecord for a managed session group.
func (s *sessionService) CreateSessionFromFlow(
	ctx context.Context, in CreateSessionInput,
) (*SessionRecord, error) {
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, "SessionService"))

	group := resolveSessionGroup(in.AppID)
	if group.Mode != SessionModeManaged {
		return nil, nil
	}

	sessionID, err := sysutils.GenerateUUIDv7()
	if err != nil {
		logger.ErrorWithContext(ctx, "Failed to generate session ID", log.Error(err))
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}
	handleID, err := sysutils.GenerateUUIDv7()
	if err != nil {
		logger.ErrorWithContext(ctx, "Failed to generate handle ID", log.Error(err))
		return nil, fmt.Errorf("failed to generate handle ID: %w", err)
	}

	cfg := config.GetServerRuntime().Config.Session
	now := time.Now().UTC()
	idleExpiresAt := now.Add(time.Duration(cfg.IdleTimeout) * time.Second)
	absoluteExpiresAt := now.Add(time.Duration(cfg.AbsoluteTimeout) * time.Second)

	rec := SessionRecord{
		SessionID:         sessionID,
		SubjectID:         in.SubjectID,
		SessionGroupID:    group.ID,
		AuthenticatedAt:   in.AuthenticatedAt,
		AssuranceLevel:    AssuranceLevelPlaceholder, // TODO Phase B: derive real ACR from flow
		CreatedAt:         now,
		LastActiveAt:      now,
		IdleExpiresAt:     idleExpiresAt,
		AbsoluteExpiresAt: absoluteExpiresAt,
		HandleID:          handleID,
		HandleIssuedAt:    now,
		HandleExpiresAt:   absoluteExpiresAt,
		Binding:           BindingContext{Type: BindingTypeCookieStrict},
		State:             SessionStateActive,
		Version:           0,
	}

	if err := s.store.CreateSession(ctx, rec); err != nil {
		logger.ErrorWithContext(ctx, "Failed to persist session record", log.Error(err))
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	logger.DebugWithContext(ctx, "Session created",
		log.String("sessionGroupID", group.ID),
		log.String("subjectID", in.SubjectID))
	return &rec, nil
}

// ResolveSession resolves the incoming session cookie to a live SessionRecord.
func (s *sessionService) ResolveSession(
	ctx context.Context, r *http.Request,
) (*SessionRecord, error) {
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, "SessionService"))

	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, nil
	}

	rec, err := s.store.GetSessionByHandle(ctx, cookie.Value)
	if err != nil {
		if errors.Is(err, errSessionNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get session by handle: %w", err)
	}

	now := time.Now().UTC()
	if !rec.IsLive(now) {
		return nil, nil
	}

	// TODO Phase B: add write-coalescing to avoid a DB write on every request.
	if _, touchErr := s.store.TouchSession(ctx, rec.SessionID, now, rec.Version); touchErr != nil {
		logger.ErrorWithContext(ctx, "Failed to touch session", log.Error(touchErr))
		return nil, fmt.Errorf("failed to update session activity: %w", touchErr)
	}

	return rec, nil
}

// resolveSessionGroup maps an appID to its session group.
// TODO Phase B: real per-group config; for now every app maps to the default managed group.
func resolveSessionGroup(_ string) SessionGroup {
	cfg := config.GetServerRuntime().Config.Session
	mode := SessionMode(cfg.DefaultMode)
	return SessionGroup{
		ID:   DefaultSessionGroupID,
		Mode: mode,
	}
}
