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

package authz

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/thunder-id/thunderid/tests/integration/testutils"
)

const (
	ssoMockNotificationServerPort = 8097

	ssoClientSecret   = "sso_test_secret"
	ssoRedirectURIOU1 = "https://localhost:3000/sso-callback-ou1"
	ssoRedirectURIOU2 = "https://localhost:3000/sso-callback-ou2"

	ssoUsername = "sso_test_user"
	ssoPassword = "testpassword123"
)

// SSO session group integration test suite.
// Fixture layout:
//
//	OU1 (ssoOU1): appA→G1/flowPwd, appB→G1/flowPwd, appD→no group, appF→G1/flowPwdOtp
//	OU2 (ssoOU2): appC→G2/flowPwd, appE→no group
//
// G1 and G2 are distinct groups; appD and appE have no explicit group so they resolve to the
// deployment-default SSO boundary.
type SSOSessionTestSuite struct {
	suite.Suite

	ouID1 string
	ouID2 string

	userSchemaID string
	userIDs      []string

	flowPwdID    string
	flowPwdOtpID string

	groupID1 string
	groupID2 string

	appAID string // OU1, G1, flowPwd
	appBID string // OU1, G1, flowPwd
	appCID string // OU2, G2, flowPwd
	appDID string // OU1, no group, flowPwd
	appEID string // OU2, no group, flowPwd
	appFID string // OU1, G1, flowPwdOtp (step-up)

	mockServer *testutils.MockNotificationServer
	senderID   string
}

func TestSSOSessionTestSuite(t *testing.T) {
	suite.Run(t, new(SSOSessionTestSuite))
}

// ---- Flow definitions ----

var ssoPwdFlow = testutils.Flow{
	Name:     "SSO Test Password Flow",
	FlowType: "AUTHENTICATION",
	Handle:   "sso_test_pwd_flow",
	Nodes: []map[string]interface{}{
		{"id": "start", "type": "START", "onSuccess": "prompt_credentials"},
		{
			"id":   "prompt_credentials",
			"type": "PROMPT",
			"prompts": []map[string]interface{}{
				{
					"inputs": []map[string]interface{}{
						{"ref": "input_001", "identifier": "username", "type": "string", "required": true},
						{"ref": "input_002", "identifier": "password", "type": "string", "required": true},
					},
					"action": map[string]interface{}{"ref": "action_001", "nextNode": "basic_auth"},
				},
			},
		},
		{
			"id":        "basic_auth",
			"type":      "TASK_EXECUTION",
			"executor":  map[string]interface{}{"name": "BasicAuthExecutor"},
			"onSuccess": "auth_assert",
		},
		{
			"id":        "auth_assert",
			"type":      "TASK_EXECUTION",
			"executor":  map[string]interface{}{"name": "AuthAssertExecutor"},
			"onSuccess": "end",
		},
		{"id": "end", "type": "END"},
	},
}

// ssoPwdOtpFlow is the MFA flow: password + SMS-OTP.
// The senderId placeholders are replaced in SetupSuite after the sender is created.
var ssoPwdOtpFlow = testutils.Flow{
	Name:     "SSO Test Password+OTP Flow",
	FlowType: "AUTHENTICATION",
	Handle:   "sso_test_pwd_otp_flow",
	Nodes: []map[string]interface{}{
		{"id": "start", "type": "START", "onSuccess": "prompt_credentials"},
		{
			"id":   "prompt_credentials",
			"type": "PROMPT",
			"prompts": []map[string]interface{}{
				{
					"inputs": []map[string]interface{}{
						{"ref": "input_001", "identifier": "username", "type": "string", "required": true},
						{"ref": "input_002", "identifier": "password", "type": "string", "required": true},
					},
					"action": map[string]interface{}{"ref": "action_001", "nextNode": "basic_auth"},
				},
			},
		},
		{
			"id":        "basic_auth",
			"type":      "TASK_EXECUTION",
			"executor":  map[string]interface{}{"name": "BasicAuthExecutor"},
			"onSuccess": "sms_otp_send",
		},
		{
			"id":   "sms_otp_send",
			"type": "TASK_EXECUTION",
			"properties": map[string]interface{}{
				"senderId": "placeholder-sender-id",
			},
			"executor":  map[string]interface{}{"name": "SMSOTPAuthExecutor", "mode": "send"},
			"onSuccess": "prompt_otp",
		},
		{
			"id":   "prompt_otp",
			"type": "PROMPT",
			"prompts": []map[string]interface{}{
				{
					"inputs": []map[string]interface{}{
						{"ref": "input_003", "identifier": "otp", "type": "string", "required": true},
					},
					"action": map[string]interface{}{"ref": "action_002", "nextNode": "sms_otp_verify"},
				},
			},
		},
		{
			"id":   "sms_otp_verify",
			"type": "TASK_EXECUTION",
			"properties": map[string]interface{}{
				"senderId": "placeholder-sender-id",
			},
			"executor":  map[string]interface{}{"name": "SMSOTPAuthExecutor", "mode": "verify"},
			"onSuccess": "auth_assert",
		},
		{
			"id":        "auth_assert",
			"type":      "TASK_EXECUTION",
			"executor":  map[string]interface{}{"name": "AuthAssertExecutor"},
			"onSuccess": "end",
		},
		{"id": "end", "type": "END"},
	},
}

// ---- User schema ----

var ssoUserSchema = testutils.UserType{
	Name: "sso_test_user",
	Schema: map[string]interface{}{
		"username": map[string]interface{}{"type": "string"},
		"password": map[string]interface{}{"type": "string", "credential": true},
		"mobileNumber": map[string]interface{}{"type": "string"},
	},
}

func (ts *SSOSessionTestSuite) SetupSuite() {
	// Create organization units.
	ouID1, err := testutils.CreateOrganizationUnit(testutils.OrganizationUnit{
		Handle:      "sso-test-ou1",
		Name:        "SSO Test OU1",
		Description: "SSO test organization unit 1",
	})
	ts.Require().NoError(err, "create OU1")
	ts.ouID1 = ouID1

	ouID2, err := testutils.CreateOrganizationUnit(testutils.OrganizationUnit{
		Handle:      "sso-test-ou2",
		Name:        "SSO Test OU2",
		Description: "SSO test organization unit 2",
	})
	ts.Require().NoError(err, "create OU2")
	ts.ouID2 = ouID2

	// Create user schema under OU1.
	ssoUserSchema.OUID = ouID1
	ssoUserSchema.AllowSelfRegistration = true
	schemaID, err := testutils.CreateUserType(ssoUserSchema)
	ts.Require().NoError(err, "create user schema")
	ts.userSchemaID = schemaID

	// Create the test user under OU1.
	testUser := testutils.User{
		OUID: ouID1,
		Type: ssoUserSchema.Name,
		Attributes: json.RawMessage(`{
			"username": "` + ssoUsername + `",
			"password": "` + ssoPassword + `",
			"mobileNumber": "+15550001234"
		}`),
	}
	userIDs, err := testutils.CreateMultipleUsers(testUser)
	ts.Require().NoError(err, "create test user")
	ts.userIDs = userIDs

	// Start mock notification server for SMS-OTP.
	ts.mockServer = testutils.NewMockNotificationServer(ssoMockNotificationServerPort)
	ts.Require().NoError(ts.mockServer.Start(), "start mock notification server")
	time.Sleep(100 * time.Millisecond)

	senderID, err := testutils.CreateNotificationSender(testutils.NotificationSender{
		Name:        "SSO Test SMS Sender",
		Description: "Sender for SSO session group tests",
		Provider:    "custom",
		Properties: []testutils.SenderProperty{
			{Name: "url", Value: ts.mockServer.GetSendSMSURL(), IsSecret: false},
			{Name: "http_method", Value: "POST", IsSecret: false},
			{Name: "content_type", Value: "JSON", IsSecret: false},
		},
	})
	ts.Require().NoError(err, "create notification sender")
	ts.senderID = senderID

	// Inject real senderID into the OTP flow nodes before creating the flow.
	pwdOtpNodes := ssoPwdOtpFlow.Nodes.([]map[string]interface{})
	pwdOtpNodes[3]["properties"].(map[string]interface{})["senderId"] = senderID
	pwdOtpNodes[5]["properties"].(map[string]interface{})["senderId"] = senderID
	ssoPwdOtpFlow.Nodes = pwdOtpNodes

	// Create flows.
	flowPwdID, err := testutils.CreateFlow(ssoPwdFlow)
	ts.Require().NoError(err, "create password flow")
	ts.flowPwdID = flowPwdID

	flowPwdOtpID, err := testutils.CreateFlow(ssoPwdOtpFlow)
	ts.Require().NoError(err, "create password+OTP flow")
	ts.flowPwdOtpID = flowPwdOtpID

	// Create session groups.
	groupID1, err := testutils.CreateSessionGroup(ouID1, "SSO Group 1", "managed")
	ts.Require().NoError(err, "create group G1")
	ts.groupID1 = groupID1

	groupID2, err := testutils.CreateSessionGroup(ouID2, "SSO Group 2", "managed")
	ts.Require().NoError(err, "create group G2")
	ts.groupID2 = groupID2

	// Create applications.
	//
	// appA — OU1, G1, flowPwd
	appAID, err := testutils.CreateApplication(testutils.Application{
		Name:             "SSO App A",
		OUID:             ouID1,
		AuthFlowID:       flowPwdID,
		ClientID:         "sso_app_a",
		ClientSecret:     ssoClientSecret,
		RedirectURIs:     []string{ssoRedirectURIOU1},
		AllowedUserTypes: []string{ssoUserSchema.Name},
		SessionGroupID:   groupID1,
	})
	ts.Require().NoError(err, "create appA")
	ts.appAID = appAID

	// appB — OU1, G1, flowPwd
	appBID, err := testutils.CreateApplication(testutils.Application{
		Name:             "SSO App B",
		OUID:             ouID1,
		AuthFlowID:       flowPwdID,
		ClientID:         "sso_app_b",
		ClientSecret:     ssoClientSecret,
		RedirectURIs:     []string{ssoRedirectURIOU1},
		AllowedUserTypes: []string{ssoUserSchema.Name},
		SessionGroupID:   groupID1,
	})
	ts.Require().NoError(err, "create appB")
	ts.appBID = appBID

	// appC — OU2, G2, flowPwd (different group, different OU)
	appCID, err := testutils.CreateApplication(testutils.Application{
		Name:             "SSO App C",
		OUID:             ouID2,
		AuthFlowID:       flowPwdID,
		ClientID:         "sso_app_c",
		ClientSecret:     ssoClientSecret,
		RedirectURIs:     []string{ssoRedirectURIOU2},
		AllowedUserTypes: []string{ssoUserSchema.Name},
		SessionGroupID:   groupID2,
	})
	ts.Require().NoError(err, "create appC")
	ts.appCID = appCID

	// appD — OU1, no explicit group (deployment default)
	appDID, err := testutils.CreateApplication(testutils.Application{
		Name:             "SSO App D",
		OUID:             ouID1,
		AuthFlowID:       flowPwdID,
		ClientID:         "sso_app_d",
		ClientSecret:     ssoClientSecret,
		RedirectURIs:     []string{ssoRedirectURIOU1},
		AllowedUserTypes: []string{ssoUserSchema.Name},
	})
	ts.Require().NoError(err, "create appD")
	ts.appDID = appDID

	// appE — OU2, no explicit group (deployment default, same boundary as appD)
	appEID, err := testutils.CreateApplication(testutils.Application{
		Name:             "SSO App E",
		OUID:             ouID2,
		AuthFlowID:       flowPwdID,
		ClientID:         "sso_app_e",
		ClientSecret:     ssoClientSecret,
		RedirectURIs:     []string{ssoRedirectURIOU2},
		AllowedUserTypes: []string{ssoUserSchema.Name},
	})
	ts.Require().NoError(err, "create appE")
	ts.appEID = appEID

	// appF — OU1, G1, flowPwdOtp (step-up: password already satisfied by appA session)
	appFID, err := testutils.CreateApplication(testutils.Application{
		Name:             "SSO App F",
		OUID:             ouID1,
		AuthFlowID:       flowPwdOtpID,
		ClientID:         "sso_app_f",
		ClientSecret:     ssoClientSecret,
		RedirectURIs:     []string{ssoRedirectURIOU1},
		AllowedUserTypes: []string{ssoUserSchema.Name},
		SessionGroupID:   groupID1,
	})
	ts.Require().NoError(err, "create appF")
	ts.appFID = appFID
}

func (ts *SSOSessionTestSuite) TearDownSuite() {
	if err := testutils.CleanupUsers(ts.userIDs); err != nil {
		ts.T().Logf("failed to cleanup users: %v", err)
	}

	for _, appID := range []string{ts.appAID, ts.appBID, ts.appCID, ts.appDID, ts.appEID, ts.appFID} {
		if appID != "" {
			if err := testutils.DeleteApplication(appID); err != nil {
				ts.T().Logf("failed to delete app %s: %v", appID, err)
			}
		}
	}

	for _, groupID := range []string{ts.groupID1, ts.groupID2} {
		if groupID != "" {
			if err := testutils.DeleteSessionGroup(groupID); err != nil {
				ts.T().Logf("failed to delete session group %s: %v", groupID, err)
			}
		}
	}

	for _, flowID := range []string{ts.flowPwdID, ts.flowPwdOtpID} {
		if flowID != "" {
			if err := testutils.DeleteFlow(flowID); err != nil {
				ts.T().Logf("failed to delete flow %s: %v", flowID, err)
			}
		}
	}

	if ts.senderID != "" {
		if err := testutils.DeleteNotificationSender(ts.senderID); err != nil {
			ts.T().Logf("failed to delete notification sender: %v", err)
		}
	}

	if ts.mockServer != nil {
		_ = ts.mockServer.Stop()
	}

	if ts.userSchemaID != "" {
		if err := testutils.DeleteUserType(ts.userSchemaID); err != nil {
			ts.T().Logf("failed to delete user schema: %v", err)
		}
	}

	for _, ouID := range []string{ts.ouID1, ts.ouID2} {
		if ouID != "" {
			if err := testutils.DeleteOrganizationUnit(ouID); err != nil {
				ts.T().Logf("failed to delete OU %s: %v", ouID, err)
			}
		}
	}
}

// ---- Test cases ----

// TestSSO_SameGroup_SilentCode verifies that two apps in the same session group share a session:
// after logging in to appA (G1), appB (G1) issues a code without a second login.
func (ts *SSOSessionTestSuite) TestSSO_SameGroup_SilentCode() {
	client := testutils.NewSSOClient()

	// Login to appA — establishes the G1 session cookie.
	_, err := testutils.InteractiveLogin(client, "sso_app_a", ssoRedirectURIOU1, "openid",
		ssoUsername, ssoPassword)
	ts.Require().NoError(err, "interactive login to appA")

	// Silent authorize for appB — same group G1, should get a code directly.
	location, err := testutils.SilentAuthorize(client, "sso_app_b", ssoRedirectURIOU1, "openid", "none")
	ts.Require().NoError(err, "silent authorize for appB")

	ts.Require().True(testutils.HasAuthorizationCode(location, ssoRedirectURIOU1),
		"expected authorization code in redirect, got: %s", location)
	ts.Require().False(testutils.AssertLoginRequired(location),
		"unexpected login_required for same-group SSO")
}

// TestSSO_DifferentGroups_LoginRequired verifies that two apps in different session groups
// do not share a session: after logging in to appA (G1), appC (G2) returns login_required.
func (ts *SSOSessionTestSuite) TestSSO_DifferentGroups_LoginRequired() {
	client := testutils.NewSSOClient()

	// Login to appA (G1).
	_, err := testutils.InteractiveLogin(client, "sso_app_a", ssoRedirectURIOU1, "openid",
		ssoUsername, ssoPassword)
	ts.Require().NoError(err, "interactive login to appA")

	// Silent authorize for appC (G2) — different group, no session.
	location, err := testutils.SilentAuthorize(client, "sso_app_c", ssoRedirectURIOU2, "openid", "none")
	ts.Require().NoError(err, "silent authorize for appC")

	ts.Require().True(testutils.AssertLoginRequired(location),
		"expected login_required for different-group apps, got: %s", location)
}

// TestSSO_GroupVsNoGroup_LoginRequired verifies that an app with an explicit group and an app
// without an explicit group do not share a session.
func (ts *SSOSessionTestSuite) TestSSO_GroupVsNoGroup_LoginRequired() {
	client := testutils.NewSSOClient()

	// Login to appA (G1).
	_, err := testutils.InteractiveLogin(client, "sso_app_a", ssoRedirectURIOU1, "openid",
		ssoUsername, ssoPassword)
	ts.Require().NoError(err, "interactive login to appA")

	// Silent authorize for appD (deployment default) — different boundary.
	location, err := testutils.SilentAuthorize(client, "sso_app_d", ssoRedirectURIOU1, "openid", "none")
	ts.Require().NoError(err, "silent authorize for appD")

	ts.Require().True(testutils.AssertLoginRequired(location),
		"expected login_required (group vs deployment-default), got: %s", location)
}

// TestSSO_BothUnassigned_SilentCode (bonus case from case 3's setup) verifies that two apps with
// no explicit session group share the deployment-level default session boundary.
func (ts *SSOSessionTestSuite) TestSSO_BothUnassigned_SilentCode() {
	client := testutils.NewSSOClient()

	// Login to appD (no group → deployment default), OU1.
	_, err := testutils.InteractiveLogin(client, "sso_app_d", ssoRedirectURIOU1, "openid",
		ssoUsername, ssoPassword)
	ts.Require().NoError(err, "interactive login to appD")

	// Silent authorize for appE (no group → deployment default), OU2 — same boundary.
	location, err := testutils.SilentAuthorize(client, "sso_app_e", ssoRedirectURIOU2, "openid", "none")
	ts.Require().NoError(err, "silent authorize for appE")

	ts.Require().True(testutils.HasAuthorizationCode(location, ssoRedirectURIOU2),
		"expected silent code for unassigned apps (deployment default SSO), got: %s", location)
	ts.Require().False(testutils.AssertLoginRequired(location),
		"unexpected login_required for deployment-default SSO")
}

// TestSSO_StepUp_OnlyOTPPrompted verifies that when appF (G1, password+OTP) is accessed after
// logging in to appA (G1, password only), the flow engine skips the password factor (already
// satisfied by the session) and only prompts for the OTP.
func (ts *SSOSessionTestSuite) TestSSO_StepUp_OnlyOTPPrompted() {
	client := testutils.NewSSOClient()

	// Login to appA (G1, password only) — establishes G1 session with CredentialsAuthenticator.
	_, err := testutils.InteractiveLogin(client, "sso_app_a", ssoRedirectURIOU1, "openid",
		ssoUsername, ssoPassword)
	ts.Require().NoError(err, "interactive login to appA")

	// Authorize for appF (G1, password+OTP) — should trigger step-up (not silent code, not login_required).
	// Use no prompt restriction so the server returns a gate redirect for the step-up flow.
	location, authID, executionID, err := testutils.StartAuthorize(client, "sso_app_f",
		ssoRedirectURIOU1, "openid", "")
	ts.Require().NoError(err, "start authorize for appF")
	ts.Require().True(testutils.IsGateRedirect(location),
		"expected gate redirect for step-up, got: %s", location)
	ts.Require().NotEmpty(authID)
	ts.Require().NotEmpty(executionID)

	// Clear any stale messages before driving the OTP flow.
	ts.mockServer.ClearMessages()

	// Initial flow step — engine skips the satisfied password factor and triggers OTP send.
	initial, err := testutils.ExecuteAuthenticationFlowWithClient(client, executionID, nil, "")
	ts.Require().NoError(err, "initial step-up execute")
	ts.Require().Equal("INCOMPLETE", initial.FlowStatus, "expected OTP prompt after password skip")

	// Verify that we are at the OTP prompt, not the credentials prompt.
	// The presence of an 'otp' input (and absence of 'username'/'password') confirms the engine
	// skipped the already-satisfied password factor.
	ts.Require().NotNil(initial.Data, "flow step data must be present")
	hasOTPInput := false
	hasCredentialsInput := false
	for _, prompt := range initial.Data.Inputs {
		switch prompt.Identifier {
		case "otp":
			hasOTPInput = true
		case "username", "password":
			hasCredentialsInput = true
		}
	}
	ts.Require().True(hasOTPInput, "expected OTP input in step-up prompt")
	ts.Require().False(hasCredentialsInput, "password factor must not be re-prompted in step-up")

	// Retrieve the OTP from the mock notification server.
	time.Sleep(500 * time.Millisecond)
	lastMessage := ts.mockServer.GetLastMessage()
	ts.Require().NotNil(lastMessage, "mock server must have received the OTP SMS")
	ts.Require().NotEmpty(lastMessage.OTP, "OTP must be present in the SMS message")

	// Submit OTP.
	otpStep, err := testutils.ExecuteAuthenticationFlowWithClient(client, executionID,
		map[string]string{"otp": lastMessage.OTP}, "action_002", initial.ChallengeToken)
	ts.Require().NoError(err, "OTP flow step")
	ts.Require().Equal("COMPLETE", otpStep.FlowStatus, "flow must complete after OTP")
	ts.Require().NotEmpty(otpStep.Assertion, "assertion must be present after OTP")

	// Complete authorization.
	authzResp, err := testutils.CompleteAuthorizationWithClient(client, authID, otpStep.Assertion)
	ts.Require().NoError(err, "complete authorization for appF")
	ts.Require().NotEmpty(authzResp.RedirectURI)

	code, err := testutils.ExtractAuthorizationCode(authzResp.RedirectURI)
	ts.Require().NoError(err, "must get authorization code from appF after step-up")
	ts.Require().NotEmpty(code)

	// Verify that subsequent silent access to appF (same group, now both factors satisfied)
	// issues a code without interaction.
	location2, err := testutils.SilentAuthorize(client, "sso_app_f", ssoRedirectURIOU1, "openid", "none")
	ts.Require().NoError(err, "second silent authorize for appF")

	ts.Assert().True(testutils.HasAuthorizationCode(location2, ssoRedirectURIOU1) ||
		testutils.IsGateRedirect(location2),
		"after step-up, subsequent appF access should either be silent or a gate redirect (not login_required): %s",
		location2)
	ts.Assert().False(testutils.AssertLoginRequired(location2),
		"login_required must not appear after step-up: %s", location2)
	// The ideal outcome is a direct code (both factors now in session).
	if testutils.HasAuthorizationCode(location2, ssoRedirectURIOU1) {
		// Session has been updated with both factors — silent SSO works.
		_, err = testutils.ExtractAuthorizationCode(location2)
		ts.Require().NoError(err)
	}
}

// ---- HTTP token exchange helpers ----

// exchangeCode performs the authorization_code token exchange and returns the HTTP status.
func exchangeCode(clientID, secret, code, redirectURI string) (int, error) {
	result, err := testutils.RequestToken(clientID, secret, code, redirectURI, "authorization_code")
	if err != nil {
		return 0, err
	}
	return result.StatusCode, nil
}

// assertCodeExchangeable confirms that a code can be exchanged for a token.
func (ts *SSOSessionTestSuite) assertCodeExchangeable(clientID, redirectURI, code string) {
	ts.T().Helper()
	status, err := exchangeCode(clientID, ssoClientSecret, code, redirectURI)
	ts.Require().NoError(err)
	ts.Require().Equal(http.StatusOK, status, "code exchange must succeed for %s", clientID)
}
