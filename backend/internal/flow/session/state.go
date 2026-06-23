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
	"time"
)

// DefaultHandleTTL is the lifetime of a freshly minted session handle, used both for the
// handle's expiry and the carrier cookie's max-age.
// TODO(sso): make the handle lifetime configurable via deployment config.
const DefaultHandleTTL = time.Hour

// allowedClaims is the allow-list of subject claim keys that may be snapshotted into the auth
// context. Anything outside this set is dropped, so raw inputs, credentials, and tokens never
// reach the saved snapshot.
var allowedClaims = map[string]struct{}{
	"sub":                {},
	"email":              {},
	"email_verified":     {},
	"name":               {},
	"given_name":         {},
	"family_name":        {},
	"preferred_username": {},
	"username":           {},
	"locale":             {},
	"picture":            {},
}

// SanitizeClaims projects raw attributes onto the allow-list, stringifying scalar values and
// dropping everything else. The result is safe to snapshot into the auth context.
func SanitizeClaims(attributes map[string]interface{}) map[string]string {
	if len(attributes) == 0 {
		return nil
	}
	claims := make(map[string]string)
	for key, value := range attributes {
		if _, ok := allowedClaims[key]; !ok {
			continue
		}
		if s, ok := stringifyScalar(value); ok {
			claims[key] = s
		}
	}
	if len(claims) == 0 {
		return nil
	}
	return claims
}

// stringifyScalar converts a scalar attribute value to a string, reporting false for
// non-scalar values (slices, maps, nil), which are not persisted.
func stringifyScalar(value interface{}) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case bool:
		return fmt.Sprintf("%t", v), true
	case int, int32, int64, float32, float64:
		return fmt.Sprintf("%v", v), true
	default:
		return "", false
	}
}

// Revoker is the seam for revoking a subject's sessions when their identity attributes or
// standing change. It is intentionally unimplemented in this POC.
// TODO(sso): implement subject-scoped revocation on identity-attribute/standing change.
type Revoker interface {
	RevokeBySubject(ctx context.Context, subjectID string) error
}
