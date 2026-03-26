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

package cloudprovider

import (
	"strings"
	"testing"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Endpoint: "https://api.nico.test",
				OrgName:  "test-org",
				Token:    "test-token",
				SiteID:   "test-site",
				TenantID: "test-tenant",
			},
			wantErr: false,
		},
		{
			name: "missing endpoint",
			config: &Config{
				OrgName:  "test-org",
				Token:    "test-token",
				SiteID:   "test-site",
				TenantID: "test-tenant",
			},
			wantErr: true,
		},
		{
			name: "missing orgName",
			config: &Config{
				Endpoint: "https://api.nico.test",
				Token:    "test-token",
				SiteID:   "test-site",
				TenantID: "test-tenant",
			},
			wantErr: true,
		},
		{
			name: "missing token",
			config: &Config{
				Endpoint: "https://api.nico.test",
				OrgName:  "test-org",
				SiteID:   "test-site",
				TenantID: "test-tenant",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseConfig_YAML(t *testing.T) {
	yamlConfig := `
endpoint: "https://api.nico.test"
orgName: "test-org"
token: "test-token"
siteId: "test-site"
tenantId: "test-tenant"
`

	config, err := parseConfig(strings.NewReader(yamlConfig))
	if err != nil {
		t.Fatalf("parseConfig() failed: %v", err)
	}

	if config.Endpoint != "https://api.nico.test" {
		t.Errorf("Expected endpoint=https://api.nico.test, got %s", config.Endpoint)
	}
	if config.OrgName != "test-org" {
		t.Errorf("Expected orgName=test-org, got %s", config.OrgName)
	}
	if config.Token != "test-token" {
		t.Errorf("Expected token=test-token, got %s", config.Token)
	}
}

func TestProviderName(t *testing.T) {
	cloud := &NicoCloud{}
	if cloud.ProviderName() != ProviderName {
		t.Errorf("Expected provider name %s, got %s", ProviderName, cloud.ProviderName())
	}
}

func TestNicoCloud_Interfaces(t *testing.T) {
	cloud := &NicoCloud{}

	// Test that InstancesV2 is supported
	if instances, supported := cloud.InstancesV2(); !supported || instances == nil {
		t.Error("InstancesV2 should be supported")
	}

	// Test that Zones is supported
	if zones, supported := cloud.Zones(); !supported || zones == nil {
		t.Error("Zones should be supported")
	}

	// Test that LoadBalancer is not supported
	if _, supported := cloud.LoadBalancer(); supported {
		t.Error("LoadBalancer should not be supported")
	}

	// Test that Routes is not supported
	if _, supported := cloud.Routes(); supported {
		t.Error("Routes should not be supported")
	}
}
