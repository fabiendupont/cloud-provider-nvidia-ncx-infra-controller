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
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nico "github.com/NVIDIA/ncx-infra-controller-rest/sdk/standard"

	"github.com/fabiendupont/cloud-provider-nvidia-ncx-infra-controller/pkg/providerid"
)

// mockNicoClient is a mock for testing
type mockNicoClient struct {
	getInstance     func(ctx context.Context, org string, instanceId string) (*nico.Instance, *http.Response, error)
	getSite         func(ctx context.Context, org string, siteId string) (*nico.Site, *http.Response, error)
	getInstanceType func(ctx context.Context, org string, instanceTypeId string) (*nico.InstanceType, *http.Response, error)
	getMachine      func(ctx context.Context, org string, machineId string) (*nico.Machine, *http.Response, error)
}

func (m *mockNicoClient) GetInstance(
	ctx context.Context, org string, instanceId string,
) (*nico.Instance, *http.Response, error) {
	if m.getInstance != nil {
		return m.getInstance(ctx, org, instanceId)
	}
	return nil, &http.Response{StatusCode: 404}, fmt.Errorf("not found")
}

func (m *mockNicoClient) GetSite(
	ctx context.Context, org string, siteId string,
) (*nico.Site, *http.Response, error) {
	if m.getSite != nil {
		return m.getSite(ctx, org, siteId)
	}
	return nil, &http.Response{StatusCode: 404}, fmt.Errorf("not found")
}

func (m *mockNicoClient) GetInstanceType(
	ctx context.Context, org string, instanceTypeId string,
) (*nico.InstanceType, *http.Response, error) {
	if m.getInstanceType != nil {
		return m.getInstanceType(ctx, org, instanceTypeId)
	}
	return nil, &http.Response{StatusCode: 404}, fmt.Errorf("not found")
}

func (m *mockNicoClient) GetMachine(
	ctx context.Context, org string, machineId string,
) (*nico.Machine, *http.Response, error) {
	if m.getMachine != nil {
		return m.getMachine(ctx, org, machineId)
	}
	return nil, &http.Response{StatusCode: 404}, fmt.Errorf("not found")
}

func TestInstanceExists(t *testing.T) {
	instanceID := uuid.New()
	pid := providerid.NewProviderID("test-org", "test-tenant", "test-site", instanceID)

	tests := []struct {
		name       string
		node       *v1.Node
		mockClient *mockNicoClient
		want       bool
		wantErr    bool
	}{
		{
			name: "instance exists",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
				Spec:       v1.NodeSpec{ProviderID: pid.String()},
			},
			mockClient: &mockNicoClient{
				getInstance: func(ctx context.Context, org string, instanceId string) (*nico.Instance, *http.Response, error) {
					id := instanceID.String()
					return &nico.Instance{
						Id:   &id,
						Name: ptr("test-instance"),
					}, &http.Response{StatusCode: 200}, nil
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "instance not found",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
				Spec:       v1.NodeSpec{ProviderID: pid.String()},
			},
			mockClient: &mockNicoClient{
				getInstance: func(ctx context.Context, org string, instanceId string) (*nico.Instance, *http.Response, error) {
					return nil, &http.Response{StatusCode: 404}, fmt.Errorf("not found")
				},
			},
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloud := &NicoCloud{
				nicoClient: tt.mockClient,
				orgName:             "test-org",
				siteID:              "test-site",
			}

			got, err := cloud.InstanceExists(context.Background(), tt.node)
			if (err != nil) != tt.wantErr {
				t.Errorf("InstanceExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("InstanceExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInstanceMetadata_InstanceType(t *testing.T) {
	instanceID := uuid.New()
	siteID := uuid.New().String()
	instanceTypeID := uuid.New().String()
	pid := providerid.NewProviderID("test-org", "test-tenant", siteID, instanceID)

	tests := []struct {
		name             string
		instanceTypeID   *string
		mockInstanceType func(ctx context.Context, org string, id string) (*nico.InstanceType, *http.Response, error)
		wantType         string
	}{
		{
			name:           "resolves instance type name",
			instanceTypeID: &instanceTypeID,
			mockInstanceType: func(ctx context.Context, org string, id string) (*nico.InstanceType, *http.Response, error) {
				return &nico.InstanceType{
					Id:   &instanceTypeID,
					Name: ptr("dgx-h100"),
				}, &http.Response{StatusCode: 200}, nil
			},
			wantType: "dgx-h100",
		},
		{
			name:           "falls back when instance type lookup fails",
			instanceTypeID: &instanceTypeID,
			mockInstanceType: func(ctx context.Context, org string, id string) (*nico.InstanceType, *http.Response, error) {
				return nil, &http.Response{StatusCode: 500}, fmt.Errorf("server error")
			},
			wantType: "nico-instance",
		},
		{
			name:           "falls back when no instance type ID",
			instanceTypeID: nil,
			wantType:       "nico-instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := instanceID.String()
			instance := &nico.Instance{
				Id:             &id,
				Name:           ptr("test-instance"),
				SiteId:         &siteID,
				InstanceTypeId: tt.instanceTypeID,
				Interfaces:     []nico.Interface{},
			}

			mock := &mockNicoClient{
				getInstance: func(ctx context.Context, org string, instanceId string) (*nico.Instance, *http.Response, error) {
					return instance, &http.Response{StatusCode: 200}, nil
				},
				getInstanceType: tt.mockInstanceType,
				getSite: func(ctx context.Context, org string, siteId string) (*nico.Site, *http.Response, error) {
					return nil, &http.Response{StatusCode: 404}, fmt.Errorf("not found")
				},
			}

			cloud := &NicoCloud{
				nicoClient: mock,
				orgName:             "test-org",
				siteID:              siteID,
			}

			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
				Spec:       v1.NodeSpec{ProviderID: pid.String()},
			}

			metadata, err := cloud.InstanceMetadata(context.Background(), node)
			if err != nil {
				t.Fatalf("InstanceMetadata() error = %v", err)
			}
			if metadata.InstanceType != tt.wantType {
				t.Errorf("InstanceType = %q, want %q", metadata.InstanceType, tt.wantType)
			}
		})
	}
}

func TestResolveZoneAndRegion(t *testing.T) {
	siteID := uuid.New().String()

	tests := []struct {
		name       string
		mockSite   func(ctx context.Context, org string, siteId string) (*nico.Site, *http.Response, error)
		wantZone   string
		wantRegion string
	}{
		{
			name: "site with full location",
			mockSite: func(ctx context.Context, org string, siteId string) (*nico.Site, *http.Response, error) {
				return &nico.Site{
					Id:   &siteID,
					Name: ptr("Santa Clara DC1"),
					Location: &nico.SiteLocation{
						Country: ptr("us"),
						State:   ptr("california"),
						City:    ptr("santa clara"),
					},
				}, &http.Response{StatusCode: 200}, nil
			},
			wantZone:   "us-california-santa-clara-dc1",
			wantRegion: "us-california",
		},
		{
			name: "site without location falls back to name",
			mockSite: func(ctx context.Context, org string, siteId string) (*nico.Site, *http.Response, error) {
				return &nico.Site{
					Id:   &siteID,
					Name: ptr("test-dc"),
				}, &http.Response{StatusCode: 200}, nil
			},
			wantZone:   "test-dc",
			wantRegion: "test-dc",
		},
		{
			name: "site lookup fails falls back to ID",
			mockSite: func(ctx context.Context, org string, siteId string) (*nico.Site, *http.Response, error) {
				return nil, &http.Response{StatusCode: 500}, fmt.Errorf("server error")
			},
			wantZone:   fmt.Sprintf("nico-zone-%s", siteID),
			wantRegion: fmt.Sprintf("nico-region-%s", siteID),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloud := &NicoCloud{
				nicoClient: &mockNicoClient{
					getSite: tt.mockSite,
				},
				orgName: "test-org",
				siteID:  siteID,
			}

			zone, region := cloud.resolveZoneAndRegion(context.Background(), siteID)
			if zone != tt.wantZone {
				t.Errorf("zone = %q, want %q", zone, tt.wantZone)
			}
			if region != tt.wantRegion {
				t.Errorf("region = %q, want %q", region, tt.wantRegion)
			}
		})
	}
}

func TestExtractNodeAddresses(t *testing.T) {
	tests := []struct {
		name       string
		interfaces []nico.Interface
		wantIPs    []v1.NodeAddress
	}{
		{
			name: "uses all IPs from non-physical interfaces",
			interfaces: []nico.Interface{
				{
					IsPhysical:  ptr(true),
					IpAddresses: []string{"10.0.0.1"},
				},
				{
					IsPhysical:  ptr(false),
					IpAddresses: []string{"192.168.1.10", "192.168.1.11"},
				},
			},
			wantIPs: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "192.168.1.10"},
				{Type: v1.NodeInternalIP, Address: "192.168.1.11"},
				{Type: v1.NodeHostName, Address: "test-node"},
			},
		},
		{
			name: "first interface without IsPhysical set",
			interfaces: []nico.Interface{
				{
					IpAddresses: []string{"10.0.0.5"},
				},
			},
			wantIPs: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.0.0.5"},
				{Type: v1.NodeHostName, Address: "test-node"},
			},
		},
		{
			name:       "no interfaces",
			interfaces: []nico.Interface{},
			wantIPs: []v1.NodeAddress{
				{Type: v1.NodeHostName, Address: "test-node"},
			},
		},
		{
			name: "skips non-physical interface with empty IPs",
			interfaces: []nico.Interface{
				{
					IsPhysical:  ptr(true),
					IpAddresses: []string{"10.0.0.1"},
				},
				{
					IsPhysical:  ptr(false),
					IpAddresses: []string{},
				},
				{
					IsPhysical:  ptr(false),
					IpAddresses: []string{"192.168.2.20"},
				},
			},
			wantIPs: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "192.168.2.20"},
				{Type: v1.NodeHostName, Address: "test-node"},
			},
		},
		{
			name: "multiple non-physical interfaces all contribute IPs",
			interfaces: []nico.Interface{
				{
					IsPhysical:  ptr(false),
					IpAddresses: []string{"192.168.1.10"},
				},
				{
					IsPhysical:  ptr(true),
					IpAddresses: []string{"10.0.0.1"},
				},
				{
					IsPhysical:  ptr(false),
					IpAddresses: []string{"192.168.2.20"},
				},
			},
			wantIPs: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "192.168.1.10"},
				{Type: v1.NodeInternalIP, Address: "192.168.2.20"},
				{Type: v1.NodeHostName, Address: "test-node"},
			},
		},
		{
			name: "all physical interfaces skipped",
			interfaces: []nico.Interface{
				{
					IsPhysical:  ptr(true),
					IpAddresses: []string{"10.0.0.1"},
				},
			},
			wantIPs: []v1.NodeAddress{
				{Type: v1.NodeHostName, Address: "test-node"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloud := &NicoCloud{}
			instance := &nico.Instance{
				Interfaces: tt.interfaces,
			}

			got := cloud.extractNodeAddresses(instance, "test-node")
			if len(got) != len(tt.wantIPs) {
				t.Fatalf("got %d addresses, want %d: %+v", len(got), len(tt.wantIPs), got)
			}
			for i, addr := range got {
				if addr.Type != tt.wantIPs[i].Type || addr.Address != tt.wantIPs[i].Address {
					t.Errorf("address[%d] = %+v, want %+v", i, addr, tt.wantIPs[i])
				}
			}
		})
	}
}

func TestParseProviderID(t *testing.T) {
	instanceID := uuid.New()
	pid := providerid.NewProviderID("myorg", "mytenant", "mysite", instanceID)

	parsed, err := providerid.ParseProviderID(pid.String())
	if err != nil {
		t.Fatalf("ParseProviderID() failed: %v", err)
	}

	if parsed.InstanceID != instanceID {
		t.Errorf("Expected instance ID %s, got %s", instanceID, parsed.InstanceID)
	}
}

func TestParseProviderID_Invalid(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		wantErr    bool
	}{
		{"empty", "", true},
		{"invalid format", "invalid-format", true},
		{"missing parts", "nico://org/site", true},
		{"invalid uuid", "nico://org/site/not-a-uuid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := providerid.ParseProviderID(tt.providerID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseProviderID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMachineHealthLabels(t *testing.T) {
	tests := []struct {
		name        string
		machineID   *string
		mockMachine func(ctx context.Context, org string, machineId string) (*nico.Machine, *http.Response, error)
		wantHealthy string
		wantNil     bool
	}{
		{
			name:      "no machine ID",
			machineID: nil,
			wantNil:   true,
		},
		{
			name:      "healthy machine",
			machineID: ptr("machine-1"),
			mockMachine: func(ctx context.Context, org string, machineId string) (*nico.Machine, *http.Response, error) {
				return &nico.Machine{
					Health: &nico.MachineHealth{
						Alerts: []nico.MachineHealthProbeAlert{},
					},
				}, &http.Response{StatusCode: 200}, nil
			},
			wantHealthy: "true",
		},
		{
			name:      "machine with alerts",
			machineID: ptr("machine-2"),
			mockMachine: func(ctx context.Context, org string, machineId string) (*nico.Machine, *http.Response, error) {
				return &nico.Machine{
					Health: &nico.MachineHealth{
						Alerts: []nico.MachineHealthProbeAlert{
							{Id: ptr("alert-1"), Message: ptr("PSU failure")},
							{Id: ptr("alert-2"), Message: ptr("ECC error")},
						},
					},
				}, &http.Response{StatusCode: 200}, nil
			},
			wantHealthy: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloud := &NicoCloud{
				nicoClient: &mockNicoClient{
					getMachine: tt.mockMachine,
				},
				orgName: "test-org",
			}

			instance := &nico.Instance{}
			if tt.machineID != nil {
				instance.MachineId = *nico.NewNullableString(tt.machineID)
			}

			labels := cloud.machineHealthLabels(context.Background(), instance)
			if tt.wantNil {
				if labels != nil {
					t.Errorf("expected nil labels, got %v", labels)
				}
				return
			}

			if labels[LabelHealthy] != tt.wantHealthy {
				t.Errorf("healthy = %q, want %q", labels[LabelHealthy], tt.wantHealthy)
			}

			if tt.wantHealthy == "false" {
				if labels[LabelHealthAlertCount] != "2" {
					t.Errorf("alert count = %q, want %q", labels[LabelHealthAlertCount], "2")
				}
			}
		})
	}
}

func TestGetZoneByProviderID(t *testing.T) {
	siteID := uuid.New().String()
	instanceID := uuid.New()
	pid := providerid.NewProviderID("test-org", "test-tenant", siteID, instanceID)

	cloud := &NicoCloud{
		nicoClient: &mockNicoClient{
			getSite: func(ctx context.Context, org string, siteId string) (*nico.Site, *http.Response, error) {
				return &nico.Site{
					Id:   &siteID,
					Name: ptr("DC1"),
					Location: &nico.SiteLocation{
						Country: ptr("us"),
						State:   ptr("ca"),
					},
				}, &http.Response{StatusCode: 200}, nil
			},
		},
		orgName: "test-org",
		siteID:  siteID,
	}

	zone, err := cloud.GetZoneByProviderID(context.Background(), pid.String())
	if err != nil {
		t.Fatalf("GetZoneByProviderID() error = %v", err)
	}

	if zone.FailureDomain != "us-ca-dc1" {
		t.Errorf("FailureDomain = %q, want %q", zone.FailureDomain, "us-ca-dc1")
	}
	if zone.Region != "us-ca" {
		t.Errorf("Region = %q, want %q", zone.Region, "us-ca")
	}
}

func ptr[T any](v T) *T {
	return &v
}
