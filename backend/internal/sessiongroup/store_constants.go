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

import dbmodel "github.com/thunder-id/thunderid/internal/system/database/model"

var queryCreateSessionGroup = dbmodel.DBQuery{
	ID: "SGQ-001",
	Query: `INSERT INTO "SESSION_GROUP"
		(DEPLOYMENT_ID, SESSION_GROUP_ID, OU_ID, NAME, SESSION_MODE, IS_DEFAULT, CREATED_AT, UPDATED_AT)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
}

var queryGetSessionGroupByID = dbmodel.DBQuery{
	ID: "SGQ-002",
	Query: `SELECT SESSION_GROUP_ID, OU_ID, NAME, SESSION_MODE, IS_DEFAULT, CREATED_AT, UPDATED_AT
		FROM "SESSION_GROUP"
		WHERE SESSION_GROUP_ID = $1 AND DEPLOYMENT_ID = $2`,
}

var queryGetDefaultSessionGroupForOU = dbmodel.DBQuery{
	ID: "SGQ-003",
	Query: `SELECT SESSION_GROUP_ID, OU_ID, NAME, SESSION_MODE, IS_DEFAULT, CREATED_AT, UPDATED_AT
		FROM "SESSION_GROUP"
		WHERE OU_ID = $1 AND IS_DEFAULT = TRUE AND DEPLOYMENT_ID = $2
		LIMIT 1`,
}

var queryListSessionGroupsByOU = dbmodel.DBQuery{
	ID: "SGQ-004",
	Query: `SELECT SESSION_GROUP_ID, OU_ID, NAME, SESSION_MODE, IS_DEFAULT, CREATED_AT, UPDATED_AT
		FROM "SESSION_GROUP"
		WHERE OU_ID = $1 AND DEPLOYMENT_ID = $2
		ORDER BY IS_DEFAULT DESC, CREATED_AT ASC`,
}

var queryDeleteSessionGroupByID = dbmodel.DBQuery{
	ID: "SGQ-005",
	Query: `DELETE FROM "SESSION_GROUP"
		WHERE SESSION_GROUP_ID = $1 AND DEPLOYMENT_ID = $2`,
}

var queryUpdateSessionGroupByID = dbmodel.DBQuery{
	ID: "SGQ-006",
	Query: `UPDATE "SESSION_GROUP"
		SET NAME = $2, SESSION_MODE = $3, UPDATED_AT = $4
		WHERE SESSION_GROUP_ID = $1 AND DEPLOYMENT_ID = $5`,
}

var queryCheckDefaultExistsForOU = dbmodel.DBQuery{
	ID: "SGQ-007",
	Query: `SELECT COUNT(*) AS count FROM "SESSION_GROUP"
		WHERE OU_ID = $1 AND IS_DEFAULT = TRUE AND DEPLOYMENT_ID = $2`,
}
