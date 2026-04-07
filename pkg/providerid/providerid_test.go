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

package providerid

import (
	"testing"

	"github.com/google/uuid"
)

func TestParseProviderID_4Segment(t *testing.T) {
	instanceID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	tests := []struct {
		name       string
		input      string
		wantOrg    string
		wantTenant string
		wantSite   string
		wantID     uuid.UUID
	}{
		{
			name:       "standard 4-segment format",
			input:      "nico://myorg/mytenant/mysite/11111111-2222-3333-4444-555555555555",
			wantOrg:    "myorg",
			wantTenant: "mytenant",
			wantSite:   "mysite",
			wantID:     instanceID,
		},
		{
			name:       "UUIDs as org/tenant/site names",
			input:      "nico://aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee/ffffffff-0000-1111-2222-333333333333/44444444-5555-6666-7777-888888888888/11111111-2222-3333-4444-555555555555",
			wantOrg:    "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			wantTenant: "ffffffff-0000-1111-2222-333333333333",
			wantSite:   "44444444-5555-6666-7777-888888888888",
			wantID:     instanceID,
		},
		{
			name:       "hyphenated segment names",
			input:      "nico://my-org/my-tenant/us-east-1/11111111-2222-3333-4444-555555555555",
			wantOrg:    "my-org",
			wantTenant: "my-tenant",
			wantSite:   "us-east-1",
			wantID:     instanceID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseProviderID(tt.input)
			if err != nil {
				t.Fatalf("ParseProviderID(%q) unexpected error: %v", tt.input, err)
			}
			if parsed.OrgName != tt.wantOrg {
				t.Errorf("OrgName = %q, want %q", parsed.OrgName, tt.wantOrg)
			}
			if parsed.TenantName != tt.wantTenant {
				t.Errorf("TenantName = %q, want %q", parsed.TenantName, tt.wantTenant)
			}
			if parsed.SiteName != tt.wantSite {
				t.Errorf("SiteName = %q, want %q", parsed.SiteName, tt.wantSite)
			}
			if parsed.InstanceID != tt.wantID {
				t.Errorf("InstanceID = %s, want %s", parsed.InstanceID, tt.wantID)
			}
		})
	}
}

func TestParseProviderID_3SegmentLegacy(t *testing.T) {
	instanceID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	tests := []struct {
		name     string
		input    string
		wantOrg  string
		wantSite string
		wantID   uuid.UUID
	}{
		{
			name:     "legacy 3-segment format",
			input:    "nico://myorg/mysite/11111111-2222-3333-4444-555555555555",
			wantOrg:  "myorg",
			wantSite: "mysite",
			wantID:   instanceID,
		},
		{
			name:     "legacy with UUID site name",
			input:    "nico://myorg/44444444-5555-6666-7777-888888888888/11111111-2222-3333-4444-555555555555",
			wantOrg:  "myorg",
			wantSite: "44444444-5555-6666-7777-888888888888",
			wantID:   instanceID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseProviderID(tt.input)
			if err != nil {
				t.Fatalf("ParseProviderID(%q) unexpected error: %v", tt.input, err)
			}
			if parsed.OrgName != tt.wantOrg {
				t.Errorf("OrgName = %q, want %q", parsed.OrgName, tt.wantOrg)
			}
			if parsed.TenantName != "" {
				t.Errorf("TenantName = %q, want empty for legacy format", parsed.TenantName)
			}
			if parsed.SiteName != tt.wantSite {
				t.Errorf("SiteName = %q, want %q", parsed.SiteName, tt.wantSite)
			}
			if parsed.InstanceID != tt.wantID {
				t.Errorf("InstanceID = %s, want %s", parsed.InstanceID, tt.wantID)
			}
		})
	}
}

func TestParseProviderID_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"no prefix", "myorg/mytenant/mysite/11111111-2222-3333-4444-555555555555"},
		{"wrong prefix ncx-infra", "ncx-infra://myorg/mytenant/mysite/11111111-2222-3333-4444-555555555555"},
		{"wrong prefix nvidia-carbide", "nvidia-carbide://myorg/mytenant/mysite/11111111-2222-3333-4444-555555555555"},
		{"too few segments (1)", "nico://org"},
		{"too few segments (2)", "nico://org/site"},
		{"too many segments (5)", "nico://a/b/c/d/11111111-2222-3333-4444-555555555555"},
		{"invalid uuid in 3-segment", "nico://org/site/not-a-uuid"},
		{"invalid uuid in 4-segment", "nico://org/tenant/site/not-a-uuid"},
		{"empty segments", "nico:////11111111-2222-3333-4444-555555555555"},
		{"trailing slash", "nico://org/tenant/site/11111111-2222-3333-4444-555555555555/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseProviderID(tt.input)
			if err == nil {
				t.Errorf("ParseProviderID(%q) expected error, got nil", tt.input)
			}
		})
	}
}

func TestNewProviderID_String(t *testing.T) {
	instanceID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	pid := NewProviderID("myorg", "mytenant", "mysite", instanceID)

	want := "nico://myorg/mytenant/mysite/11111111-2222-3333-4444-555555555555"
	if got := pid.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	instanceID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	original := NewProviderID("myorg", "mytenant", "mysite", instanceID)

	parsed, err := ParseProviderID(original.String())
	if err != nil {
		t.Fatalf("round-trip ParseProviderID failed: %v", err)
	}

	if parsed.OrgName != original.OrgName {
		t.Errorf("OrgName mismatch: %q vs %q", parsed.OrgName, original.OrgName)
	}
	if parsed.TenantName != original.TenantName {
		t.Errorf("TenantName mismatch: %q vs %q", parsed.TenantName, original.TenantName)
	}
	if parsed.SiteName != original.SiteName {
		t.Errorf("SiteName mismatch: %q vs %q", parsed.SiteName, original.SiteName)
	}
	if parsed.InstanceID != original.InstanceID {
		t.Errorf("InstanceID mismatch: %s vs %s", parsed.InstanceID, original.InstanceID)
	}
}
