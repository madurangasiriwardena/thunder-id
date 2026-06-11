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
	"fmt"
	"strings"
	"time"

	"github.com/thunder-id/thunderid/internal/system/config"
	"github.com/thunder-id/thunderid/internal/system/database/provider"
	"github.com/thunder-id/thunderid/internal/system/transaction"
)

// sessionGroupStoreInterface defines the data-access contract for SESSION_GROUP.
type sessionGroupStoreInterface interface {
	Create(ctx context.Context, g SessionGroup) error
	GetByID(ctx context.Context, id string) (*SessionGroup, error)
	GetDefaultForOU(ctx context.Context, ouID string) (*SessionGroup, error)
	ListByOU(ctx context.Context, ouID string) ([]SessionGroup, error)
	Update(ctx context.Context, g SessionGroup) error
	Delete(ctx context.Context, id string) error
	DefaultExistsForOU(ctx context.Context, ouID string) (bool, error)
}

var getDBProvider = provider.GetDBProvider

type sessionGroupStore struct {
	dbProvider   provider.DBProviderInterface
	deploymentID string
}

func newSessionGroupStore() (sessionGroupStoreInterface, transaction.Transactioner, error) {
	p := getDBProvider()
	t, err := p.GetUserDBTransactioner()
	if err != nil {
		return nil, nil, err
	}
	return &sessionGroupStore{
		dbProvider:   p,
		deploymentID: config.GetServerRuntime().Config.Server.Identifier,
	}, t, nil
}

func (s *sessionGroupStore) Create(ctx context.Context, g SessionGroup) error {
	db, err := s.dbProvider.GetUserDBClient()
	if err != nil {
		return fmt.Errorf("failed to get db client: %w", err)
	}
	_, err = db.ExecuteContext(ctx, queryCreateSessionGroup,
		s.deploymentID, g.ID, g.OUID, g.Name, string(g.Mode), g.IsDefault, g.CreatedAt, g.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create session group: %w", err)
	}
	return nil
}

func (s *sessionGroupStore) GetByID(ctx context.Context, id string) (*SessionGroup, error) {
	db, err := s.dbProvider.GetUserDBClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get db client: %w", err)
	}
	rows, err := db.QueryContext(ctx, queryGetSessionGroupByID, id, s.deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query session group: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrSessionGroupNotFound
	}
	return buildSessionGroupFromRow(rows[0])
}

func (s *sessionGroupStore) GetDefaultForOU(ctx context.Context, ouID string) (*SessionGroup, error) {
	db, err := s.dbProvider.GetUserDBClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get db client: %w", err)
	}
	rows, err := db.QueryContext(ctx, queryGetDefaultSessionGroupForOU, ouID, s.deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query default session group: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrSessionGroupNotFound
	}
	return buildSessionGroupFromRow(rows[0])
}

func (s *sessionGroupStore) ListByOU(ctx context.Context, ouID string) ([]SessionGroup, error) {
	db, err := s.dbProvider.GetUserDBClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get db client: %w", err)
	}
	rows, err := db.QueryContext(ctx, queryListSessionGroupsByOU, ouID, s.deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list session groups: %w", err)
	}
	groups := make([]SessionGroup, 0, len(rows))
	for _, row := range rows {
		g, err := buildSessionGroupFromRow(row)
		if err != nil {
			return nil, err
		}
		groups = append(groups, *g)
	}
	return groups, nil
}

func (s *sessionGroupStore) Update(ctx context.Context, g SessionGroup) error {
	db, err := s.dbProvider.GetUserDBClient()
	if err != nil {
		return fmt.Errorf("failed to get db client: %w", err)
	}
	_, err = db.ExecuteContext(ctx, queryUpdateSessionGroupByID,
		g.ID, g.Name, string(g.Mode), g.UpdatedAt, s.deploymentID)
	if err != nil {
		return fmt.Errorf("failed to update session group: %w", err)
	}
	return nil
}

func (s *sessionGroupStore) Delete(ctx context.Context, id string) error {
	db, err := s.dbProvider.GetUserDBClient()
	if err != nil {
		return fmt.Errorf("failed to get db client: %w", err)
	}
	_, err = db.ExecuteContext(ctx, queryDeleteSessionGroupByID, id, s.deploymentID)
	if err != nil {
		return fmt.Errorf("failed to delete session group: %w", err)
	}
	return nil
}

func (s *sessionGroupStore) DefaultExistsForOU(ctx context.Context, ouID string) (bool, error) {
	db, err := s.dbProvider.GetUserDBClient()
	if err != nil {
		return false, fmt.Errorf("failed to get db client: %w", err)
	}
	rows, err := db.QueryContext(ctx, queryCheckDefaultExistsForOU, ouID, s.deploymentID)
	if err != nil {
		return false, fmt.Errorf("failed to check default group: %w", err)
	}
	if len(rows) == 0 {
		return false, nil
	}
	if count, ok := rows[0]["count"].(int64); ok {
		return count > 0, nil
	}
	return false, nil
}

func buildSessionGroupFromRow(row map[string]interface{}) (*SessionGroup, error) {
	id, ok := row["session_group_id"].(string)
	if !ok {
		return nil, fmt.Errorf("session_group_id is not a string")
	}
	ouID, ok := row["ou_id"].(string)
	if !ok {
		return nil, fmt.Errorf("ou_id is not a string")
	}
	name, ok := row["name"].(string)
	if !ok {
		return nil, fmt.Errorf("name is not a string")
	}
	mode, ok := row["session_mode"].(string)
	if !ok {
		return nil, fmt.Errorf("session_mode is not a string")
	}
	isDefault := parseBool(row["is_default"])

	createdAt, err := parseTimeField(row["created_at"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse created_at: %w", err)
	}
	updatedAt, err := parseTimeField(row["updated_at"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse updated_at: %w", err)
	}

	return &SessionGroup{
		ID:        id,
		OUID:      ouID,
		Name:      name,
		Mode:      SessionMode(mode),
		IsDefault: isDefault,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

// parseBool handles bool columns returned as bool, int64, or string by different DB drivers.
func parseBool(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case int64:
		return val != 0
	case string:
		return strings.EqualFold(val, "true") || val == "1"
	}
	return false
}

func parseTimeField(field interface{}) (time.Time, error) {
	const customFmt = "2006-01-02 15:04:05.999999999"
	switch v := field.(type) {
	case time.Time:
		return v, nil
	case string:
		parts := strings.SplitN(v, " ", 3)
		trimmed := v
		if len(parts) >= 2 {
			trimmed = parts[0] + " " + parts[1]
		}
		t, err := time.Parse(customFmt, trimmed)
		if err != nil {
			t, err = time.Parse(time.RFC3339, v)
			if err != nil {
				return time.Time{}, fmt.Errorf("cannot parse time %q: %w", v, err)
			}
		}
		return t, nil
	case nil:
		return time.Time{}, fmt.Errorf("time field is nil")
	default:
		return time.Time{}, fmt.Errorf("unexpected time type %T", field)
	}
}
