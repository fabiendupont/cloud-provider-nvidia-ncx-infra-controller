/*
Copyright 2026 Fabien Dupont.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	keycloakRealm        = "carbide-dev"
	keycloakClientID     = "carbide-api"
	keycloakClientSecret = "carbide-local-secret"
	keycloakUsername      = "admin@example.com"
	keycloakPassword     = "adminpassword"
)

// getKeycloakToken acquires a JWT from Keycloak using the resource owner password grant.
func getKeycloakToken() string {
	keycloakURL := os.Getenv("NVIDIA_CARBIDE_KEYCLOAK_URL")
	Expect(keycloakURL).NotTo(BeEmpty(), "NVIDIA_CARBIDE_KEYCLOAK_URL must be set")

	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", keycloakURL, keycloakRealm)

	data := url.Values{
		"grant_type":    {"password"},
		"client_id":     {keycloakClientID},
		"client_secret": {keycloakClientSecret},
		"username":      {keycloakUsername},
		"password":      {keycloakPassword},
	}

	resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	Expect(err).NotTo(HaveOccurred(), "Failed to request Keycloak token")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK),
		"Keycloak token request failed with status %d: %s", resp.StatusCode, string(body))

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	Expect(json.Unmarshal(body, &tokenResp)).To(Succeed())
	Expect(tokenResp.AccessToken).NotTo(BeEmpty(), "Received empty access token from Keycloak")

	_, _ = fmt.Fprintf(GinkgoWriter, "Successfully acquired Keycloak token\n")
	return tokenResp.AccessToken
}

// createCloudConfigSecret creates a Kubernetes secret with the cloud-config for the CCM.
func createCloudConfigSecret(endpoint, orgName, token, siteID, tenantID string) string {
	return fmt.Sprintf(`endpoint: %q
orgName: %q
token: %q
siteId: %q
tenantId: %q
`, endpoint, orgName, token, siteID, tenantID)
}

// carbideAPIRequest makes an authenticated request to the Carbide REST API.
func carbideAPIRequest(method, path, token string, body interface{}) (map[string]interface{}, int) {
	endpoint := os.Getenv("NVIDIA_CARBIDE_API_ENDPOINT")
	Expect(endpoint).NotTo(BeEmpty())

	var reqBody io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		reqBody = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequest(method, endpoint+path, reqBody)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())

	var result map[string]interface{}
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &result)
	}

	_, _ = fmt.Fprintf(GinkgoWriter, "%s %s -> %d\n", method, path, resp.StatusCode)
	return result, resp.StatusCode
}

// getExistingSiteID finds the local-dev-site created by setup-local.sh.
// VPC creation triggers a Temporal workflow that requires a site-agent — only
// the pre-provisioned local-dev-site has one connected.
func getExistingSiteID(token, orgName string) string {
	endpoint := os.Getenv("NVIDIA_CARBIDE_API_ENDPOINT")
	apiBase := fmt.Sprintf("/v2/org/%s/carbide", orgName)

	req, err := http.NewRequest("GET", endpoint+apiBase+"/site", nil)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())

	var sites []map[string]interface{}
	Expect(json.Unmarshal(body, &sites)).To(Succeed())
	Expect(sites).NotTo(BeEmpty(), "No sites found — was setup-local.sh run?")

	siteID := sites[0]["id"].(string)
	siteName := sites[0]["name"].(string)
	_, _ = fmt.Fprintf(GinkgoWriter, "Using existing site %s (id=%s)\n", siteName, siteID)
	return siteID
}

// ensureSiteRegistered ensures the site is in Registered state.
func ensureSiteRegistered(siteID string) {
	cmd := exec.Command("kubectl", "exec", "-n", "postgres", "statefulset/postgres", "--",
		"psql", "-U", "forge", "-d", "forge", "-c",
		fmt.Sprintf("UPDATE site SET status = 'Registered' WHERE id = '%s' AND status != 'Registered'", siteID))
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Failed to ensure site is registered: %s", string(output))
	_, _ = fmt.Fprintf(GinkgoWriter, "Ensured site %s is Registered\n", siteID)
}

// setupInfrastructureViaAPI uses the existing local-dev-site and creates
// Tenant -> IP Block -> Allocation -> VPC -> Subnet.
// Returns siteID for use in tests. Instance creation requires instance types
// which are not available in the mock-core setup.
func setupInfrastructureViaAPI(token, orgName, prefix string) (siteID, tenantID string) {
	apiBase := fmt.Sprintf("/v2/org/%s/carbide", orgName)

	// Use the existing site (has a connected site-agent for Temporal workflows)
	siteID = getExistingSiteID(token, orgName)
	ensureSiteRegistered(siteID)

	// Get or create Tenant (idempotent)
	carbideAPIRequest("POST", apiBase+"/tenant", token, map[string]interface{}{"org": orgName})
	currentTenant, tStatus := carbideAPIRequest("GET", apiBase+"/tenant/current", token, nil)
	Expect(tStatus).To(Equal(http.StatusOK), "Failed to get current tenant: %v", currentTenant)
	tenantID = currentTenant["id"].(string)
	_, _ = fmt.Fprintf(GinkgoWriter, "Tenant ID: %s\n", tenantID)

	// Create IP Block
	ipBlockResult, status := carbideAPIRequest("POST", apiBase+"/ipblock", token, map[string]interface{}{
		"name": prefix + "-ipblock", "siteId": siteID,
		"prefix": "10.0.0.0", "prefixLength": 16, "protocolVersion": "IPv4", "routingType": "Public",
	})
	Expect(status).To(Equal(http.StatusCreated), "Failed to create IP block: %v", ipBlockResult)
	ipBlockID := ipBlockResult["id"].(string)

	// Create Allocation (links Tenant to Site with IP Block access)
	// The allocation creates a child IP block owned by the tenant — subnets must use that child ID.
	allocResult, status := carbideAPIRequest("POST", apiBase+"/allocation", token, map[string]interface{}{
		"name":     prefix + "-allocation",
		"tenantId": tenantID,
		"siteId":   siteID,
		"allocationConstraints": []map[string]interface{}{
			{"resourceType": "IPBlock", "resourceTypeId": ipBlockID, "constraintType": "OnDemand", "constraintValue": 24},
		},
	})
	Expect(status).To(Equal(http.StatusCreated), "Failed to create allocation: %v", allocResult)

	// Extract the child IP block ID from the allocation response
	constraints := allocResult["allocationConstraints"].([]interface{})
	firstConstraint := constraints[0].(map[string]interface{})
	childIPBlockID := firstConstraint["derivedResourceId"].(string)
	_, _ = fmt.Fprintf(GinkgoWriter, "Child IP Block ID: %s\n", childIPBlockID)

	// Create VPC (validates the full Temporal workflow path through the site-agent).
	// The forge→site-agent gRPC connection may not be established immediately after
	// the stack starts, so retry with backoff until the connection is ready.
	var vpcResult map[string]interface{}
	var vpcStatus int
	const maxRetries = 12
	for i := range maxRetries {
		vpcResult, vpcStatus = carbideAPIRequest("POST", apiBase+"/vpc", token, map[string]interface{}{
			"name": fmt.Sprintf("%s-vpc-%d", prefix, i), "siteId": siteID,
		})
		if vpcStatus == http.StatusCreated {
			break
		}
		_, _ = fmt.Fprintf(GinkgoWriter,
			"VPC creation attempt %d/%d failed (status %d), waiting for gRPC readiness...\n",
			i+1, maxRetries, vpcStatus)
		time.Sleep(10 * time.Second)
	}
	Expect(vpcStatus).To(Equal(http.StatusCreated), "Failed to create VPC after %d attempts: %v", maxRetries, vpcResult)
	vpcID := vpcResult["id"].(string)

	// Create Subnet (uses child IP block, not the parent)
	subnetResult, status := carbideAPIRequest("POST", apiBase+"/subnet", token, map[string]interface{}{
		"name": prefix + "-subnet", "vpcId": vpcID, "ipv4BlockId": childIPBlockID, "prefixLength": 26,
	})
	Expect(status).To(Equal(http.StatusCreated), "Failed to create subnet: %v", subnetResult)
	subnetID := subnetResult["id"].(string)

	_, _ = fmt.Fprintf(GinkgoWriter, "Infrastructure: site=%s vpc=%s subnet=%s\n",
		siteID, vpcID, subnetID)
	return
}

// cleanupInfrastructureViaAPI cleans up infrastructure created by setupInfrastructureViaAPI.
func cleanupInfrastructureViaAPI(token, orgName, siteID string) {
	// VPCs, subnets, and allocations are cleaned up automatically when the Kind cluster is reset.
	// We don't delete the site since it's the shared local-dev-site.
}
