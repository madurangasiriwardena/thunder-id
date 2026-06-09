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
	"strings"
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
	// EnsureClientSession returns the existing ClientSession for (sessionID, clientID) or
	// creates a new one. Uses create-or-reuse semantics.
	EnsureClientSession(ctx context.Context, sessionID, clientID string, grantedScopes []string) (*ClientSession, error)
	// GetSessionByID retrieves a SessionRecord by its internal PK. For /token use only;
	// SESSION_ID is never exposed to clients.
	GetSessionByID(ctx context.Context, sessionID string) (*SessionRecord, error)
	// GetClientSessionByID retrieves a ClientSession by its PK, for /token use.
	GetClientSessionByID(ctx context.Context, clientSessionID string) (*ClientSession, error)
}

// CreateSessionInput carries the facts from a completed authentication flow.
type CreateSessionInput struct {
	SubjectID       string
	AppID           string
	// OUID is the OU that owns the app. When non-empty it is used as the session group ID,
	// enabling OU-scoped SSO across apps in the same OU.
	OUID            string
	AuthenticatedAt time.Time
	// AssuranceLevel is the ACR value from the completed flow. When empty,
	// AssuranceLevelPlaceholder is used.
	AssuranceLevel string
}

// sessionService is the implementation of SessionServiceInterface.
type sessionService struct {
	store   SessionRecordStoreInterface
	csStore ClientSessionStoreInterface
}

func newSessionService(store SessionRecordStoreInterface, csStore ClientSessionStoreInterface) SessionServiceInterface {
	return &sessionService{store: store, csStore: csStore}
}

// CreateSessionFromFlow returns the active SessionRecord for the subject+group, creating one
// if none exists. Returns (nil, nil) when the resolved session group is sessionless.
func (s *sessionService) CreateSessionFromFlow(
	ctx context.Context, in CreateSessionInput,
) (*SessionRecord, error) {
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, "SessionService"))

	group := ResolveSessionGroup(in.OUID)
	if group.Mode != SessionModeManaged {
		return nil, nil
	}

	// Find-or-create: reuse an existing ACTIVE session for this (subject, group) pair.
	existing, err := s.store.GetActiveSessionBySubjectAndGroup(ctx, in.SubjectID, group.ID)
	if err != nil && !errors.Is(err, errSessionNotFound) {
		logger.ErrorWithContext(ctx, "Failed to look up existing session", log.Error(err))
		return nil, fmt.Errorf("failed to look up existing session: %w", err)
	}
	if existing != nil {
		return existing, nil
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

	assuranceLevel := in.AssuranceLevel
	if assuranceLevel == "" {
		assuranceLevel = AssuranceLevelPlaceholder
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
		AssuranceLevel:    assuranceLevel,
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

// EnsureClientSession returns the existing ClientSession for (sessionID, clientID) or creates one.
func (s *sessionService) EnsureClientSession(
	ctx context.Context, sessionID, clientID string, grantedScopes []string,
) (*ClientSession, error) {
	logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, "SessionService"))

	existing, err := s.csStore.GetClientSessionBySessionAndClient(ctx, sessionID, clientID)
	if err != nil && !errors.Is(err, errClientSessionNotFound) {
		return nil, fmt.Errorf("failed to look up client session: %w", err)
	}
	if existing != nil {
		now := time.Now().UTC()
		if touchErr := s.csStore.TouchClientSession(ctx, existing.ClientSessionID, now); touchErr != nil {
			logger.ErrorWithContext(ctx, "Failed to touch client session", log.Error(touchErr))
		}
		existing.LastUsedAt = now
		return existing, nil
	}

	clientSessionID, err := sysutils.GenerateUUIDv7()
	if err != nil {
		logger.ErrorWithContext(ctx, "Failed to generate client session ID", log.Error(err))
		return nil, fmt.Errorf("failed to generate client session ID: %w", err)
	}
	oidcSID, err := sysutils.GenerateUUIDv7()
	if err != nil {
		logger.ErrorWithContext(ctx, "Failed to generate OIDC SID", log.Error(err))
		return nil, fmt.Errorf("failed to generate OIDC SID: %w", err)
	}

	now := time.Now().UTC()
	cs := ClientSession{
		ClientSessionID: clientSessionID,
		SessionID:       sessionID,
		ClientID:        clientID,
		OIDCSID:         oidcSID,
		CreatedAt:       now,
		LastUsedAt:      now,
		Status:          ClientSessionStateActive,
		GrantedScopes:   strings.Join(grantedScopes, " "),
		Version:         0,
	}

	if err := s.csStore.CreateClientSession(ctx, cs); err != nil {
		logger.ErrorWithContext(ctx, "Failed to persist client session", log.Error(err))
		return nil, fmt.Errorf("failed to create client session: %w", err)
	}

	logger.DebugWithContext(ctx, "Client session created",
		log.String("sessionID", sessionID),
		log.String("clientID", clientID))
	return &cs, nil
}

// GetSessionByID retrieves a SessionRecord by its internal PK.
func (s *sessionService) GetSessionByID(ctx context.Context, sessionID string) (*SessionRecord, error) {
	rec, err := s.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, errSessionNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get session by ID: %w", err)
	}
	return rec, nil
}

// GetClientSessionByID retrieves a ClientSession by its PK.
func (s *sessionService) GetClientSessionByID(ctx context.Context, clientSessionID string) (*ClientSession, error) {
	cs, err := s.csStore.GetClientSessionByID(ctx, clientSessionID)
	if err != nil {
		if errors.Is(err, errClientSessionNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get client session by ID: %w", err)
	}
	return cs, nil
}

// ResolveSessionGroup maps an OU ID to its session group.
// When ouID is non-empty, the OU is the group boundary for SSO.
// Falls back to DefaultSessionGroupID when ouID is empty.
// TODO Phase C: replace with real per-group config when SessionGroup entity is introduced.
func ResolveSessionGroup(ouID string) SessionGroup {
	cfg := config.GetServerRuntime().Config.Session
	mode := SessionMode(cfg.DefaultMode)
	groupID := DefaultSessionGroupID
	if ouID != "" {
		groupID = ouID
	}
	return SessionGroup{
		ID:   groupID,
		Mode: mode,
	}
}
