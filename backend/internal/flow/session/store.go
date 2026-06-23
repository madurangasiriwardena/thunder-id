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
	"fmt"
	"strings"
	"time"

	"github.com/thunder-id/thunderid/internal/system/database/provider"
)

// StoreInterface defines the persistence operations for SSO sessions.
type StoreInterface interface {
	// Create persists a new session.
	Create(ctx context.Context, s Session) error
	// GetByHandle fetches a session by its opaque handle ID. It returns (nil, nil) when no
	// session matches; liveness checks are the resolver's responsibility.
	GetByHandle(ctx context.Context, handleID string) (*Session, error)
	// Update writes the mutable fields of an existing session under an optimistic-lock
	// guard. It returns ErrVersionConflict when the stored version no longer matches, and
	// bumps the in-memory Version on success.
	Update(ctx context.Context, s *Session) error
}

// store implements StoreInterface against the runtime relational store.
type store struct {
	dbProvider   provider.DBProviderInterface
	deploymentID string
}

// NewStore creates a session store backed by the given runtime DB provider.
func NewStore(dbProvider provider.DBProviderInterface, deploymentID string) StoreInterface {
	return &store{
		dbProvider:   dbProvider,
		deploymentID: deploymentID,
	}
}

// Create persists a new session.
func (st *store) Create(ctx context.Context, s Session) error {
	return withRuntimeDBClient(st.dbProvider, func(dbClient provider.DBClientInterface) error {
		_, err := dbClient.ExecuteContext(ctx, QueryCreateSession,
			s.SessionID, st.deploymentID, s.SubjectID, s.FlowID, s.FlowVersion,
			s.HandleID, s.HandleIssuedAt, s.HandleExpiresAt, s.Binding,
			s.AssuranceLevel, s.AuthenticatedAt, s.CreatedAt, s.LastActiveAt,
			nullableTime(s.IdleExpiresAt), nullableTime(s.AbsoluteExpiresAt), string(s.State), s.Version)
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
		return nil
	})
}

// GetByHandle fetches a session by its opaque handle ID.
func (st *store) GetByHandle(ctx context.Context, handleID string) (*Session, error) {
	var result *Session

	err := withRuntimeDBClient(st.dbProvider, func(dbClient provider.DBClientInterface) error {
		results, err := dbClient.QueryContext(ctx, QueryGetSessionByHandle, handleID, st.deploymentID)
		if err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
		if len(results) == 0 {
			return nil
		}
		if len(results) != 1 {
			return fmt.Errorf("unexpected number of results: %d", len(results))
		}

		s, buildErr := buildSessionFromRow(results[0])
		if buildErr != nil {
			return buildErr
		}
		result = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Update writes the mutable fields of an existing session under an optimistic-lock guard. It
// touches only SESSION — never the auth context — so an activity touch stays lean.
func (st *store) Update(ctx context.Context, s *Session) error {
	return withRuntimeDBClient(st.dbProvider, func(dbClient provider.DBClientInterface) error {
		rowsAffected, err := dbClient.ExecuteContext(ctx, QueryUpdateSession,
			s.FlowVersion, s.HandleID, s.HandleIssuedAt, s.HandleExpiresAt, s.Binding,
			s.AssuranceLevel, s.LastActiveAt,
			nullableTime(s.IdleExpiresAt), nullableTime(s.AbsoluteExpiresAt), string(s.State),
			s.SessionID, st.deploymentID, s.Version)
		if err != nil {
			return fmt.Errorf("failed to update session: %w", err)
		}
		if rowsAffected == 0 {
			return ErrVersionConflict
		}
		s.Version++
		return nil
	})
}

// withRuntimeDBClient runs fn with a runtime database client.
func withRuntimeDBClient(dbProvider provider.DBProviderInterface,
	fn func(provider.DBClientInterface) error) error {
	dbClient, err := dbProvider.GetRuntimeDBClient()
	if err != nil {
		return fmt.Errorf("failed to get database client: %w", err)
	}
	return fn(dbClient)
}

// nullableTime returns nil for a zero time so nullable columns store NULL, otherwise the time.
func nullableTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

// buildSessionFromRow maps a database result row into a Session.
func buildSessionFromRow(row map[string]interface{}) (*Session, error) {
	sessionID, err := parseString(row["session_id"], "session_id")
	if err != nil {
		return nil, err
	}
	subjectID, err := parseString(row["subject_id"], "subject_id")
	if err != nil {
		return nil, err
	}
	flowID, err := parseString(row["flow_id"], "flow_id")
	if err != nil {
		return nil, err
	}
	flowVersion, err := parseInt(row["flow_version"], "flow_version")
	if err != nil {
		return nil, err
	}
	handleID, err := parseString(row["handle_id"], "handle_id")
	if err != nil {
		return nil, err
	}
	handleIssuedAt, err := parseTime(row["handle_issued_at"], "handle_issued_at")
	if err != nil {
		return nil, err
	}
	handleExpiresAt, err := parseTime(row["handle_expires_at"], "handle_expires_at")
	if err != nil {
		return nil, err
	}
	authenticatedAt, err := parseTime(row["authenticated_at"], "authenticated_at")
	if err != nil {
		return nil, err
	}
	createdAt, err := parseTime(row["created_at"], "created_at")
	if err != nil {
		return nil, err
	}
	lastActiveAt, err := parseTime(row["last_active_at"], "last_active_at")
	if err != nil {
		return nil, err
	}
	version, err := parseInt(row["version"], "version")
	if err != nil {
		return nil, err
	}

	return &Session{
		SessionID:         sessionID,
		SubjectID:         subjectID,
		FlowID:            flowID,
		FlowVersion:       flowVersion,
		HandleID:          handleID,
		HandleIssuedAt:    handleIssuedAt,
		HandleExpiresAt:   handleExpiresAt,
		Binding:           parseNullableString(row["binding"]),
		AssuranceLevel:    parseNullableString(row["assurance_level"]),
		AuthenticatedAt:   authenticatedAt,
		CreatedAt:         createdAt,
		LastActiveAt:      lastActiveAt,
		IdleExpiresAt:     parseNullableTime(row["idle_expires_at"]),
		AbsoluteExpiresAt: parseNullableTime(row["absolute_expires_at"]),
		State:             State(parseNullableString(row["state"])),
		Version:           version,
	}, nil
}

// parseString parses a required string column.
func parseString(value interface{}, field string) (string, error) {
	if s := parseNullableStringPtr(value); s != nil {
		return *s, nil
	}
	return "", fmt.Errorf("failed to parse %s as string", field)
}

// parseNullableString parses an optional string column, returning "" when null.
func parseNullableString(value interface{}) string {
	if s := parseNullableStringPtr(value); s != nil {
		return *s
	}
	return ""
}

// parseNullableStringPtr parses a string column, handling the []byte form some drivers return.
func parseNullableStringPtr(value interface{}) *string {
	switch v := value.(type) {
	case string:
		return &v
	case []byte:
		s := string(v)
		return &s
	default:
		return nil
	}
}

// parseInt parses an integer column across the numeric forms drivers may return.
func parseInt(value interface{}, field string) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("failed to parse %s as int: got %T", field, value)
	}
}

// parseNullableTime parses an optional time column, returning the zero time when null.
func parseNullableTime(value interface{}) time.Time {
	if value == nil {
		return time.Time{}
	}
	t, err := parseTime(value, "")
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseTime parses a required time column across the string and time.Time forms drivers return.
func parseTime(value interface{}, field string) (time.Time, error) {
	const customTimeFormat = "2006-01-02 15:04:05.999999999"

	switch v := value.(type) {
	case time.Time:
		return v, nil
	case string:
		trimmed := trimTimeString(v)
		parsed, err := time.Parse(customTimeFormat, trimmed)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, v)
			if err != nil {
				return time.Time{}, fmt.Errorf("error parsing %s: %w", field, err)
			}
		}
		return parsed, nil
	case []byte:
		return parseTime(string(v), field)
	default:
		return time.Time{}, fmt.Errorf("unexpected type for %s: %T", field, value)
	}
}

// trimTimeString trims sub-second/zone trailers so SQLite datetime strings parse cleanly.
func trimTimeString(timeStr string) string {
	parts := strings.SplitN(timeStr, " ", 3)
	if len(parts) >= 2 {
		return parts[0] + " " + parts[1]
	}
	return timeStr
}
