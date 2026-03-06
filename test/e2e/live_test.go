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
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/fabiendupont/cloud-provider-nvidia-carbide/pkg/cloudprovider"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testOrgName  = "test-org"
	pollInterval = 10 * time.Second
)

func TestE2ELive(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting cloud-provider-nvidia-carbide live e2e test suite\n")
	RunSpecs(t, "Live E2E Suite")
}

var _ = Describe("Live Cloud Provider E2E", Label("live"), func() {
	var (
		ctx      context.Context
		token    string
		endpoint string
	)

	BeforeEach(func() {
		ctx = context.Background()

		endpoint = os.Getenv("NVIDIA_CARBIDE_API_ENDPOINT")
		if endpoint == "" {
			Skip("NVIDIA_CARBIDE_API_ENDPOINT must be set")
		}

		token = getKeycloakToken()
	})

	Context("Cloud provider with real infrastructure", Ordered, func() {
		var siteID string

		BeforeAll(func() {
			prefix := fmt.Sprintf("e2e-ccm-%d", time.Now().Unix())
			siteID = setupInfrastructureViaAPI(token, testOrgName, prefix)
		})

		It("should initialize the cloud provider with a valid config", func() {
			cloudConfig := createCloudConfigSecret(endpoint, testOrgName, token, siteID, siteID)

			provider, err := cloudprovider.NewNvidiaCarbideCloud(strings.NewReader(cloudConfig))
			Expect(err).NotTo(HaveOccurred())
			Expect(provider).NotTo(BeNil())
			Expect(provider.ProviderName()).To(Equal("nvidia-carbide"))

			instancesV2, supported := provider.InstancesV2()
			Expect(supported).To(BeTrue())
			Expect(instancesV2).NotTo(BeNil())

			zones, supported := provider.Zones()
			Expect(supported).To(BeTrue())
			Expect(zones).NotTo(BeNil())
		})

		It("should return false for InstanceExists with a non-existent instance", func() {
			cloudConfig := createCloudConfigSecret(endpoint, testOrgName, token, siteID, siteID)

			provider, err := cloudprovider.NewNvidiaCarbideCloud(strings.NewReader(cloudConfig))
			Expect(err).NotTo(HaveOccurred())

			instancesV2, _ := provider.InstancesV2()

			fakeInstanceID := uuid.New().String()
			providerID := fmt.Sprintf("nvidia-carbide://%s/%s/%s/%s",
				testOrgName, siteID, siteID, fakeInstanceID)

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "test-node-not-exists"},
				Spec:       corev1.NodeSpec{ProviderID: providerID},
			}

			exists, err := instancesV2.InstanceExists(ctx, node)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse(), "Non-existent instance should not be found")
		})

		// Note: Tests for InstanceExists(true) and InstanceMetadata require a real
		// instance, which needs instance types from the mock-core. These will be
		// added once the mock-core is configured with test instance types.
	})
})
