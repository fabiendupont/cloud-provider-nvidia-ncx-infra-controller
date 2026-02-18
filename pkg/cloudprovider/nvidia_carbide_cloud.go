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
	"io"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	bmm "github.com/nvidia/bare-metal-manager-rest/sdk/standard"
)

const (
	// ProviderName is the name of the NVIDIA Carbide cloud provider
	ProviderName = "nvidia-carbide"

	// Default environment variable names for configuration
	EnvEndpoint = "NVIDIA_CARBIDE_ENDPOINT"
	EnvOrgName  = "NVIDIA_CARBIDE_ORG_NAME"
	EnvToken    = "NVIDIA_CARBIDE_TOKEN"
	EnvSiteID   = "NVIDIA_CARBIDE_SITE_ID"
	EnvTenantID = "NVIDIA_CARBIDE_TENANT_ID"
)

// NvidiaCarbideClientInterface defines the methods we need from the NVIDIA Carbide REST client
type NvidiaCarbideClientInterface interface {
	GetInstance(ctx context.Context, org string, instanceId string) (*bmm.Instance, *http.Response, error)
}

// carbideClient wraps the SDK APIClient and injects auth context
type carbideClient struct {
	client *bmm.APIClient
	token  string
}

func (c *carbideClient) authCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, bmm.ContextAccessToken, c.token)
}

func (c *carbideClient) GetInstance(
	ctx context.Context, org, instanceId string,
) (*bmm.Instance, *http.Response, error) {
	return c.client.InstanceAPI.GetInstance(c.authCtx(ctx), org, instanceId).Execute()
}

// NvidiaCarbideCloud implements the Kubernetes cloud provider interface for NVIDIA Carbide
type NvidiaCarbideCloud struct {
	nvidiaCarbideClient NvidiaCarbideClientInterface
	orgName             string
	siteID              string
	tenantID            string
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		return NewNvidiaCarbideCloud(config)
	})
}

// NewNvidiaCarbideCloud creates a new NVIDIA Carbide cloud provider instance
func NewNvidiaCarbideCloud(config io.Reader) (cloudprovider.Interface, error) {
	// Parse configuration
	cfg, err := parseConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create NVIDIA Carbide API client
	sdkCfg := bmm.NewConfiguration()
	sdkCfg.Servers = bmm.ServerConfigurations{
		{URL: cfg.Endpoint},
	}
	nvidiaCarbideClient := &carbideClient{
		client: bmm.NewAPIClient(sdkCfg),
		token:  cfg.Token,
	}

	klog.Infof("NVIDIA Carbide cloud provider initialized for org=%s, site=%s", cfg.OrgName, cfg.SiteID)

	return &NvidiaCarbideCloud{
		nvidiaCarbideClient: nvidiaCarbideClient,
		orgName:             cfg.OrgName,
		siteID:              cfg.SiteID,
		tenantID:            cfg.TenantID,
	}, nil
}

// NewNvidiaCarbideCloudWithClient creates a new NVIDIA Carbide cloud provider with injected client (for testing)
func NewNvidiaCarbideCloudWithClient(
	client NvidiaCarbideClientInterface, orgName, siteID, tenantID string,
) cloudprovider.Interface {
	return &NvidiaCarbideCloud{
		nvidiaCarbideClient: client,
		orgName:             orgName,
		siteID:              siteID,
		tenantID:            tenantID,
	}
}

// Initialize provides the cloud provider with the client builder and may be called multiple times
func (c *NvidiaCarbideCloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	klog.Info("Initializing NVIDIA Carbide cloud provider")
}

// LoadBalancer returns a LoadBalancer interface
// NVIDIA Carbide does not currently support load balancers
func (c *NvidiaCarbideCloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return nil, false
}

// Instances returns an Instances interface (deprecated, use InstancesV2)
func (c *NvidiaCarbideCloud) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

// InstancesV2 returns an InstancesV2 interface for node lifecycle management
func (c *NvidiaCarbideCloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return c, true
}

// Zones returns a Zones interface
func (c *NvidiaCarbideCloud) Zones() (cloudprovider.Zones, bool) {
	return c, true
}

// Clusters returns a Clusters interface (deprecated)
func (c *NvidiaCarbideCloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

// Routes returns a Routes interface
// NVIDIA Carbide does not currently support routes
func (c *NvidiaCarbideCloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

// ProviderName returns the cloud provider name
func (c *NvidiaCarbideCloud) ProviderName() string {
	return ProviderName
}

// HasClusterID returns true if the cluster has a cluster ID
func (c *NvidiaCarbideCloud) HasClusterID() bool {
	return true
}

// Config holds the NVIDIA Carbide cloud provider configuration
type Config struct {
	// Endpoint is the NVIDIA Carbide API endpoint URL
	Endpoint string `yaml:"endpoint"`

	// OrgName is the NVIDIA Carbide organization name
	OrgName string `yaml:"orgName"`

	// Token is the NVIDIA Carbide API authentication token
	Token string `yaml:"token"`

	// SiteID is the NVIDIA Carbide site UUID
	SiteID string `yaml:"siteId"`

	// TenantID is the NVIDIA Carbide tenant UUID
	TenantID string `yaml:"tenantId"`
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	if c.OrgName == "" {
		return fmt.Errorf("orgName is required")
	}
	if c.Token == "" {
		return fmt.Errorf("token is required")
	}
	if c.SiteID == "" {
		return fmt.Errorf("siteId is required")
	}
	if c.TenantID == "" {
		return fmt.Errorf("tenantId is required")
	}
	return nil
}

// parseConfig parses the cloud provider configuration from YAML or environment variables
func parseConfig(config io.Reader) (*Config, error) {
	cfg := &Config{}

	// First, try to parse from config file (YAML)
	if config != nil {
		data, err := io.ReadAll(config)
		if err != nil {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}

		if len(data) > 0 {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("failed to unmarshal YAML config: %w", err)
			}
			klog.V(4).Info("Loaded configuration from YAML file")
		}
	}

	// Override with environment variables if present
	if endpoint := os.Getenv(EnvEndpoint); endpoint != "" {
		cfg.Endpoint = endpoint
		klog.V(4).Infof("Using endpoint from environment: %s", endpoint)
	}
	if orgName := os.Getenv(EnvOrgName); orgName != "" {
		cfg.OrgName = orgName
		klog.V(4).Infof("Using orgName from environment: %s", orgName)
	}
	if token := os.Getenv(EnvToken); token != "" {
		cfg.Token = token
		klog.V(4).Info("Using token from environment")
	}
	if siteID := os.Getenv(EnvSiteID); siteID != "" {
		cfg.SiteID = siteID
		klog.V(4).Infof("Using siteID from environment: %s", siteID)
	}
	if tenantID := os.Getenv(EnvTenantID); tenantID != "" {
		cfg.TenantID = tenantID
		klog.V(4).Infof("Using tenantID from environment: %s", tenantID)
	}

	return cfg, nil
}
