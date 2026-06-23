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
	"time"
)

// Resolver loads a session from an opaque handle and returns it only when it is live and
// the presented binding matches. It does not check flow identity or version — that is the
// SSO-Check node's responsibility.
type Resolver interface {
	// Resolve returns the session referenced by handleID when it is ACTIVE, within its
	// deadlines at now, and bound to presentedBinding. It returns (nil, nil) for every
	// "no live session" case (absent, ended/revoked, expired, binding mismatch), and a
	// non-nil error only on a store failure.
	Resolve(ctx context.Context, handleID, presentedBinding string, now time.Time) (*Session, error)
}

type resolver struct {
	store StoreInterface
}

// NewResolver creates a Resolver backed by the given session store.
func NewResolver(store StoreInterface) Resolver {
	return &resolver{store: store}
}

// Resolve implements Resolver.
func (r *resolver) Resolve(ctx context.Context, handleID, presentedBinding string,
	now time.Time) (*Session, error) {
	if handleID == "" {
		return nil, nil
	}

	s, err := r.store.GetByHandle(ctx, handleID)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, nil
	}

	if s.State != StateActive {
		return nil, nil
	}
	if expired(s.HandleExpiresAt, now) || expired(s.IdleExpiresAt, now) || expired(s.AbsoluteExpiresAt, now) {
		return nil, nil
	}
	if s.Binding != presentedBinding {
		return nil, nil
	}

	return s, nil
}

// expired reports whether a deadline is set and has been reached at now. A zero deadline
// means "no deadline" and never expires.
func expired(deadline, now time.Time) bool {
	return !deadline.IsZero() && !now.Before(deadline)
}
