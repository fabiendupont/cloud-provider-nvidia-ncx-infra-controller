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
	"strings"

	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	bmm "github.com/nvidia/bare-metal-manager-rest/sdk/standard"

	"github.com/fabiendupont/cloud-provider-nvidia-carbide/pkg/providerid"
)

const defaultInstanceType = "nvidia-carbide-instance"

// InstanceExists checks if the instance exists for the given node
func (c *NvidiaCarbideCloud) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	providerID := node.Spec.ProviderID
	if providerID == "" {
		return false, fmt.Errorf("node %s has no provider ID", node.Name)
	}

	parsed, err := providerid.ParseProviderID(providerID)
	if err != nil {
		return false, fmt.Errorf("failed to parse provider ID: %w", err)
	}

	_, httpResp, err := c.nvidiaCarbideClient.GetInstance(ctx, c.orgName, parsed.InstanceID.String())
	if err != nil {
		klog.Warningf("Instance %s not found: %v", parsed.InstanceID, err)
		return false, nil
	}

	if httpResp.StatusCode != http.StatusOK {
		klog.Warningf("Instance %s not found, status %d", parsed.InstanceID, httpResp.StatusCode)
		return false, nil
	}

	klog.V(2).InfoS("Instance exists", "node", node.Name, "instanceID", parsed.InstanceID)
	return true, nil
}

// InstanceShutdown checks if the instance is shutdown
func (c *NvidiaCarbideCloud) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	providerID := node.Spec.ProviderID
	if providerID == "" {
		return false, fmt.Errorf("node %s has no provider ID", node.Name)
	}

	parsed, err := providerid.ParseProviderID(providerID)
	if err != nil {
		return false, fmt.Errorf("failed to parse provider ID: %w", err)
	}

	instance, httpResp, err := c.nvidiaCarbideClient.GetInstance(ctx, c.orgName, parsed.InstanceID.String())
	if err != nil {
		return false, fmt.Errorf("failed to get instance: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK || instance == nil {
		return false, fmt.Errorf("failed to get instance, status %d", httpResp.StatusCode)
	}

	if instance.Status != nil {
		switch *instance.Status {
		case bmm.INSTANCESTATUS_TERMINATING,
			bmm.INSTANCESTATUS_ERROR:
			klog.V(2).InfoS("Instance is shut down", "node", node.Name, "instanceID", parsed.InstanceID, "status", *instance.Status)
			return true, nil
		// "Terminated" has no SDK constant (the OpenAPI spec does not define it),
		// but the platform can return it after an instance finishes terminating.
		case "Terminated":
			klog.V(2).InfoS("Instance is shut down", "node", node.Name, "instanceID", parsed.InstanceID, "status", "Terminated")
			return true, nil
		default:
			return false, nil
		}
	}

	return false, nil
}

// InstanceMetadata returns metadata for the instance
func (c *NvidiaCarbideCloud) InstanceMetadata(
	ctx context.Context, node *v1.Node,
) (*cloudprovider.InstanceMetadata, error) {
	providerID := node.Spec.ProviderID
	if providerID == "" {
		return nil, fmt.Errorf("node %s has no provider ID", node.Name)
	}

	parsed, err := providerid.ParseProviderID(providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse provider ID: %w", err)
	}

	instance, httpResp, err := c.nvidiaCarbideClient.GetInstance(ctx, c.orgName, parsed.InstanceID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK || instance == nil {
		return nil, fmt.Errorf("failed to get instance, status %d", httpResp.StatusCode)
	}

	instanceType := c.resolveInstanceType(ctx, instance)

	siteID := c.siteID
	if instance.HasSiteId() {
		siteID = instance.GetSiteId()
	}
	zone, region := c.resolveZoneAndRegion(ctx, siteID)

	addresses := c.extractNodeAddresses(instance, node.Name)

	additionalLabels := c.machineHealthLabels(ctx, instance)

	metadata := &cloudprovider.InstanceMetadata{
		ProviderID:       providerID,
		InstanceType:     instanceType,
		NodeAddresses:    addresses,
		Zone:             zone,
		Region:           region,
		AdditionalLabels: additionalLabels,
	}

	klog.V(4).Infof("Instance metadata for %s: %+v", node.Name, metadata)

	return metadata, nil
}

// resolveInstanceType looks up the instance type name from the Carbide API.
// Falls back to defaultInstanceType if the lookup fails.
func (c *NvidiaCarbideCloud) resolveInstanceType(ctx context.Context, instance *bmm.Instance) string {
	if !instance.HasInstanceTypeId() {
		klog.Warning("Instance has no instance type ID, using fallback")
		return defaultInstanceType
	}

	instanceTypeID := instance.GetInstanceTypeId()
	it, httpResp, err := c.nvidiaCarbideClient.GetInstanceType(ctx, c.orgName, instanceTypeID)
	if err != nil || httpResp.StatusCode != http.StatusOK || it == nil {
		klog.Warningf("Failed to get instance type %s, using fallback: %v", instanceTypeID, err)
		return defaultInstanceType
	}

	if it.HasName() {
		return it.GetName()
	}

	return defaultInstanceType
}

// resolveZoneAndRegion looks up the site from the Carbide API and constructs
// zone ({country}-{state}-{site-name}) and region ({country}-{state}).
// Falls back to site-ID-based placeholders if the lookup fails.
func (c *NvidiaCarbideCloud) resolveZoneAndRegion(ctx context.Context, siteID string) (string, string) {
	info, err := c.getCachedSite(ctx, siteID)
	if err != nil || info == nil {
		klog.Warningf("Failed to get site %s, using fallback zone/region: %v", siteID, err)
		return fmt.Sprintf("nvidia-carbide-zone-%s", siteID),
			fmt.Sprintf("nvidia-carbide-region-%s", siteID)
	}

	if info.country != "" && info.state != "" {
		region := info.country + "-" + info.state
		zone := region + "-" + info.name
		return zone, region
	}

	klog.Warningf("Site %s has no location data, using fallback zone/region", siteID)
	if info.name != "" {
		return info.name, info.name
	}
	return fmt.Sprintf("nvidia-carbide-zone-%s", siteID),
		fmt.Sprintf("nvidia-carbide-region-%s", siteID)
}

// getCachedSite returns cached site info or fetches it from the API.
func (c *NvidiaCarbideCloud) getCachedSite(ctx context.Context, siteID string) (*siteInfo, error) {
	if cached, ok := c.siteCache.Load(siteID); ok {
		return cached.(*siteInfo), nil
	}

	site, httpResp, err := c.nvidiaCarbideClient.GetSite(ctx, c.orgName, siteID)
	if err != nil || httpResp.StatusCode != http.StatusOK || site == nil {
		return nil, fmt.Errorf("failed to get site %s: %w", siteID, err)
	}

	info := &siteInfo{
		name: strings.ToLower(strings.ReplaceAll(site.GetName(), " ", "-")),
	}
	if site.HasLocation() {
		loc := site.GetLocation()
		info.country = strings.ToLower(loc.GetCountry())
		info.state = strings.ToLower(loc.GetState())
		info.city = strings.ToLower(loc.GetCity())
	}

	c.siteCache.Store(siteID, info)
	return info, nil
}

// siteInfo holds cached site location data.
type siteInfo struct {
	name    string
	country string
	state   string
	city    string
}

// extractNodeAddresses classifies instance interfaces into Kubernetes node addresses.
// The first non-physical interface's first IP is used as NodeInternalIP.
// Physical interfaces (CIN/InfiniBand) are skipped as they are not Kubernetes-routable.
func (c *NvidiaCarbideCloud) extractNodeAddresses(instance *bmm.Instance, nodeName string) []v1.NodeAddress {
	var addresses []v1.NodeAddress
	foundInternalIP := false

	for _, iface := range instance.Interfaces {
		// Skip physical interfaces (CIN/InfiniBand) — not Kubernetes-routable
		if iface.HasIsPhysical() && iface.GetIsPhysical() {
			continue
		}

		for _, ipAddr := range iface.IpAddresses {
			if !foundInternalIP {
				addresses = append(addresses, v1.NodeAddress{
					Type:    v1.NodeInternalIP,
					Address: ipAddr,
				})
				klog.V(2).InfoS("Resolved node internal IP", "node", nodeName, "address", ipAddr)
				foundInternalIP = true
				break
			}
			// TODO: Additional non-physical interfaces could be classified as
			// NodeExternalIP if we can determine management vs. data interfaces
			// from subnet metadata. For now, only the first IP is used.
		}

		// Only stop after we've found an IP; if this non-physical interface
		// had no IPs, continue to the next one.
		if foundInternalIP {
			break
		}
	}

	addresses = append(addresses, v1.NodeAddress{
		Type:    v1.NodeHostName,
		Address: nodeName,
	})

	return addresses
}
