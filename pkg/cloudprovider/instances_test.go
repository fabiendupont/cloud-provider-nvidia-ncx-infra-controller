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

	bmm "github.com/nvidia/bare-metal-manager-rest/sdk/standard"

	"github.com/fabiendupont/cloud-provider-nvidia-carbide/pkg/providerid"
)

// mockClient is a minimal mock for testing
type mockNvidiaCarbideClient struct {
	getInstance func(ctx context.Context, org string, instanceId string) (*bmm.Instance, *http.Response, error)
}

func (m *mockNvidiaCarbideClient) GetInstance(
	ctx context.Context, org string, instanceId string,
) (*bmm.Instance, *http.Response, error) {
	if m.getInstance != nil {
		return m.getInstance(ctx, org, instanceId)
	}
	return nil, nil, nil
}

func TestInstanceExists(t *testing.T) {
	instanceID := uuid.New()
	pid := providerid.NewProviderID("test-org", "test-tenant", "test-site", instanceID)

	tests := []struct {
		name       string
		node       *v1.Node
		mockClient *mockNvidiaCarbideClient
		want       bool
		wantErr    bool
	}{
		{
			name: "instance exists",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					ProviderID: pid.String(),
				},
			},
			mockClient: &mockNvidiaCarbideClient{
				getInstance: func(ctx context.Context, org string, instanceId string) (*bmm.Instance, *http.Response, error) {
					id := instanceID.String()
					return &bmm.Instance{
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
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					ProviderID: pid.String(),
				},
			},
			mockClient: &mockNvidiaCarbideClient{
				getInstance: func(ctx context.Context, org string, instanceId string) (*bmm.Instance, *http.Response, error) {
					return nil, &http.Response{StatusCode: 404}, fmt.Errorf("not found")
				},
			},
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloud := &NvidiaCarbideCloud{
				nvidiaCarbideClient: tt.mockClient,
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

func TestParseProviderID(t *testing.T) {
	instanceID := uuid.New()
	pid := providerid.NewProviderID("myorg", "mytenant", "mysite", instanceID)

	parsed, err := parseProviderID(pid.String())
	if err != nil {
		t.Fatalf("parseProviderID() failed: %v", err)
	}

	if parsed != instanceID {
		t.Errorf("Expected instance ID %s, got %s", instanceID, parsed)
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
		{"missing parts", "nvidia-carbide://org/site", true},
		{"invalid uuid", "nvidia-carbide://org/site/not-a-uuid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseProviderID(tt.providerID)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseProviderID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func ptr[T any](v T) *T {
	return &v
}
