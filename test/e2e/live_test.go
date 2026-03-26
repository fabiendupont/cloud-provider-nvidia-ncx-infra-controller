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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/fabiendupont/cloud-provider-nvidia-ncx-infra-controller/pkg/cloudprovider"
	"github.com/fabiendupont/cloud-provider-nvidia-ncx-infra-controller/pkg/providerid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testOrgName  = "test-org"
	testTenantID = "e2e-tenant"
	testSiteID   = "e2e-site"
	// Keycloak is disabled in the operator deployment, so any token works.
	testToken = "e2e-dummy-token"
)

func TestE2ELive(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter,
		"Starting cloud-provider-nico live e2e test suite\n")
	RunSpecs(t, "Live E2E Suite")
}

var _ = Describe("Live Cloud Provider E2E", Label("live"), func() {
	var (
		ctx      context.Context
		endpoint string
	)

	BeforeEach(func() {
		ctx = context.Background()

		endpoint = os.Getenv("NICO_API_ENDPOINT")
		if endpoint == "" {
			Skip("NICO_API_ENDPOINT must be set")
		}
	})

	Context("Cloud provider with live API", func() {
		It("should initialize the cloud provider with a valid config", func() {
			cloudConfig := createCloudConfigSecret(
				endpoint, testOrgName, testToken, testSiteID, testTenantID)

			provider, err := cloudprovider.NewNicoCloud(
				strings.NewReader(cloudConfig))
			Expect(err).NotTo(HaveOccurred())
			Expect(provider).NotTo(BeNil())
			Expect(provider.ProviderName()).To(Equal("nico"))

			instancesV2, supported := provider.InstancesV2()
			Expect(supported).To(BeTrue())
			Expect(instancesV2).NotTo(BeNil())

			zones, supported := provider.Zones()
			Expect(supported).To(BeTrue())
			Expect(zones).NotTo(BeNil())
		})

		It("should return false for InstanceExists with a non-existent instance",
			func() {
				cloudConfig := createCloudConfigSecret(
					endpoint, testOrgName, testToken, testSiteID, testTenantID)

				provider, err := cloudprovider.NewNicoCloud(
					strings.NewReader(cloudConfig))
				Expect(err).NotTo(HaveOccurred())

				instancesV2, _ := provider.InstancesV2()

				fakeInstanceID := uuid.New()
				pid := providerid.NewProviderID(
					testOrgName, testTenantID, testSiteID, fakeInstanceID)

				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node-not-exists"},
					Spec:       corev1.NodeSpec{ProviderID: pid.String()},
				}

				exists, err := instancesV2.InstanceExists(ctx, node)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse(),
					"Non-existent instance should not be found")
			})
	})
})
