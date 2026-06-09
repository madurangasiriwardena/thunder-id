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
	// queryCreateClientSession inserts a new CLIENT_SESSION row.
	// Args: $1=DEPLOYMENT_ID, $2=CLIENT_SESSION_ID, $3=SESSION_ID, $4=CLIENT_ID,
	//       $5=OIDC_SID, $6=CREATED_AT, $7=LAST_USED_AT, $8=STATUS,
	//       $9=GRANTED_SCOPES, $10=VERSION
	queryCreateClientSession = dbmodel.DBQuery{
		ID: "SSQ-CSS-01",
		Query: `INSERT INTO "CLIENT_SESSION" ` +
			`(DEPLOYMENT_ID, CLIENT_SESSION_ID, SESSION_ID, CLIENT_ID, OIDC_SID, ` +
			`CREATED_AT, LAST_USED_AT, STATUS, GRANTED_SCOPES, VERSION) ` +
			`VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
	}

	// queryGetClientSessionBySessionAndClient retrieves a CLIENT_SESSION by SESSION_ID and CLIENT_ID.
	// Args: $1=SESSION_ID, $2=CLIENT_ID, $3=DEPLOYMENT_ID
	queryGetClientSessionBySessionAndClient = dbmodel.DBQuery{
		ID: "SSQ-CSS-02",
		Query: `SELECT CLIENT_SESSION_ID, SESSION_ID, CLIENT_ID, OIDC_SID, ` +
			`CREATED_AT, LAST_USED_AT, STATUS, GRANTED_SCOPES, VERSION ` +
			`FROM "CLIENT_SESSION" WHERE SESSION_ID = $1 AND CLIENT_ID = $2 AND DEPLOYMENT_ID = $3`,
	}

	// queryGetClientSessionByID retrieves a CLIENT_SESSION by its PK.
	// Args: $1=CLIENT_SESSION_ID, $2=DEPLOYMENT_ID
	queryGetClientSessionByID = dbmodel.DBQuery{
		ID: "SSQ-CSS-03",
		Query: `SELECT CLIENT_SESSION_ID, SESSION_ID, CLIENT_ID, OIDC_SID, ` +
			`CREATED_AT, LAST_USED_AT, STATUS, GRANTED_SCOPES, VERSION ` +
			`FROM "CLIENT_SESSION" WHERE CLIENT_SESSION_ID = $1 AND DEPLOYMENT_ID = $2`,
	}
)
