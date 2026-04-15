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

package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"

	nico "github.com/NVIDIA/ncx-infra-controller-rest/sdk/standard"
	nicoprovider "github.com/fabiendupont/cloud-provider-nvidia-ncx-infra-controller/pkg/cloudprovider"
)

var (
	ctx        context.Context
	cancel     context.CancelFunc
	cloud      cloudprovider.Interface
	mockClient *mockNicoClient
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cloud Provider Integration Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.TODO())

	// Create mock client
	mockClient = &mockNicoClient{}

	// Create cloud provider with mock client
	cloud = nicoprovider.NewNicoCloudWithClient(
		mockClient,
		"test-org",
		"8a880c71-fe4b-4e43-9e24-ebfcb8a84c5f",
		"b013708a-99f0-47b2-a630-cabb4ae1d3df",
	)
})

var _ = AfterSuite(func() {
	cancel()
})

// mockHTTPResponse creates a mock HTTP response with the given status code
func mockHTTPResponse(statusCode int) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader([]byte{})),
		Header:     make(http.Header),
	}
}

// mockNicoClient for testing
type mockNicoClient struct {
	getInstanceFunc     func(ctx context.Context, org string, instanceId string) (*nico.Instance, *http.Response, error)
	getSiteFunc         func(ctx context.Context, org string, siteId string) (*nico.Site, *http.Response, error)
	getInstanceTypeFunc func(
		ctx context.Context, org string, instanceTypeId string,
	) (*nico.InstanceType, *http.Response, error)
	getMachineFunc      func(ctx context.Context, org string, machineId string) (*nico.Machine, *http.Response, error)
	getCapabilitiesFunc func(ctx context.Context, org string) (*nico.CapabilitiesResponse, *http.Response, error)
	getHealthEventsFunc func(
		ctx context.Context, org string, machineID string,
	) ([]nico.FaultEvent, *http.Response, error)
	ingestHealthEventFunc func(
		ctx context.Context, org string, event nico.FaultIngestionRequest,
	) (*nico.FaultEvent, *http.Response, error)
}

func (m *mockNicoClient) GetInstance(
	ctx context.Context, org string, instanceId string,
) (*nico.Instance, *http.Response, error) {
	if m.getInstanceFunc != nil {
		return m.getInstanceFunc(ctx, org, instanceId)
	}

	// Default: return a running instance with IP addresses
	status := nico.InstanceStatus("Running")
	id := instanceId

	return &nico.Instance{
		Id:     &id,
		Name:   ptr("test-instance"),
		Status: &status,
		Interfaces: []nico.Interface{
			{
				IpAddresses: []string{"10.100.1.10"},
			},
		},
	}, mockHTTPResponse(200), nil
}

func (m *mockNicoClient) GetSite(
	ctx context.Context, org string, siteId string,
) (*nico.Site, *http.Response, error) {
	if m.getSiteFunc != nil {
		return m.getSiteFunc(ctx, org, siteId)
	}
	return nil, mockHTTPResponse(404), fmt.Errorf("not found")
}

func (m *mockNicoClient) GetInstanceType(
	ctx context.Context, org string, instanceTypeId string,
) (*nico.InstanceType, *http.Response, error) {
	if m.getInstanceTypeFunc != nil {
		return m.getInstanceTypeFunc(ctx, org, instanceTypeId)
	}
	return nil, mockHTTPResponse(404), fmt.Errorf("not found")
}

func (m *mockNicoClient) GetMachine(
	ctx context.Context, org string, machineId string,
) (*nico.Machine, *http.Response, error) {
	if m.getMachineFunc != nil {
		return m.getMachineFunc(ctx, org, machineId)
	}
	return nil, mockHTTPResponse(404), fmt.Errorf("not found")
}

func (m *mockNicoClient) GetCapabilities(
	ctx context.Context, org string,
) (*nico.CapabilitiesResponse, *http.Response, error) {
	if m.getCapabilitiesFunc != nil {
		return m.getCapabilitiesFunc(ctx, org)
	}
	return nil, mockHTTPResponse(404), fmt.Errorf("not found")
}

func (m *mockNicoClient) GetHealthEvents(
	ctx context.Context, org string, machineID string,
) ([]nico.FaultEvent, *http.Response, error) {
	if m.getHealthEventsFunc != nil {
		return m.getHealthEventsFunc(ctx, org, machineID)
	}
	return nil, mockHTTPResponse(404), fmt.Errorf("not found")
}

func (m *mockNicoClient) IngestHealthEvent(
	ctx context.Context, org string, event nico.FaultIngestionRequest,
) (*nico.FaultEvent, *http.Response, error) {
	if m.ingestHealthEventFunc != nil {
		return m.ingestHealthEventFunc(ctx, org, event)
	}
	return nil, mockHTTPResponse(202), nil
}

var _ = Describe("InstancesV2 Interface", func() {
	var (
		node       *corev1.Node
		instanceID uuid.UUID
	)

	BeforeEach(func() {
		instanceID = uuid.MustParse("12345678-1234-1234-1234-123456789abc")
		providerID := "nico://test-org/test-tenant/8a880c71-fe4b-4e43-9e24-ebfcb8a84c5f/" + instanceID.String()

		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
			},
			Spec: corev1.NodeSpec{
				ProviderID: providerID,
			},
		}
	})

	Describe("InstanceExists", func() {
		It("should return true for existing instance", func() {
			instancesV2, supported := cloud.InstancesV2()
			Expect(supported).To(BeTrue())

			exists, err := instancesV2.InstanceExists(ctx, node)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("should return false for non-existent instance", func() {
			// Override mock to return 404
			mockClient.getInstanceFunc = func(
				ctx context.Context, org string, instanceId string,
			) (*nico.Instance, *http.Response, error) {
				return nil, mockHTTPResponse(404), fmt.Errorf("not found")
			}

			instancesV2, _ := cloud.InstancesV2()
			exists, err := instancesV2.InstanceExists(ctx, node)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())

			// Reset mock
			mockClient.getInstanceFunc = nil
		})
	})

	Describe("InstanceShutdown", func() {
		It("should return false for running instance", func() {
			instancesV2, _ := cloud.InstancesV2()

			shutdown, err := instancesV2.InstanceShutdown(ctx, node)
			Expect(err).NotTo(HaveOccurred())
			Expect(shutdown).To(BeFalse())
		})

		It("should return true for terminated instance", func() {
			// Override mock to return terminated status
			mockClient.getInstanceFunc = func(
				ctx context.Context, org string, instanceId string,
			) (*nico.Instance, *http.Response, error) {
				status := nico.InstanceStatus("Terminated")
				return &nico.Instance{
					Id:     &instanceId,
					Status: &status,
				}, mockHTTPResponse(200), nil
			}

			instancesV2, _ := cloud.InstancesV2()
			shutdown, err := instancesV2.InstanceShutdown(ctx, node)
			Expect(err).NotTo(HaveOccurred())
			Expect(shutdown).To(BeTrue())

			// Reset mock
			mockClient.getInstanceFunc = nil
		})
	})

	Describe("InstanceMetadata", func() {
		It("should return metadata with addresses and zone", func() {
			instancesV2, _ := cloud.InstancesV2()

			metadata, err := instancesV2.InstanceMetadata(ctx, node)
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).NotTo(BeNil())
			Expect(metadata.ProviderID).To(Equal(node.Spec.ProviderID))
			Expect(metadata.NodeAddresses).NotTo(BeEmpty())
			Expect(metadata.Zone).To(ContainSubstring("nico-zone"))
			Expect(metadata.Region).To(ContainSubstring("nico-region"))
		})
	})
})

var _ = Describe("Zones Interface", func() {
	Describe("GetZone", func() {
		It("should return zone and region", func() {
			zones, supported := cloud.Zones()
			Expect(supported).To(BeTrue())

			zone, err := zones.GetZone(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.FailureDomain).To(ContainSubstring("nico-zone"))
			Expect(zone.Region).To(ContainSubstring("nico-region"))
		})
	})

	Describe("GetZoneByProviderID", func() {
		It("should return zone for provider ID", func() {
			zones, _ := cloud.Zones()
			providerID := "nico://test-org/test-tenant/" +
				"8a880c71-fe4b-4e43-9e24-ebfcb8a84c5f/" +
				"12345678-1234-1234-1234-123456789abc"

			zone, err := zones.GetZoneByProviderID(ctx, providerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.FailureDomain).To(ContainSubstring("nico-zone"))
			Expect(zone.Region).To(ContainSubstring("nico-region"))
		})
	})

	Describe("GetZoneByNodeName", func() {
		It("should return zone for node name", func() {
			zones, _ := cloud.Zones()
			nodeName := types.NodeName("test-node")

			zone, err := zones.GetZoneByNodeName(ctx, nodeName)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.FailureDomain).To(ContainSubstring("nico-zone"))
			Expect(zone.Region).To(ContainSubstring("nico-region"))
		})
	})
})

func ptr[T any](v T) *T {
	return &v
}
