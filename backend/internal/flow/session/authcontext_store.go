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

	"github.com/thunder-id/thunderid/internal/system/database/provider"
)

// AuthContextStoreInterface defines persistence for the 1:1 session auth context. The context
// payload is encrypted at rest via the configured Encryptor.
type AuthContextStoreInterface interface {
	// Create persists the auth context for a session.
	Create(ctx context.Context, c AuthContext) error
	// GetBySessionID fetches a session's auth context. It returns (nil, nil) when none exists.
	GetBySessionID(ctx context.Context, sessionID string) (*AuthContext, error)
	// Delete removes a session's auth context.
	Delete(ctx context.Context, sessionID string) error
}

// authContextStore implements AuthContextStoreInterface against the runtime relational store.
type authContextStore struct {
	dbProvider   provider.DBProviderInterface
	deploymentID string
	encryptor    Encryptor
}

// NewAuthContextStore creates an auth-context store. The encryptor is the encryption-at-rest
// seam applied to the serialized context payload.
func NewAuthContextStore(dbProvider provider.DBProviderInterface, deploymentID string,
	encryptor Encryptor) AuthContextStoreInterface {
	return &authContextStore{
		dbProvider:   dbProvider,
		deploymentID: deploymentID,
		encryptor:    encryptor,
	}
}

// Create persists the auth context, encrypting the serialized payload at rest. It rejects
// payloads exceeding MaxAuthContextBytes.
func (st *authContextStore) Create(ctx context.Context, c AuthContext) error {
	payload, err := c.serializePayload()
	if err != nil {
		return err
	}
	if len(payload) > MaxAuthContextBytes {
		return ErrAuthContextTooLarge
	}
	ciphertext, err := st.encryptor.Encrypt(payload)
	if err != nil {
		return fmt.Errorf("failed to encrypt auth context: %w", err)
	}

	return withRuntimeDBClient(st.dbProvider, func(dbClient provider.DBClientInterface) error {
		_, execErr := dbClient.ExecuteContext(ctx, QueryCreateAuthContext,
			c.SessionID, st.deploymentID, ciphertext, c.ContextVersion)
		if execErr != nil {
			return fmt.Errorf("failed to create auth context: %w", execErr)
		}
		return nil
	})
}

// GetBySessionID fetches and decrypts a session's auth context.
func (st *authContextStore) GetBySessionID(ctx context.Context, sessionID string) (*AuthContext, error) {
	var result *AuthContext

	err := withRuntimeDBClient(st.dbProvider, func(dbClient provider.DBClientInterface) error {
		results, queryErr := dbClient.QueryContext(ctx, QueryGetAuthContextBySessionID, sessionID, st.deploymentID)
		if queryErr != nil {
			return fmt.Errorf("failed to execute query: %w", queryErr)
		}
		if len(results) == 0 {
			return nil
		}
		if len(results) != 1 {
			return fmt.Errorf("unexpected number of results: %d", len(results))
		}

		c, buildErr := st.buildAuthContextFromRow(results[0])
		if buildErr != nil {
			return buildErr
		}
		result = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Delete removes a session's auth context.
func (st *authContextStore) Delete(ctx context.Context, sessionID string) error {
	return withRuntimeDBClient(st.dbProvider, func(dbClient provider.DBClientInterface) error {
		_, err := dbClient.ExecuteContext(ctx, QueryDeleteAuthContext, sessionID, st.deploymentID)
		if err != nil {
			return fmt.Errorf("failed to delete auth context: %w", err)
		}
		return nil
	})
}

// buildAuthContextFromRow decrypts and parses a result row into an AuthContext.
func (st *authContextStore) buildAuthContextFromRow(row map[string]interface{}) (*AuthContext, error) {
	sessionID, err := parseString(row["session_id"], "session_id")
	if err != nil {
		return nil, err
	}
	contextVersion, err := parseInt(row["context_version"], "context_version")
	if err != nil {
		return nil, err
	}
	ciphertext := parseNullableString(row["context"])

	plaintext, err := st.encryptor.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt auth context: %w", err)
	}
	payload, err := parseAuthContextPayload(plaintext)
	if err != nil {
		return nil, err
	}

	return &AuthContext{
		SessionID:      sessionID,
		Subject:        payload.Subject,
		CompletedSteps: payload.CompletedSteps,
		SnapshotClaims: payload.SnapshotClaims,
		ContextVersion: contextVersion,
	}, nil
}
