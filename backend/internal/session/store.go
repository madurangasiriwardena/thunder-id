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
	"fmt"
	"strings"
	"time"

	"github.com/thunder-id/thunderid/internal/system/config"
	"github.com/thunder-id/thunderid/internal/system/database/provider"
)

// SessionRecordStoreInterface defines session persistence operations.
type SessionRecordStoreInterface interface {
	CreateSession(ctx context.Context, rec SessionRecord) error
	GetSessionByHandle(ctx context.Context, handleID string) (*SessionRecord, error)
	// GetSessionByID retrieves a SessionRecord by its internal PK. For internal use only;
	// SESSION_ID is never exposed to clients.
	GetSessionByID(ctx context.Context, sessionID string) (*SessionRecord, error)
	// TouchSession updates LAST_ACTIVE_AT and increments VERSION atomically.
	// Returns true when the row was updated (VERSION matched), false on a version
	// mismatch (concurrent update won the race), error on backend failure.
	TouchSession(ctx context.Context, sessionID string, lastActiveAt time.Time, version int) (bool, error)
	// GetActiveSessionBySubjectAndGroup retrieves the single ACTIVE SessionRecord for a
	// (subjectID, groupID) pair. Returns errSessionNotFound when none exists.
	GetActiveSessionBySubjectAndGroup(ctx context.Context, subjectID, groupID string) (*SessionRecord, error)
}

// sessionRecordStore is the runtime-DB-backed implementation.
type sessionRecordStore struct {
	dbProvider   provider.DBProviderInterface
	deploymentID string
}

// newSessionRecordStore returns a SessionRecordStoreInterface backed by the runtime database.
func newSessionRecordStore(dbProvider provider.DBProviderInterface) SessionRecordStoreInterface {
	return &sessionRecordStore{
		dbProvider:   dbProvider,
		deploymentID: config.GetServerRuntime().Config.Server.Identifier,
	}
}

// CreateSession inserts a new SessionRecord into the database.
func (s *sessionRecordStore) CreateSession(ctx context.Context, rec SessionRecord) error {
	dbClient, err := s.dbProvider.GetRuntimeDBClient()
	if err != nil {
		return fmt.Errorf("failed to get database client: %w", err)
	}

	_, err = dbClient.ExecuteContext(ctx, queryCreateSession,
		s.deploymentID,
		rec.SessionID,
		rec.SubjectID,
		rec.SessionGroupID,
		rec.AuthenticatedAt.UTC(),
		rec.AssuranceLevel,
		rec.CreatedAt.UTC(),
		rec.LastActiveAt.UTC(),
		rec.IdleExpiresAt.UTC(),
		rec.AbsoluteExpiresAt.UTC(),
		rec.HandleID,
		rec.HandleIssuedAt.UTC(),
		rec.HandleExpiresAt.UTC(),
		rec.Binding.Type,
		string(rec.State),
		rec.Version,
	)
	if err != nil {
		return fmt.Errorf("failed to insert session record: %w", err)
	}
	return nil
}

// GetSessionByHandle retrieves a SessionRecord by its opaque handle. Returns
// errSessionNotFound when no matching row exists.
func (s *sessionRecordStore) GetSessionByHandle(ctx context.Context, handleID string) (*SessionRecord, error) {
	dbClient, err := s.dbProvider.GetRuntimeDBClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get database client: %w", err)
	}

	results, err := dbClient.QueryContext(ctx, queryGetSessionByHandle, handleID, s.deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query session by handle: %w", err)
	}

	if len(results) == 0 {
		return nil, errSessionNotFound
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("unexpected number of session rows: %d", len(results))
	}

	return s.buildFromRow(results[0])
}

// GetSessionByID retrieves a SessionRecord by its internal SESSION_ID.
func (s *sessionRecordStore) GetSessionByID(ctx context.Context, sessionID string) (*SessionRecord, error) {
	dbClient, err := s.dbProvider.GetRuntimeDBClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get database client: %w", err)
	}

	results, err := dbClient.QueryContext(ctx, queryGetSessionByID, sessionID, s.deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query session by ID: %w", err)
	}

	if len(results) == 0 {
		return nil, errSessionNotFound
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("unexpected number of session rows: %d", len(results))
	}

	return s.buildFromRow(results[0])
}

// TouchSession performs an optimistic update of LAST_ACTIVE_AT + VERSION.
func (s *sessionRecordStore) TouchSession(
	ctx context.Context, sessionID string, lastActiveAt time.Time, version int,
) (bool, error) {
	dbClient, err := s.dbProvider.GetRuntimeDBClient()
	if err != nil {
		return false, fmt.Errorf("failed to get database client: %w", err)
	}

	rowsAffected, err := dbClient.ExecuteContext(ctx, queryTouchSession,
		sessionID,
		lastActiveAt.UTC(),
		version,
		s.deploymentID,
	)
	if err != nil {
		return false, fmt.Errorf("failed to touch session: %w", err)
	}
	return rowsAffected > 0, nil
}

// GetActiveSessionBySubjectAndGroup retrieves the ACTIVE SessionRecord for a (subjectID, groupID) pair.
func (s *sessionRecordStore) GetActiveSessionBySubjectAndGroup(
	ctx context.Context, subjectID, groupID string,
) (*SessionRecord, error) {
	dbClient, err := s.dbProvider.GetRuntimeDBClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get database client: %w", err)
	}

	results, err := dbClient.QueryContext(ctx, queryGetActiveSessionBySubjectAndGroup,
		subjectID, groupID, s.deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query active session by subject and group: %w", err)
	}

	if len(results) == 0 {
		return nil, errSessionNotFound
	}

	return s.buildFromRow(results[0])
}

// buildFromRow maps a database result row to a SessionRecord.
func (s *sessionRecordStore) buildFromRow(row map[string]interface{}) (*SessionRecord, error) {
	sessionID, err := s.requireString(row, "session_id")
	if err != nil {
		return nil, err
	}
	subjectID, err := s.requireString(row, "subject_id")
	if err != nil {
		return nil, err
	}
	groupID, err := s.requireString(row, "session_group_id")
	if err != nil {
		return nil, err
	}
	assuranceLevel, err := s.requireString(row, "assurance_level")
	if err != nil {
		return nil, err
	}
	handleID, err := s.requireString(row, "handle_id")
	if err != nil {
		return nil, err
	}
	bindingType, err := s.requireString(row, "binding_type")
	if err != nil {
		return nil, err
	}
	stateStr, err := s.requireString(row, "session_state")
	if err != nil {
		return nil, err
	}

	authenticatedAt, err := s.requireTime(row, "authenticated_at")
	if err != nil {
		return nil, err
	}
	createdAt, err := s.requireTime(row, "created_at")
	if err != nil {
		return nil, err
	}
	lastActiveAt, err := s.requireTime(row, "last_active_at")
	if err != nil {
		return nil, err
	}
	idleExpiresAt, err := s.requireTime(row, "idle_expires_at")
	if err != nil {
		return nil, err
	}
	absoluteExpiresAt, err := s.requireTime(row, "absolute_expires_at")
	if err != nil {
		return nil, err
	}
	handleIssuedAt, err := s.requireTime(row, "handle_issued_at")
	if err != nil {
		return nil, err
	}
	handleExpiresAt, err := s.requireTime(row, "handle_expires_at")
	if err != nil {
		return nil, err
	}

	version, err := s.requireInt(row, "version")
	if err != nil {
		return nil, err
	}

	return &SessionRecord{
		SessionID:         sessionID,
		SubjectID:         subjectID,
		SessionGroupID:    groupID,
		AuthenticatedAt:   authenticatedAt,
		AssuranceLevel:    assuranceLevel,
		CreatedAt:         createdAt,
		LastActiveAt:      lastActiveAt,
		IdleExpiresAt:     idleExpiresAt,
		AbsoluteExpiresAt: absoluteExpiresAt,
		HandleID:          handleID,
		HandleIssuedAt:    handleIssuedAt,
		HandleExpiresAt:   handleExpiresAt,
		Binding:           BindingContext{Type: bindingType},
		State:             SessionState(stateStr),
		Version:           version,
	}, nil
}

func (s *sessionRecordStore) requireString(row map[string]interface{}, key string) (string, error) {
	v := row[key]
	if str, ok := v.(string); ok {
		return str, nil
	}
	if b, ok := v.([]byte); ok {
		return string(b), nil
	}
	return "", fmt.Errorf("session row: cannot parse %q as string (got %T)", key, v)
}

func (s *sessionRecordStore) requireTime(row map[string]interface{}, key string) (time.Time, error) {
	const customFmt = "2006-01-02 15:04:05.999999999"
	v := row[key]
	switch t := v.(type) {
	case time.Time:
		return t, nil
	case string:
		trimmed := trimTimeString(t)
		parsed, err := time.Parse(customFmt, trimmed)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, t)
			if err != nil {
				return time.Time{}, fmt.Errorf("session row: cannot parse %q as time: %w", key, err)
			}
		}
		return parsed, nil
	case nil:
		return time.Time{}, fmt.Errorf("session row: %q is nil", key)
	default:
		return time.Time{}, fmt.Errorf("session row: unexpected type for %q: %T", key, v)
	}
}

func (s *sessionRecordStore) requireInt(row map[string]interface{}, key string) (int, error) {
	v := row[key]
	switch n := v.(type) {
	case int:
		return n, nil
	case int32:
		return int(n), nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("session row: cannot parse %q as int (got %T)", key, v)
	}
}

func trimTimeString(s string) string {
	parts := strings.SplitN(s, " ", 3)
	if len(parts) >= 2 {
		return parts[0] + " " + parts[1]
	}
	return s
}
