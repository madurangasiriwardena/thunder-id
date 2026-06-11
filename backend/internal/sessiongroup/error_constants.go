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
	"errors"

	"github.com/thunder-id/thunderid/internal/system/error/serviceerror"
	"github.com/thunder-id/thunderid/internal/system/i18n/core"
)

// Client-facing service errors for session group management.
var (
	ErrorInvalidRequestFormat = serviceerror.ServiceError{
		Type: serviceerror.ClientErrorType,
		Code: "SG-1001",
		Error: core.I18nMessage{
			Key:          "error.sessiongroup.invalid_request_format",
			DefaultValue: "Invalid request format",
		},
		ErrorDescription: core.I18nMessage{
			Key:          "error.sessiongroup.invalid_request_format_description",
			DefaultValue: "The request body is malformed or required fields are missing",
		},
	}
	ErrorSessionGroupNotFound = serviceerror.ServiceError{
		Type: serviceerror.ClientErrorType,
		Code: "SG-1002",
		Error: core.I18nMessage{
			Key:          "error.sessiongroup.not_found",
			DefaultValue: "Session group not found",
		},
		ErrorDescription: core.I18nMessage{
			Key:          "error.sessiongroup.not_found_description",
			DefaultValue: "The session group with the specified ID does not exist",
		},
	}
	ErrorMissingSessionGroupID = serviceerror.ServiceError{
		Type: serviceerror.ClientErrorType,
		Code: "SG-1003",
		Error: core.I18nMessage{
			Key:          "error.sessiongroup.missing_id",
			DefaultValue: "Missing session group ID",
		},
		ErrorDescription: core.I18nMessage{
			Key:          "error.sessiongroup.missing_id_description",
			DefaultValue: "Session group ID is required",
		},
	}
	ErrorDuplicateDefault = serviceerror.ServiceError{
		Type: serviceerror.ClientErrorType,
		Code: "SG-1004",
		Error: core.I18nMessage{
			Key:          "error.sessiongroup.duplicate_default",
			DefaultValue: "Default group already exists",
		},
		ErrorDescription: core.I18nMessage{
			Key:          "error.sessiongroup.duplicate_default_description",
			DefaultValue: "An OU can have at most one default session group",
		},
	}
	ErrorCannotDeleteDefault = serviceerror.ServiceError{
		Type: serviceerror.ClientErrorType,
		Code: "SG-1005",
		Error: core.I18nMessage{
			Key:          "error.sessiongroup.cannot_delete_default",
			DefaultValue: "Cannot delete default session group",
		},
		ErrorDescription: core.I18nMessage{
			Key:          "error.sessiongroup.cannot_delete_default_description",
			DefaultValue: "The default session group for an OU cannot be deleted",
		},
	}
	ErrorInvalidSessionMode = serviceerror.ServiceError{
		Type: serviceerror.ClientErrorType,
		Code: "SG-1006",
		Error: core.I18nMessage{
			Key:          "error.sessiongroup.invalid_mode",
			DefaultValue: "Invalid session mode",
		},
		ErrorDescription: core.I18nMessage{
			Key:          "error.sessiongroup.invalid_mode_description",
			DefaultValue: "Session mode must be 'managed' or 'sessionless'",
		},
	}
	ErrorMissingOUID = serviceerror.ServiceError{
		Type: serviceerror.ClientErrorType,
		Code: "SG-1007",
		Error: core.I18nMessage{
			Key:          "error.sessiongroup.missing_ou_id",
			DefaultValue: "Missing OU ID",
		},
		ErrorDescription: core.I18nMessage{
			Key:          "error.sessiongroup.missing_ou_id_description",
			DefaultValue: "OU ID is required to create or list session groups",
		},
	}
	ErrorOUTenantMismatch = serviceerror.ServiceError{
		Type: serviceerror.ClientErrorType,
		Code: "SG-1008",
		Error: core.I18nMessage{
			Key:          "error.sessiongroup.ou_tenant_mismatch",
			DefaultValue: "Session group does not belong to this OU",
		},
		ErrorDescription: core.I18nMessage{
			Key:          "error.sessiongroup.ou_tenant_mismatch_description",
			DefaultValue: "The requested session group belongs to a different OU",
		},
	}
)

// Internal sentinel errors.
var (
	ErrSessionGroupNotFound = errors.New("session group not found")
	ErrDuplicateDefault     = errors.New("duplicate default session group")
)
