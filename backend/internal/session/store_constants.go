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

import dbmodel "github.com/thunder-id/thunderid/internal/system/database/model"

var (
	// queryCreateSession inserts a new SESSION_RECORD row.
	// Args: $1=DEPLOYMENT_ID, $2=SESSION_ID, $3=SUBJECT_ID, $4=SESSION_GROUP_ID,
	//       $5=AUTHENTICATED_AT, $6=ASSURANCE_LEVEL, $7=CREATED_AT, $8=LAST_ACTIVE_AT,
	//       $9=IDLE_EXPIRES_AT, $10=ABSOLUTE_EXPIRES_AT, $11=HANDLE_ID,
	//       $12=HANDLE_ISSUED_AT, $13=HANDLE_EXPIRES_AT, $14=BINDING_TYPE,
	//       $15=SESSION_STATE, $16=VERSION
	queryCreateSession = dbmodel.DBQuery{
		ID: "SSQ-SRS-01",
		Query: `INSERT INTO "SESSION_RECORD" ` +
			`(DEPLOYMENT_ID, SESSION_ID, SUBJECT_ID, SESSION_GROUP_ID, ` +
			`AUTHENTICATED_AT, ASSURANCE_LEVEL, CREATED_AT, LAST_ACTIVE_AT, ` +
			`IDLE_EXPIRES_AT, ABSOLUTE_EXPIRES_AT, HANDLE_ID, HANDLE_ISSUED_AT, ` +
			`HANDLE_EXPIRES_AT, BINDING_TYPE, SESSION_STATE, VERSION) ` +
			`VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
	}

	// queryGetSessionByHandle retrieves a SESSION_RECORD by HANDLE_ID scoped to the deployment.
	// Args: $1=HANDLE_ID, $2=DEPLOYMENT_ID
	queryGetSessionByHandle = dbmodel.DBQuery{
		ID: "SSQ-SRS-02",
		Query: `SELECT SESSION_ID, SUBJECT_ID, SESSION_GROUP_ID, AUTHENTICATED_AT, ` +
			`ASSURANCE_LEVEL, CREATED_AT, LAST_ACTIVE_AT, IDLE_EXPIRES_AT, ` +
			`ABSOLUTE_EXPIRES_AT, HANDLE_ID, HANDLE_ISSUED_AT, HANDLE_EXPIRES_AT, ` +
			`BINDING_TYPE, SESSION_STATE, VERSION ` +
			`FROM "SESSION_RECORD" WHERE HANDLE_ID = $1 AND DEPLOYMENT_ID = $2`,
	}

	// queryTouchSession performs an optimistic-locking update of LAST_ACTIVE_AT + VERSION.
	// Returns rowsAffected > 0 on success; rowsAffected == 0 means VERSION mismatch (lost race).
	// Args: $1=SESSION_ID, $2=LAST_ACTIVE_AT, $3=VERSION (current), $4=DEPLOYMENT_ID
	queryTouchSession = dbmodel.DBQuery{
		ID: "SSQ-SRS-03",
		Query: `UPDATE "SESSION_RECORD" ` +
			`SET LAST_ACTIVE_AT = $2, VERSION = VERSION + 1 ` +
			`WHERE SESSION_ID = $1 AND VERSION = $3 AND DEPLOYMENT_ID = $4`,
	}

	// queryGetSessionByID retrieves a SESSION_RECORD by its internal PK.
	// Args: $1=SESSION_ID, $2=DEPLOYMENT_ID
	queryGetSessionByID = dbmodel.DBQuery{
		ID: "SSQ-SRS-04",
		Query: `SELECT SESSION_ID, SUBJECT_ID, SESSION_GROUP_ID, AUTHENTICATED_AT, ` +
			`ASSURANCE_LEVEL, CREATED_AT, LAST_ACTIVE_AT, IDLE_EXPIRES_AT, ` +
			`ABSOLUTE_EXPIRES_AT, HANDLE_ID, HANDLE_ISSUED_AT, HANDLE_EXPIRES_AT, ` +
			`BINDING_TYPE, SESSION_STATE, VERSION ` +
			`FROM "SESSION_RECORD" WHERE SESSION_ID = $1 AND DEPLOYMENT_ID = $2`,
	}
)
