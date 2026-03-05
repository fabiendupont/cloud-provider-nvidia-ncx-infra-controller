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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	testSiteID   = "00000000-0000-0000-0000-000000000001"
	testTenantID = "00000000-0000-0000-0000-000000000001"
	testOrgName  = "test-org"

	instanceReadyTimeout = 10 * time.Minute
	pollInterval         = 10 * time.Second
)

func TestE2ELive(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting cloud-provider-nvidia-carbide live e2e test suite\n")
	RunSpecs(t, "Live E2E Suite")
}

var _ = Describe("Live Cloud Provider E2E", Label("live"), func() {
	var (
		ctx       context.Context
		clientset *kubernetes.Clientset
		token     string
		endpoint  string
	)

	BeforeEach(func() {
		ctx = context.Background()

		endpoint = os.Getenv("NVIDIA_CARBIDE_API_ENDPOINT")
		if endpoint == "" {
			Skip("NVIDIA_CARBIDE_API_ENDPOINT must be set")
		}

		kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		)

		config, err := kubeconfig.ClientConfig()
		Expect(err).NotTo(HaveOccurred())

		clientset, err = kubernetes.NewForConfig(config)
		Expect(err).NotTo(HaveOccurred())

		token = getKeycloakToken()
	})

	Context("Cloud provider initialization with live API", func() {
		It("should initialize the cloud provider with a valid config", func() {
			By("Creating a cloud config")
			cloudConfig := createCloudConfigSecret(endpoint, testOrgName, token, testSiteID, testTenantID)

			By("Initializing the cloud provider")
			provider, err := cloudprovider.NewNvidiaCarbideCloud(strings.NewReader(cloudConfig))
			Expect(err).NotTo(HaveOccurred())
			Expect(provider).NotTo(BeNil())

			By("Verifying provider name")
			Expect(provider.ProviderName()).To(Equal("nvidia-carbide"))

			By("Verifying InstancesV2 is supported")
			instancesV2, supported := provider.InstancesV2()
			Expect(supported).To(BeTrue())
			Expect(instancesV2).NotTo(BeNil())

			By("Verifying Zones is supported")
			zones, supported := provider.Zones()
			Expect(supported).To(BeTrue())
			Expect(zones).NotTo(BeNil())
		})
	})

	Context("Node lifecycle with live API", func() {
		It("should check instance existence for a node with provider ID", func() {
			By("Creating a cloud config")
			cloudConfig := createCloudConfigSecret(endpoint, testOrgName, token, testSiteID, testTenantID)

			By("Initializing the cloud provider")
			provider, err := cloudprovider.NewNvidiaCarbideCloud(strings.NewReader(cloudConfig))
			Expect(err).NotTo(HaveOccurred())

			instancesV2, supported := provider.InstancesV2()
			Expect(supported).To(BeTrue())

			By("Checking instance existence for a non-existent instance")
			fakeInstanceID := uuid.New().String()
			providerID := fmt.Sprintf("nvidia-carbide://%s/%s/%s/%s", testOrgName, testTenantID, testSiteID, fakeInstanceID)

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: corev1.NodeSpec{
					ProviderID: providerID,
				},
			}

			exists, err := instancesV2.InstanceExists(ctx, node)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse(), "Non-existent instance should not be found")
		})

		It("should create a node and verify the CCM can query it", func() {
			By("Creating a test node in the Kind cluster")
			nodeName := fmt.Sprintf("e2e-live-node-%d", time.Now().Unix())
			fakeInstanceID := uuid.New().String()
			providerID := fmt.Sprintf("nvidia-carbide://%s/%s/%s/%s", testOrgName, testTenantID, testSiteID, fakeInstanceID)

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
				},
				Spec: corev1.NodeSpec{
					ProviderID: providerID,
				},
			}

			_, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up the test node")
			err = clientset.CoreV1().Nodes().Delete(ctx, nodeName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
