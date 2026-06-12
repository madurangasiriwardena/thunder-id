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

package testutils

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

// NewSSOClient returns an HTTP client with a cookie jar suitable for SSO flows.
// The jar preserves the per-group session cookies (__Host-tid_session_<groupId>) across requests.
// Redirects are not followed so callers can inspect the Location header.
func NewSSOClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// StartAuthorize issues GET /oauth2/authorize using the provided client (which carries any
// existing session cookies) and returns the raw Location header, the authId, and the
// executionId extracted from it.
// prompt may be empty, "none", "login", etc.
func StartAuthorize(client *http.Client, clientID, redirectURI, scope, prompt string) (
	location, authID, executionID string, err error) {

	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", scope)
	params.Set("state", "sso-test-state")
	if prompt != "" {
		params.Set("prompt", prompt)
	}

	req, err := http.NewRequest(http.MethodGet,
		TestServerURL+"/oauth2/authorize?"+params.Encode(), nil)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to build authorize request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("authorize request failed: %w", err)
	}
	defer resp.Body.Close()

	location = resp.Header.Get("Location")
	if location == "" {
		body, _ := io.ReadAll(resp.Body)
		return "", "", "", fmt.Errorf("no Location in authorize response (status %d): %s",
			resp.StatusCode, string(body))
	}

	authID, executionID, err = ExtractAuthData(location)
	if err != nil {
		// Not a gate redirect — e.g. direct redirect to client (silent SSO) or error redirect.
		// Return the location and let the caller decide.
		return location, "", "", nil
	}
	return location, authID, executionID, nil
}

// SilentAuthorize issues GET /oauth2/authorize?prompt=<prompt> using the client and returns
// the Location header without following the redirect.
// On SSO hit the location will be the app's redirect URI with a code.
// On no session the location will carry error=login_required (or error=interaction_required
// for step-up when prompt=none).
func SilentAuthorize(client *http.Client, clientID, redirectURI, scope, prompt string) (string, error) {
	location, _, _, err := StartAuthorize(client, clientID, redirectURI, scope, prompt)
	return location, err
}

// ExecuteAuthenticationFlowWithClient is the cookie-jar-aware variant of ExecuteAuthenticationFlow.
// It uses the provided client so that any Set-Cookie headers on the response (session cookie on
// flow completion) are stored in the jar.
func ExecuteAuthenticationFlowWithClient(client *http.Client, executionID string,
	inputs map[string]string, action string, challengeToken ...string) (*FlowStep, error) {

	flowData := map[string]interface{}{
		"executionId": executionID,
	}
	if len(inputs) > 0 {
		flowData["inputs"] = inputs
	}
	if action != "" {
		flowData["action"] = action
	}
	if len(challengeToken) > 0 && challengeToken[0] != "" {
		flowData["challengeToken"] = challengeToken[0]
	}

	flowJSON, err := json.Marshal(flowData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal flow data: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, TestServerURL+"/flow/execute", bytes.NewBuffer(flowJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create flow request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute flow: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("flow execution failed with status %d: %s", resp.StatusCode, string(body))
	}

	var step FlowStep
	if err := json.Unmarshal(body, &step); err != nil {
		return nil, fmt.Errorf("failed to decode flow response: %w", err)
	}
	return &step, nil
}

// CompleteAuthorizationWithClient is the cookie-jar-aware variant of CompleteAuthorization.
// Using the same client ensures the session cookie is already in the jar from the flow/execute
// response; this call returns the redirect URI containing the authorization code.
func CompleteAuthorizationWithClient(client *http.Client, authID, assertion string) (*AuthorizationResponse, error) {
	body, err := json.Marshal(map[string]interface{}{
		"authId":    authID,
		"assertion": assertion,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal auth callback data: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, TestServerURL+"/oauth2/auth/callback", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create auth callback request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth callback request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth callback failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var authzResp AuthorizationResponse
	if err := json.Unmarshal(respBody, &authzResp); err != nil {
		return nil, fmt.Errorf("failed to decode auth callback response: %w", err)
	}
	return &authzResp, nil
}

// InteractiveLogin performs a full interactive password-based authorization code flow using the
// provided cookie-jar client, and returns the authorization code.
// The session cookie is stored in client.Jar after a successful login.
func InteractiveLogin(client *http.Client, clientID, redirectURI, scope, username, password string) (string, error) {
	// Step 1: Initiate authorize — expect a gate redirect with authId + executionId.
	location, authID, executionID, err := StartAuthorize(client, clientID, redirectURI, scope, "")
	if err != nil {
		return "", fmt.Errorf("StartAuthorize: %w", err)
	}
	if authID == "" || executionID == "" {
		return "", fmt.Errorf("authorize did not redirect to gate (location=%s)", location)
	}

	// Step 2: Initial flow step (no inputs) — gets the login prompt + challenge token.
	initial, err := ExecuteAuthenticationFlowWithClient(client, executionID, nil, "")
	if err != nil {
		return "", fmt.Errorf("initial flow step: %w", err)
	}
	if initial.FlowStatus == "COMPLETE" {
		// Shouldn't happen on a fresh client, but handle gracefully.
		return completeAndExtractCode(client, authID, initial.Assertion, redirectURI)
	}

	// Step 3: Submit credentials.
	credStep, err := ExecuteAuthenticationFlowWithClient(client, executionID,
		map[string]string{"username": username, "password": password},
		"action_001", initial.ChallengeToken)
	if err != nil {
		return "", fmt.Errorf("credentials flow step: %w", err)
	}

	if credStep.FlowStatus != "COMPLETE" {
		stepJSON, _ := json.Marshal(credStep)
		return "", fmt.Errorf("flow not complete after credentials (status=%s): %s",
			credStep.FlowStatus, string(stepJSON))
	}

	return completeAndExtractCode(client, authID, credStep.Assertion, redirectURI)
}

// InteractiveLoginWithOTP performs a full interactive password + SMS-OTP authorization code flow.
// otpFn is called after OTP is sent to retrieve the one-time password (e.g. mockServer.GetLastMessage().OTP).
func InteractiveLoginWithOTP(client *http.Client, clientID, redirectURI, scope, username, password string,
	otpFn func() string) (string, error) {

	location, authID, executionID, err := StartAuthorize(client, clientID, redirectURI, scope, "")
	if err != nil {
		return "", fmt.Errorf("StartAuthorize: %w", err)
	}
	if authID == "" || executionID == "" {
		return "", fmt.Errorf("authorize did not redirect to gate (location=%s)", location)
	}

	initial, err := ExecuteAuthenticationFlowWithClient(client, executionID, nil, "")
	if err != nil {
		return "", fmt.Errorf("initial flow step: %w", err)
	}

	credStep, err := ExecuteAuthenticationFlowWithClient(client, executionID,
		map[string]string{"username": username, "password": password},
		"action_001", initial.ChallengeToken)
	if err != nil {
		return "", fmt.Errorf("credentials flow step: %w", err)
	}
	if credStep.FlowStatus == "COMPLETE" {
		return completeAndExtractCode(client, authID, credStep.Assertion, redirectURI)
	}

	// OTP step.
	otp := otpFn()
	if otp == "" {
		return "", fmt.Errorf("otpFn returned empty OTP")
	}

	otpStep, err := ExecuteAuthenticationFlowWithClient(client, executionID,
		map[string]string{"otp": otp},
		"action_002", credStep.ChallengeToken)
	if err != nil {
		return "", fmt.Errorf("OTP flow step: %w", err)
	}
	if otpStep.FlowStatus != "COMPLETE" {
		stepJSON, _ := json.Marshal(otpStep)
		return "", fmt.Errorf("flow not complete after OTP (status=%s): %s",
			otpStep.FlowStatus, string(stepJSON))
	}

	return completeAndExtractCode(client, authID, otpStep.Assertion, redirectURI)
}

// StepUpWithOTP drives a step-up flow (session already has password, only OTP remains).
// The initial execute is expected to trigger OTP send; otpFn retrieves the OTP.
func StepUpWithOTP(client *http.Client, authID, executionID string, otpFn func() string) (string, error) {
	// Initial step — engine skips satisfied password factor, sends OTP automatically.
	initial, err := ExecuteAuthenticationFlowWithClient(client, executionID, nil, "")
	if err != nil {
		return "", fmt.Errorf("initial step-up step: %w", err)
	}
	if initial.FlowStatus == "COMPLETE" {
		return "", fmt.Errorf("unexpected COMPLETE on initial step-up step (expected OTP prompt)")
	}

	otp := otpFn()
	if otp == "" {
		return "", fmt.Errorf("otpFn returned empty OTP for step-up")
	}

	otpStep, err := ExecuteAuthenticationFlowWithClient(client, executionID,
		map[string]string{"otp": otp},
		"action_002", initial.ChallengeToken)
	if err != nil {
		return "", fmt.Errorf("OTP step-up step: %w", err)
	}
	if otpStep.FlowStatus != "COMPLETE" {
		stepJSON, _ := json.Marshal(otpStep)
		return "", fmt.Errorf("flow not complete after OTP step-up (status=%s): %s",
			otpStep.FlowStatus, string(stepJSON))
	}

	return otpStep.Assertion, nil
}

// AssertLoginRequired returns true when the location contains error=login_required.
func AssertLoginRequired(location string) bool {
	u, err := url.Parse(location)
	if err != nil {
		return false
	}
	return u.Query().Get("error") == "login_required"
}

// IsGateRedirect returns true when the location looks like a gate (login page) redirect,
// i.e. it carries authId and executionId query parameters.
func IsGateRedirect(location string) bool {
	u, err := url.Parse(location)
	if err != nil {
		return false
	}
	return u.Query().Get("authId") != "" && u.Query().Get("executionId") != ""
}

// IsInteractionRequired returns true when the location contains error=interaction_required.
func IsInteractionRequired(location string) bool {
	u, err := url.Parse(location)
	if err != nil {
		return false
	}
	return u.Query().Get("error") == "interaction_required"
}

// HasAuthorizationCode returns true when the location is a redirect to the app's redirect URI
// with a code parameter (i.e. silent SSO succeeded).
func HasAuthorizationCode(location, redirectURI string) bool {
	if !strings.HasPrefix(location, redirectURI) {
		return false
	}
	u, err := url.Parse(location)
	if err != nil {
		return false
	}
	return u.Query().Get("code") != ""
}

// completeAndExtractCode calls the auth callback with the assertion and extracts the code from
// the returned redirect URI.
func completeAndExtractCode(client *http.Client, authID, assertion, redirectURI string) (string, error) {
	authzResp, err := CompleteAuthorizationWithClient(client, authID, assertion)
	if err != nil {
		return "", fmt.Errorf("CompleteAuthorizationWithClient: %w", err)
	}
	_ = redirectURI
	code, err := ExtractAuthorizationCode(authzResp.RedirectURI)
	if err != nil {
		return "", fmt.Errorf("extract code from %s: %w", authzResp.RedirectURI, err)
	}
	return code, nil
}
