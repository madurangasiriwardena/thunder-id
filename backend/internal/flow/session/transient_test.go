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
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestModelsCarryNoTransientExecutionState guards the storage boundary: transient per-execution
// state (current node, partial inputs, runtime data, execution history, challenge token) belongs
// to the flow engine's execution store (FLOW_CONTEXT), never the session or its auth context.
//
// On the SSO path a fresh execution runs and loads the durable auth-event facts from the auth
// context; it does not resume persisted execution state from the session. This test fails if a
// transient-looking field is ever added to the session models.
func TestModelsCarryNoTransientExecutionState(t *testing.T) {
	forbidden := []string{
		"runtimedata",
		"userinput",
		"currentnode",
		"currentaction",
		"currentsegment",
		"executionhistory",
		"challengetoken",
		"forwardeddata",
		"partialinput",
		"flowstate",
	}

	types := []reflect.Type{
		reflect.TypeOf(Session{}),
		reflect.TypeOf(AuthContext{}),
		reflect.TypeOf(SubjectSnapshot{}),
		reflect.TypeOf(StepFact{}),
	}

	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			name := strings.ToLower(typ.Field(i).Name)
			for _, f := range forbidden {
				assert.NotContains(t, name, f,
					"%s.%s looks like transient execution state; it belongs in the flow store, not the session",
					typ.Name(), typ.Field(i).Name)
			}
		}
	}
}
