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

	bmm "github.com/nvidia/bare-metal-manager-rest/sdk/standard"
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
	LabelHealthy = "nvidia-carbide.io/healthy"

	// LabelHealthAlertCount is the number of active health alerts on the machine.
	LabelHealthAlertCount = "nvidia-carbide.io/health-alert-count"
)

// machineHealthLabels returns labels describing machine health status.
// Returns nil if health data is unavailable. External controllers (e.g. RHWA
// NodeHealthCheck) can use these labels to take remediation actions.
func (c *NvidiaCarbideCloud) machineHealthLabels(ctx context.Context, instance *bmm.Instance) map[string]string {
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

	machine, httpResp, err := c.nvidiaCarbideClient.GetMachine(ctx, c.orgName, *machineID)
	if err != nil || httpResp.StatusCode != http.StatusOK || machine == nil {
		klog.V(4).Infof("Could not fetch machine %s for health check: %v", *machineID, err)
		return nil
	}

	var labels map[string]string
	if machine.Health == nil {
		labels = nil
	} else if len(machine.Health.Alerts) == 0 {
		labels = map[string]string{
			LabelHealthy: "true",
		}
	} else {
		labels = map[string]string{
			LabelHealthy:          "false",
			LabelHealthAlertCount: fmt.Sprintf("%d", len(machine.Health.Alerts)),
		}
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
		klog.V(2).InfoS("Machine health status changed", "machineID", *machineID, "previous", previousHealthy, "current", currentHealthy)
	}

	c.machineHealthCache.Store(*machineID, &machineHealthCacheEntry{
		labels:    labels,
		expiresAt: time.Now().Add(machineHealthCacheTTL),
	})

	return labels
}
