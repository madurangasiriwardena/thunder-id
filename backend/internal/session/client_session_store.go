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
	"time"

	"github.com/thunder-id/thunderid/internal/system/config"
	"github.com/thunder-id/thunderid/internal/system/database/provider"
)

// ClientSessionStoreInterface defines client-session persistence operations.
type ClientSessionStoreInterface interface {
	CreateClientSession(ctx context.Context, cs ClientSession) error
	// GetClientSessionBySessionAndClient retrieves a ClientSession by SESSION_ID + CLIENT_ID.
	// Returns errClientSessionNotFound when no matching row exists.
	GetClientSessionBySessionAndClient(ctx context.Context, sessionID, clientID string) (*ClientSession, error)
	// GetClientSessionByID retrieves a ClientSession by its PK.
	// Returns errClientSessionNotFound when no matching row exists.
	GetClientSessionByID(ctx context.Context, clientSessionID string) (*ClientSession, error)
	// TouchClientSession updates LAST_USED_AT for a CLIENT_SESSION.
	TouchClientSession(ctx context.Context, clientSessionID string, lastUsedAt time.Time) error
}

// clientSessionStore is the runtime-DB-backed implementation.
type clientSessionStore struct {
	dbProvider   provider.DBProviderInterface
	deploymentID string
}

// newClientSessionStore returns a ClientSessionStoreInterface backed by the runtime database.
func newClientSessionStore(dbProvider provider.DBProviderInterface) ClientSessionStoreInterface {
	return &clientSessionStore{
		dbProvider:   dbProvider,
		deploymentID: config.GetServerRuntime().Config.Server.Identifier,
	}
}

// CreateClientSession inserts a new ClientSession row.
func (s *clientSessionStore) CreateClientSession(ctx context.Context, cs ClientSession) error {
	dbClient, err := s.dbProvider.GetRuntimeDBClient()
	if err != nil {
		return fmt.Errorf("failed to get database client: %w", err)
	}
	_, err = dbClient.ExecuteContext(ctx, queryCreateClientSession,
		s.deploymentID,
		cs.ClientSessionID,
		cs.SessionID,
		cs.ClientID,
		cs.OIDCSID,
		cs.CreatedAt.UTC(),
		cs.LastUsedAt.UTC(),
		cs.Status,
		cs.GrantedScopes,
		cs.Version,
	)
	if err != nil {
		return fmt.Errorf("failed to insert client session: %w", err)
	}
	return nil
}

// GetClientSessionBySessionAndClient retrieves a ClientSession by SESSION_ID and CLIENT_ID.
func (s *clientSessionStore) GetClientSessionBySessionAndClient(
	ctx context.Context, sessionID, clientID string,
) (*ClientSession, error) {
	dbClient, err := s.dbProvider.GetRuntimeDBClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get database client: %w", err)
	}
	results, err := dbClient.QueryContext(ctx, queryGetClientSessionBySessionAndClient,
		sessionID, clientID, s.deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query client session by session and client: %w", err)
	}
	if len(results) == 0 {
		return nil, errClientSessionNotFound
	}
	return buildClientSessionFromRow(results[0])
}

// GetClientSessionByID retrieves a ClientSession by its PK.
func (s *clientSessionStore) GetClientSessionByID(
	ctx context.Context, clientSessionID string,
) (*ClientSession, error) {
	dbClient, err := s.dbProvider.GetRuntimeDBClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get database client: %w", err)
	}
	results, err := dbClient.QueryContext(ctx, queryGetClientSessionByID,
		clientSessionID, s.deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query client session by ID: %w", err)
	}
	if len(results) == 0 {
		return nil, errClientSessionNotFound
	}
	return buildClientSessionFromRow(results[0])
}

// TouchClientSession updates LAST_USED_AT for the given CLIENT_SESSION.
func (s *clientSessionStore) TouchClientSession(
	ctx context.Context, clientSessionID string, lastUsedAt time.Time,
) error {
	dbClient, err := s.dbProvider.GetRuntimeDBClient()
	if err != nil {
		return fmt.Errorf("failed to get database client: %w", err)
	}
	_, err = dbClient.ExecuteContext(ctx, queryTouchClientSession,
		lastUsedAt.UTC(), clientSessionID, s.deploymentID)
	if err != nil {
		return fmt.Errorf("failed to touch client session: %w", err)
	}
	return nil
}

// buildClientSessionFromRow maps a database result row to a ClientSession.
func buildClientSessionFromRow(row map[string]interface{}) (*ClientSession, error) {
	clientSessionID, err := requireRowString(row, "client_session_id")
	if err != nil {
		return nil, err
	}
	sessionID, err := requireRowString(row, "session_id")
	if err != nil {
		return nil, err
	}
	clientID, err := requireRowString(row, "client_id")
	if err != nil {
		return nil, err
	}
	oidcSID, err := requireRowString(row, "oidc_sid")
	if err != nil {
		return nil, err
	}
	status, err := requireRowString(row, "status")
	if err != nil {
		return nil, err
	}
	grantedScopes, err := requireRowString(row, "granted_scopes")
	if err != nil {
		return nil, err
	}
	createdAt, err := requireRowTime(row, "created_at")
	if err != nil {
		return nil, err
	}
	lastUsedAt, err := requireRowTime(row, "last_used_at")
	if err != nil {
		return nil, err
	}
	version, err := requireRowInt(row, "version")
	if err != nil {
		return nil, err
	}
	return &ClientSession{
		ClientSessionID: clientSessionID,
		SessionID:       sessionID,
		ClientID:        clientID,
		OIDCSID:         oidcSID,
		CreatedAt:       createdAt,
		LastUsedAt:      lastUsedAt,
		Status:          status,
		GrantedScopes:   grantedScopes,
		Version:         version,
	}, nil
}

// requireRowString extracts a string from a result row (shared by session and client-session stores).
func requireRowString(row map[string]interface{}, key string) (string, error) {
	v := row[key]
	if str, ok := v.(string); ok {
		return str, nil
	}
	if b, ok := v.([]byte); ok {
		return string(b), nil
	}
	return "", fmt.Errorf("row: cannot parse %q as string (got %T)", key, v)
}

// requireRowTime extracts a time.Time from a result row.
func requireRowTime(row map[string]interface{}, key string) (time.Time, error) {
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
				return time.Time{}, fmt.Errorf("row: cannot parse %q as time: %w", key, err)
			}
		}
		return parsed, nil
	case nil:
		return time.Time{}, fmt.Errorf("row: %q is nil", key)
	default:
		return time.Time{}, fmt.Errorf("row: unexpected type for %q: %T", key, v)
	}
}

// requireRowInt extracts an int from a result row.
func requireRowInt(row map[string]interface{}, key string) (int, error) {
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
		return 0, fmt.Errorf("row: cannot parse %q as int (got %T)", key, v)
	}
}
