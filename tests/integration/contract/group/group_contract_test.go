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

// Package group holds runtime contract tests for the API quality gate (see
// api-quality-gate/). Unlike the broader integration suites, these assert the CONTRACT
// the OpenAPI spec promises, at runtime, against a live ThunderID instance: a usable
// pagination "next" link, 409 on conflict, and a membership lifecycle that is visible
// through the listing.
//
// validate-test-coverage (api-quality-gate/scripts/check-coverage.mjs) requires every
// operationId in api/group.yaml to appear in coveredOperations below and to resolve to
// a test that exists and does not skip. The scheduled audit goes further and reports
// coverage from what each test DID, so a skipped or failing test counts for nothing.
package group

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/thunder-id/thunderid/tests/integration/testutils"
)

const testServerURL = "https://localhost:8095"

// coveredOperations maps every operationId in api/group.yaml to the test that exercises
// it. Naming a test here cannot certify coverage on its own: validate-test-coverage
// requires the named function to exist and not skip, and the audit reports coverage
// from what the test actually did at runtime, so a skipped or failing test counts for
// nothing.
var coveredOperations = map[string]string{
	"createGroup":        "TestGroupLifecycle",
	"getGroup":           "TestGroupLifecycle",
	"updateGroup":        "TestGroupLifecycle",
	"deleteGroup":        "TestGroupLifecycle",
	"listGroups":         "TestListGroupsNextCursorUsable",
	"createGroupByPath":  "TestListGroupsByPathNextCursorUsable",
	"listGroupsByPath":   "TestListGroupsByPathNextCursorUsable",
	"listGroupMembers":   "TestGroupMemberLifecycle",
	"addGroupMembers":    "TestGroupMemberLifecycle",
	"removeGroupMembers": "TestGroupMemberLifecycle",
}

type link struct {
	Href string `json:"href"`
	Rel  string `json:"rel"`
}

type group struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	OUID        string `json:"ouId"`
}

type createGroupRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	OUID        string   `json:"ouId,omitempty"`
	Members     []string `json:"members,omitempty"`
}

type groupListResponse struct {
	TotalResults int     `json:"totalResults"`
	StartIndex   int     `json:"startIndex"`
	Count        int     `json:"count"`
	Groups       []group `json:"groups"`
	Links        []link  `json:"links"`
}

type GroupContractTestSuite struct {
	suite.Suite
	ouID       string
	ouHandle   string
	userTypeID string
	userID     string
}

func TestGroupContractTestSuite(t *testing.T) {
	suite.Run(t, new(GroupContractTestSuite))
}

func (s *GroupContractTestSuite) SetupSuite() {
	s.ouHandle = "contract-test-group-ou"
	ouID, err := testutils.CreateOrganizationUnit(testutils.OrganizationUnit{
		Handle:      s.ouHandle,
		Name:        "Contract Test OU for Groups",
		Description: "Organization unit created for the API contract quality gate",
	})
	s.Require().NoError(err, "failed to create test organization unit")
	s.ouID = ouID

	// A real member is needed to exercise the member operations; the group
	// integration suite seeds a user the same way.
	userTypeID, err := testutils.CreateUserType(testutils.UserType{
		Name: "contract-test-person",
		OUID: s.ouID,
		Schema: map[string]interface{}{
			"email":      map[string]interface{}{"type": "string"},
			"given_name": map[string]interface{}{"type": "string"},
		},
	})
	s.Require().NoError(err, "failed to create test user type")
	s.userTypeID = userTypeID

	userID, err := testutils.CreateUser(testutils.User{
		Type:       "contract-test-person",
		OUID:       s.ouID,
		Attributes: json.RawMessage(`{"email":"contract@example.com","given_name":"Contract"}`),
	})
	s.Require().NoError(err, "failed to create test user")
	s.userID = userID
}

func (s *GroupContractTestSuite) TearDownSuite() {
	if s.userID != "" {
		_ = testutils.DeleteUser(s.userID)
	}
	if s.userTypeID != "" {
		_ = testutils.DeleteUserType(s.userTypeID)
	}
	if s.ouID != "" {
		// Best effort; groups created by tests clean up after themselves.
		_ = testutils.DeleteOrganizationUnit(s.ouID)
	}
}

// createGroup POSTs to /groups (operationId: createGroup) and returns the created id.
func (s *GroupContractTestSuite) createGroup(name string) (string, int) {
	body, err := json.Marshal(createGroupRequest{Name: name, OUID: s.ouID})
	s.Require().NoError(err)

	client := testutils.GetHTTPClient()
	req, err := http.NewRequest(http.MethodPost, testServerURL+"/groups", bytes.NewBuffer(body))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", resp.StatusCode
	}
	var g group
	s.Require().NoError(json.NewDecoder(resp.Body).Decode(&g))
	return g.ID, resp.StatusCode
}

// TestGroupLifecycle exercises createGroup -> getGroup -> updateGroup -> deleteGroup and
// asserts the conflict contract (a duplicate name under the same OU returns 409).
func (s *GroupContractTestSuite) TestGroupLifecycle() {
	client := testutils.GetHTTPClient()

	// createGroup
	id, status := s.createGroup("contract-lifecycle")
	s.Require().Equal(http.StatusCreated, status, "createGroup should return 201")
	s.Require().NotEmpty(id)
	defer func() { _ = testutils.DeleteGroup(id) }()

	// getGroup
	req, err := http.NewRequest(http.MethodGet, testServerURL+"/groups/"+id, nil)
	s.Require().NoError(err)
	resp, err := client.Do(req)
	s.Require().NoError(err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode, "getGroup should return 200")
	var got group
	s.Require().NoError(json.Unmarshal(body, &got))
	s.Equal("contract-lifecycle", got.Name)
	s.Equal(s.ouID, got.OUID)

	// Conflict contract: a second create with the same name+OU must return 409 and must
	// not create a duplicate.
	_, dupStatus := s.createGroup("contract-lifecycle")
	s.Equal(http.StatusConflict, dupStatus, "duplicate group name in the same OU must return 409")

	// updateGroup
	upd, err := json.Marshal(createGroupRequest{Name: "contract-lifecycle-renamed", OUID: s.ouID})
	s.Require().NoError(err)
	req, err = http.NewRequest(http.MethodPut, testServerURL+"/groups/"+id, bytes.NewBuffer(upd))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	s.Require().NoError(err)
	resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode, "updateGroup should return 200")

	// deleteGroup
	req, err = http.NewRequest(http.MethodDelete, testServerURL+"/groups/"+id, nil)
	s.Require().NoError(err)
	resp, err = client.Do(req)
	s.Require().NoError(err)
	resp.Body.Close()
	s.Require().Equal(http.StatusNoContent, resp.StatusCode, "deleteGroup should return 204")

	// The deleted group must be gone.
	req, err = http.NewRequest(http.MethodGet, testServerURL+"/groups/"+id, nil)
	s.Require().NoError(err)
	resp, err = client.Do(req)
	s.Require().NoError(err)
	resp.Body.Close()
	s.Equal(http.StatusNotFound, resp.StatusCode, "getGroup on a deleted group must return 404")
}

// TestListGroupsNextCursorUsable proves the "next" pagination link returned by listGroups
// is real and usable: following it yields another successful page (not a 4xx/5xx).
func (s *GroupContractTestSuite) TestListGroupsNextCursorUsable() {
	// Ensure at least two groups exist so a next link is produced under limit=1.
	id1, st1 := s.createGroup("contract-page-1")
	s.Require().Equal(http.StatusCreated, st1)
	defer func() { _ = testutils.DeleteGroup(id1) }()
	id2, st2 := s.createGroup("contract-page-2")
	s.Require().Equal(http.StatusCreated, st2)
	defer func() { _ = testutils.DeleteGroup(id2) }()

	list := s.getGroupList(testServerURL + "/groups?limit=1&offset=0")
	s.Require().Greater(list.TotalResults, 1, "expected more than one group so a next link is produced")
	s.Require().LessOrEqual(list.Count, 1, "limit=1 must return at most one group")

	next := findRel(list.Links, "next")
	s.Require().NotEmpty(next, "listGroups must return a usable next link when more results exist")

	// The next link is a relative URL (e.g. "groups?offset=1&limit=1"); it must be usable.
	nextList := s.getGroupList(s.nextURL(next))
	s.Equal(1, nextList.Count, "following the next link must return the next page")
}

// TestListGroupsByPathNextCursorUsable creates groups under an OU handle path
// (createGroupByPath) and pages the OU-scoped listing (listGroupsByPath), asserting the
// next link is usable within a deterministic, OU-scoped result set.
func (s *GroupContractTestSuite) TestListGroupsByPathNextCursorUsable() {
	client := testutils.GetHTTPClient()
	base := testServerURL + "/groups/tree/" + s.ouHandle

	created := make([]string, 0, 2)
	for _, name := range []string{"contract-path-1", "contract-path-2"} {
		body, err := json.Marshal(createGroupRequest{Name: name})
		s.Require().NoError(err)
		req, err := http.NewRequest(http.MethodPost, base, bytes.NewBuffer(body))
		s.Require().NoError(err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		s.Require().NoError(err)
		var g group
		_ = json.NewDecoder(resp.Body).Decode(&g)
		resp.Body.Close()
		s.Require().Equal(http.StatusCreated, resp.StatusCode, "createGroupByPath should return 201")
		if g.ID != "" {
			created = append(created, g.ID)
		}
	}
	defer func() {
		for _, id := range created {
			_ = testutils.DeleteGroup(id)
		}
	}()

	list := s.getGroupList(base + "?limit=1&offset=0")
	s.Require().Equal(2, list.TotalResults, "OU-scoped listing should see exactly the two created groups")
	next := findRel(list.Links, "next")
	s.Require().NotEmpty(next, "listGroupsByPath must return a usable next link")
	nextList := s.getGroupList(s.nextURL(next))
	s.Equal(1, nextList.Count, "following the next link must return the second page")
}

func (s *GroupContractTestSuite) getGroupList(url string) groupListResponse {
	client := testutils.GetHTTPClient()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	s.Require().NoError(err)
	resp, err := client.Do(req)
	s.Require().NoError(err)
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode, "list %s should return 200", url)
	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	var out groupListResponse
	s.Require().NoError(json.Unmarshal(body, &out))
	return out
}

// nextURL resolves a pagination href against the server root. The href is
// server-relative and may or may not carry a leading slash, so joining it blindly
// produced "//groups", which only worked because the server 307-redirects. Following
// a link the API did not hand out is not a test of that link.
func (s *GroupContractTestSuite) nextURL(href string) string {
	return testServerURL + "/" + strings.TrimPrefix(href, "/")
}

func findRel(links []link, rel string) string {
	for _, l := range links {
		if l.Rel == rel {
			return l.Href
		}
	}
	return ""
}

type member struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type membersRequest struct {
	Members []member `json:"members"`
}

type memberListResponse struct {
	TotalResults int      `json:"totalResults"`
	Count        int      `json:"count"`
	Members      []member `json:"members"`
	Links        []link   `json:"links"`
}

// postMembers drives addGroupMembers / removeGroupMembers and returns the status.
func (s *GroupContractTestSuite) postMembers(path string, ids ...string) int {
	ms := make([]member, 0, len(ids))
	for _, id := range ids {
		ms = append(ms, member{Type: "user", ID: id})
	}
	body, err := json.Marshal(membersRequest{Members: ms})
	s.Require().NoError(err)

	req, err := http.NewRequest(http.MethodPost, testServerURL+path, bytes.NewBuffer(body))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutils.GetHTTPClient().Do(req)
	s.Require().NoError(err)
	defer resp.Body.Close()
	return resp.StatusCode
}

// listMembers drives listGroupMembers.
func (s *GroupContractTestSuite) listMembers(groupID string) memberListResponse {
	req, err := http.NewRequest(http.MethodGet, testServerURL+"/groups/"+groupID+"/members", nil)
	s.Require().NoError(err)
	resp, err := testutils.GetHTTPClient().Do(req)
	s.Require().NoError(err)
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode, "listGroupMembers should return 200")
	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	var out memberListResponse
	s.Require().NoError(json.Unmarshal(body, &out))
	return out
}

// TestGroupMemberLifecycle asserts the membership contract end to end: a member added
// through addGroupMembers is visible to listGroupMembers, and is gone after
// removeGroupMembers. It replaces a t.Skip stub that claimed coverage for these three
// operations without exercising anything.
func (s *GroupContractTestSuite) TestGroupMemberLifecycle() {
	id, status := s.createGroup("contract-members")
	s.Require().Equal(http.StatusCreated, status, "createGroup should return 201")
	defer func() { _ = testutils.DeleteGroup(id) }()

	s.Require().Empty(s.listMembers(id).Members, "a new group should have no members")

	s.Require().Equal(http.StatusOK, s.postMembers("/groups/"+id+"/members/add", s.userID),
		"addGroupMembers should return 200")

	added := s.listMembers(id)
	s.Require().Len(added.Members, 1, "the added member must be listed")
	s.Equal(s.userID, added.Members[0].ID)
	s.Equal("user", added.Members[0].Type)
	s.Equal(1, added.TotalResults, "totalResults must count the added member")

	s.Require().Equal(http.StatusOK, s.postMembers("/groups/"+id+"/members/remove", s.userID),
		"removeGroupMembers should return 200")

	s.Empty(s.listMembers(id).Members, "the removed member must no longer be listed")
}
