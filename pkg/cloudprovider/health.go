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

	bmm "github.com/nvidia/bare-metal-manager-rest/sdk/standard"
	"k8s.io/klog/v2"
)

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

	machine, httpResp, err := c.nvidiaCarbideClient.GetMachine(ctx, c.orgName, *machineID)
	if err != nil || httpResp.StatusCode != http.StatusOK || machine == nil {
		klog.V(4).Infof("Could not fetch machine %s for health check: %v", *machineID, err)
		return nil
	}

	if machine.Health == nil {
		return nil
	}

	alerts := machine.Health.Alerts
	if len(alerts) == 0 {
		return map[string]string{
			LabelHealthy: "true",
		}
	}

	return map[string]string{
		LabelHealthy:          "false",
		LabelHealthAlertCount: fmt.Sprintf("%d", len(alerts)),
	}
}
