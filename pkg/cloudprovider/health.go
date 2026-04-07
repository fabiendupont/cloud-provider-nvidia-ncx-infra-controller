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
	"time"

	nico "github.com/NVIDIA/ncx-infra-controller-rest/sdk/standard"
	"k8s.io/klog/v2"
)

const machineHealthCacheTTL = 2 * time.Minute

type machineHealthCacheEntry struct {
	labels    map[string]string
	expiresAt time.Time
}

const (
	// LabelHealthy indicates whether the machine has health alerts.
	// Set to "true" if healthy, "false" if alerts are present.
	LabelHealthy = "nico.io/healthy"

	// LabelHealthAlertCount is the number of active health alerts on the machine.
	LabelHealthAlertCount = "nico.io/health-alert-count"

	// LabelFaultComponent is the infrastructure component affected by the fault.
	LabelFaultComponent = "nico.io/fault-component"

	// LabelFaultClassification is the source-specific fault type.
	LabelFaultClassification = "nico.io/fault-classification"

	// LabelFaultState is the lifecycle state of the fault.
	LabelFaultState = "nico.io/fault-state"
)

// machineHealthLabels returns labels describing machine health status.
// Returns nil if health data is unavailable. External controllers (e.g. RHWA
// NodeHealthCheck) can use these labels to take remediation actions.
//
// If the fault-management capability is available (NEP-0007), labels are
// derived from the structured health events API. Otherwise, falls back to
// parsing the machine.health JSONB field.
func (c *NicoCloud) machineHealthLabels(ctx context.Context, instance *nico.Instance) map[string]string {
	machineID, ok := instance.GetMachineIdOk()
	if !ok || machineID == nil || *machineID == "" {
		return nil
	}

	if cached, ok := c.machineHealthCache.Load(*machineID); ok {
		entry := cached.(*machineHealthCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.labels
		}
	}

	var labels map[string]string
	if c.hasFaultManagement(ctx) {
		labels = c.healthLabelsFromFaultAPI(ctx, *machineID)
	} else {
		labels = c.healthLabelsFromJSONB(ctx, *machineID)
	}

	// Detect health status transitions for logging
	var previousHealthy string
	if cached, ok := c.machineHealthCache.Load(*machineID); ok {
		prev := cached.(*machineHealthCacheEntry)
		if prev.labels != nil {
			previousHealthy = prev.labels[LabelHealthy]
		}
	}
	var currentHealthy string
	if labels != nil {
		currentHealthy = labels[LabelHealthy]
	}
	if previousHealthy != "" && currentHealthy != "" && previousHealthy != currentHealthy {
		klog.V(2).InfoS("Machine health status changed",
			"machineID", *machineID,
			"previous", previousHealthy, "current", currentHealthy)
	}

	c.machineHealthCache.Store(*machineID, &machineHealthCacheEntry{
		labels:    labels,
		expiresAt: time.Now().Add(machineHealthCacheTTL),
	})

	return labels
}

// hasFaultManagement checks whether the NICo API supports the fault-management
// feature (NEP-0007). The result is cached for the lifetime of the CCM process.
func (c *NicoCloud) hasFaultManagement(ctx context.Context) bool {
	if c.faultManagementAvailable != nil {
		return *c.faultManagementAvailable
	}

	caps, _, err := c.nicoClient.GetCapabilities(ctx, c.orgName)
	if err != nil {
		klog.V(4).Infof("Capabilities endpoint unavailable, using JSONB fallback: %v", err)
		result := false
		c.faultManagementAvailable = &result
		return false
	}

	result := false
	for _, f := range caps.Features {
		if f == "fault-management" {
			result = true
			break
		}
	}
	c.faultManagementAvailable = &result
	if result {
		klog.V(2).Info("Fault management capability detected, using structured health events API")
	} else {
		klog.V(2).Info("Fault management capability not available, using JSONB fallback")
	}
	return result
}

// healthLabelsFromFaultAPI derives health labels from the structured health
// events API (NEP-0007). Returns labels with fault component, classification,
// and state from the first open fault event.
func (c *NicoCloud) healthLabelsFromFaultAPI(ctx context.Context, machineID string) map[string]string {
	events, _, err := c.nicoClient.GetHealthEvents(ctx, c.orgName, machineID)
	if err != nil {
		klog.V(4).Infof("Failed to fetch health events for machine %s: %v", machineID, err)
		return nil
	}

	if len(events) == 0 {
		return map[string]string{
			LabelHealthy: "true",
		}
	}

	labels := map[string]string{
		LabelHealthy:          "false",
		LabelHealthAlertCount: fmt.Sprintf("%d", len(events)),
	}

	// Use the first fault event for component-level labels
	first := events[0]
	if first.Component != "" {
		labels[LabelFaultComponent] = first.Component
	}
	if first.Classification != "" {
		labels[LabelFaultClassification] = first.Classification
	}
	if first.State != "" {
		labels[LabelFaultState] = first.State
	}

	return labels
}

// healthLabelsFromJSONB derives health labels from the machine.health JSONB
// field. This is the legacy path used when fault-management is not available.
func (c *NicoCloud) healthLabelsFromJSONB(ctx context.Context, machineID string) map[string]string {
	machine, httpResp, err := c.nicoClient.GetMachine(ctx, c.orgName, machineID)
	if err != nil || httpResp.StatusCode != http.StatusOK || machine == nil {
		klog.V(4).Infof("Could not fetch machine %s for health check: %v", machineID, err)
		return nil
	}

	if machine.Health == nil {
		return nil
	}

	if len(machine.Health.Alerts) == 0 {
		return map[string]string{
			LabelHealthy: "true",
		}
	}

	return map[string]string{
		LabelHealthy:          "false",
		LabelHealthAlertCount: fmt.Sprintf("%d", len(machine.Health.Alerts)),
	}
}

const (
	// LabelSiteNVLink indicates whether the site supports NVLink partitioning.
	LabelSiteNVLink = "nico.io/site-nvlink"

	// LabelSiteNSG indicates whether the site supports network security groups.
	LabelSiteNSG = "nico.io/site-nsg"

	// LabelSiteRLA indicates whether the site supports rack-level administration.
	LabelSiteRLA = "nico.io/site-rla"
)

// siteCapabilityLabels returns labels describing site capabilities.
// Data is served from the site cache populated by getCachedSite().
func (c *NicoCloud) siteCapabilityLabels(ctx context.Context, siteID string) map[string]string {
	info, err := c.getCachedSite(ctx, siteID)
	if err != nil || info == nil {
		klog.V(4).Infof("Could not fetch site %s for capabilities: %v", siteID, err)
		return nil
	}

	labels := map[string]string{}
	if info.nvLinkPartition != nil {
		labels[LabelSiteNVLink] = fmt.Sprintf("%t", *info.nvLinkPartition)
	}
	if info.networkSecurityGroup != nil {
		labels[LabelSiteNSG] = fmt.Sprintf("%t", *info.networkSecurityGroup)
	}
	if info.rackLevelAdministration != nil {
		labels[LabelSiteRLA] = fmt.Sprintf("%t", *info.rackLevelAdministration)
	}

	return labels
}
