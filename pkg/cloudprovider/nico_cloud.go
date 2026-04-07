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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	nico "github.com/NVIDIA/ncx-infra-controller-rest/sdk/standard"
)

const (
	// ProviderName is the name of the NICo cloud provider
	ProviderName = "nico"

	// Default environment variable names for configuration
	EnvEndpoint = "NICO_ENDPOINT"
	EnvOrgName  = "NICO_ORG_NAME"
	EnvToken    = "NICO_TOKEN"
	EnvSiteID   = "NICO_SITE_ID"
	EnvTenantID = "NICO_TENANT_ID"
	EnvAPIName  = "NICO_API_NAME"
)

// NicoClientInterface defines the methods we need from the NICo REST client
type NicoClientInterface interface {
	GetInstance(ctx context.Context, org string, instanceId string) (*nico.Instance, *http.Response, error)
	GetSite(ctx context.Context, org string, siteId string) (*nico.Site, *http.Response, error)
	GetInstanceType(ctx context.Context, org string, instanceTypeId string) (*nico.InstanceType, *http.Response, error)
	GetMachine(ctx context.Context, org string, machineId string) (*nico.Machine, *http.Response, error)
	GetCapabilities(ctx context.Context, org string) (*CapabilitiesResponse, *http.Response, error)
	GetHealthEvents(ctx context.Context, org string, machineID string) ([]FaultEvent, *http.Response, error)
}

// CapabilitiesResponse represents the response from the capabilities endpoint.
type CapabilitiesResponse struct {
	Features []string `json:"features"`
}

// FaultEvent represents a structured fault event from the health events API.
type FaultEvent struct {
	ID             string  `json:"id"`
	MachineID      string  `json:"machine_id"`
	Source         string  `json:"source"`
	Severity       string  `json:"severity"`
	Component      string  `json:"component"`
	Classification string  `json:"classification"`
	Message        string  `json:"message"`
	State          string  `json:"state"`
	DetectedAt     string  `json:"detected_at"`
}

// nicoAPIClient wraps the SDK APIClient and injects auth context
type nicoAPIClient struct {
	client *nico.APIClient
	token  string
}

func (c *nicoAPIClient) authCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, nico.ContextAccessToken, c.token)
}

func (c *nicoAPIClient) GetInstance(
	ctx context.Context, org, instanceId string,
) (*nico.Instance, *http.Response, error) {
	return c.client.InstanceAPI.GetInstance(c.authCtx(ctx), org, instanceId).Execute()
}

func (c *nicoAPIClient) GetSite(
	ctx context.Context, org, siteId string,
) (*nico.Site, *http.Response, error) {
	return c.client.SiteAPI.GetSite(c.authCtx(ctx), org, siteId).Execute()
}

func (c *nicoAPIClient) GetInstanceType(
	ctx context.Context, org, instanceTypeId string,
) (*nico.InstanceType, *http.Response, error) {
	return c.client.InstanceTypeAPI.GetInstanceType(c.authCtx(ctx), org, instanceTypeId).Execute()
}

func (c *nicoAPIClient) GetMachine(
	ctx context.Context, org, machineId string,
) (*nico.Machine, *http.Response, error) {
	return c.client.MachineAPI.GetMachine(c.authCtx(ctx), org, machineId).Execute()
}

func (c *nicoAPIClient) GetCapabilities(
	ctx context.Context, org string,
) (*CapabilitiesResponse, *http.Response, error) {
	baseURL := c.client.GetConfig().Servers[0].URL
	url := fmt.Sprintf("%s/v2/org/%s/carbide/capabilities", baseURL, org)

	req, err := http.NewRequestWithContext(c.authCtx(ctx), http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create capabilities request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.GetConfig().HTTPClient.Do(req)
	if err != nil {
		return nil, resp, fmt.Errorf("capabilities request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, resp, fmt.Errorf("capabilities endpoint returned status %d", resp.StatusCode)
	}

	var caps CapabilitiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&caps); err != nil {
		return nil, resp, fmt.Errorf("failed to decode capabilities response: %w", err)
	}
	return &caps, resp, nil
}

func (c *nicoAPIClient) GetHealthEvents(
	ctx context.Context, org, machineID string,
) ([]FaultEvent, *http.Response, error) {
	baseURL := c.client.GetConfig().Servers[0].URL
	url := fmt.Sprintf("%s/v2/org/%s/carbide/health/events?machine_id=%s&state=open",
		baseURL, org, machineID)

	req, err := http.NewRequestWithContext(c.authCtx(ctx), http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create health events request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.GetConfig().HTTPClient.Do(req)
	if err != nil {
		return nil, resp, fmt.Errorf("health events request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, resp, fmt.Errorf("health events endpoint returned status %d", resp.StatusCode)
	}

	var events []FaultEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, resp, fmt.Errorf("failed to decode health events response: %w", err)
	}
	return events, resp, nil
}

// NicoCloud implements the Kubernetes cloud provider interface for NICo
type NicoCloud struct {
	nicoClient NicoClientInterface
	orgName             string
	siteID              string
	tenantID            string
	// siteCache maps siteID -> *siteInfo. Entries are never evicted because
	// sites rarely change; restarting the CCM clears the cache.
	siteCache          sync.Map
	machineHealthCache sync.Map // map[machineID]*machineHealthCacheEntry
	// faultManagementAvailable caches whether the fault-management feature
	// is available. nil = not yet checked, non-nil = cached result.
	faultManagementAvailable *bool
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		return NewNicoCloud(config)
	})
}

// NewNicoCloud creates a new NICo cloud provider instance
func NewNicoCloud(config io.Reader) (cloudprovider.Interface, error) {
	// Parse configuration
	cfg, err := parseConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create NICo API client
	var sdkCfg *nico.Configuration
	if cfg.APIName != "" {
		sdkCfg = nico.NewConfigurationWithAPIName(cfg.APIName)
	} else {
		sdkCfg = nico.NewConfiguration()
	}
	sdkCfg.Servers = nico.ServerConfigurations{
		{URL: cfg.Endpoint},
	}
	nicoClient := &nicoAPIClient{
		client: nico.NewAPIClient(sdkCfg),
		token:  cfg.Token,
	}

	klog.Infof("NICo cloud provider initialized for org=%s, site=%s", cfg.OrgName, cfg.SiteID)

	return &NicoCloud{
		nicoClient: nicoClient,
		orgName:             cfg.OrgName,
		siteID:              cfg.SiteID,
		tenantID:            cfg.TenantID,
	}, nil
}

// NewNicoCloudWithClient creates a new NICo cloud provider with injected client (for testing)
func NewNicoCloudWithClient(
	client NicoClientInterface, orgName, siteID, tenantID string,
) cloudprovider.Interface {
	return &NicoCloud{
		nicoClient: client,
		orgName:             orgName,
		siteID:              siteID,
		tenantID:            tenantID,
	}
}

// Initialize provides the cloud provider with the client builder and may be called multiple times
func (c *NicoCloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	klog.Info("Initializing NICo cloud provider")
}

// LoadBalancer returns a LoadBalancer interface.
// NICo does not implement cloud load balancers. Clusters should use
// an external solution such as MetalLB, kube-vip, or a site-local hardware
// load balancer.
func (c *NicoCloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return nil, false
}

// Instances returns an Instances interface (deprecated, use InstancesV2)
func (c *NicoCloud) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

// InstancesV2 returns an InstancesV2 interface for node lifecycle management
func (c *NicoCloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return c, true
}

// Zones returns a Zones interface
func (c *NicoCloud) Zones() (cloudprovider.Zones, bool) {
	return c, true
}

// Clusters returns a Clusters interface (deprecated)
func (c *NicoCloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

// Routes returns a Routes interface
// NICo does not currently support routes
func (c *NicoCloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

// ProviderName returns the cloud provider name
func (c *NicoCloud) ProviderName() string {
	return ProviderName
}

// HasClusterID returns true if the cluster has a cluster ID
func (c *NicoCloud) HasClusterID() bool {
	return true
}

// Config holds the NICo cloud provider configuration
type Config struct {
	// Endpoint is the NICo API endpoint URL
	Endpoint string `yaml:"endpoint"`

	// OrgName is the NICo organization name
	OrgName string `yaml:"orgName"`

	// Token is the NICo API authentication token
	Token string `yaml:"token"`

	// SiteID is the NICo site UUID
	SiteID string `yaml:"siteId"`

	// TenantID is the NICo tenant UUID
	TenantID string `yaml:"tenantId"`

	// APIName overrides the API path segment after /org/{org}/.
	// Leave empty to use the default "carbide" path.
	APIName string `yaml:"apiName"`
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
	if apiName := os.Getenv(EnvAPIName); apiName != "" {
		cfg.APIName = apiName
		klog.V(4).Infof("Using apiName from environment: %s", apiName)
	}

	return cfg, nil
}
