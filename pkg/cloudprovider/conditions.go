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

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	// ConditionNicoHealthy indicates whether the NICo machine backing this
	// node has any open faults.
	ConditionNicoHealthy v1.NodeConditionType = "NicoHealthy"

	// ConditionNicoFaultRemediation indicates whether an automated
	// remediation workflow is in progress for this node's machine.
	ConditionNicoFaultRemediation v1.NodeConditionType = "NicoFaultRemediation"
)

// nodeConditionsFromHealth builds NodeCondition entries from health labels.
// Returns nil if labels are nil (health data unavailable).
func nodeConditionsFromHealth(labels map[string]string) []v1.NodeCondition {
	if labels == nil {
		return nil
	}

	now := metav1.Now()
	healthy := labels[LabelHealthy]

	if healthy == healthyTrue {
		return []v1.NodeCondition{
			{
				Type:               ConditionNicoHealthy,
				Status:             v1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "NoFaults",
				Message:            "All health checks passing",
			},
			{
				Type:               ConditionNicoFaultRemediation,
				Status:             v1.ConditionFalse,
				LastTransitionTime: now,
				Reason:             "NoRemediation",
				Message:            "No remediation in progress",
			},
		}
	}

	if healthy == healthyFalse {
		component := labels[LabelFaultComponent]
		classification := labels[LabelFaultClassification]
		faultState := labels[LabelFaultState]

		message := "Fault detected"
		if component != "" && classification != "" {
			message = fmt.Sprintf("%s fault: %s", component, classification)
			if faultState != "" {
				message = fmt.Sprintf("%s (%s)", message, faultState)
			}
		}

		conditions := make([]v1.NodeCondition, 0, 2)
		conditions = append(conditions, v1.NodeCondition{
			Type:               ConditionNicoHealthy,
			Status:             v1.ConditionFalse,
			LastTransitionTime: now,
			Reason:             "FaultDetected",
			Message:            message,
		})

		remediationCondition := v1.NodeCondition{
			Type:               ConditionNicoFaultRemediation,
			LastTransitionTime: now,
		}
		if faultState == "remediating" {
			remediationCondition.Status = v1.ConditionTrue
			remediationCondition.Reason = "RemediationInProgress"
			remediationCondition.Message = fmt.Sprintf("Automated remediation in progress for %s", classification)
		} else {
			remediationCondition.Status = v1.ConditionFalse
			remediationCondition.Reason = "NoRemediation"
			remediationCondition.Message = "No remediation in progress"
		}
		conditions = append(conditions, remediationCondition)

		return conditions
	}

	return nil
}

// syncNodeConditions patches the node's status.conditions with NICo health
// conditions. If the kubeClient is nil (not yet initialized) or labels are
// nil (health data unavailable), this is a no-op.
func (c *NicoCloud) syncNodeConditions(ctx context.Context, node *v1.Node, labels map[string]string) {
	if c.kubeClient == nil {
		return
	}

	desired := nodeConditionsFromHealth(labels)
	if desired == nil {
		return
	}

	// Check if conditions actually changed to avoid unnecessary patches
	if !conditionsChanged(node.Status.Conditions, desired) {
		return
	}

	patch := buildConditionPatch(desired)
	if patch == nil {
		return
	}

	_, err := c.kubeClient.CoreV1().Nodes().PatchStatus(ctx, node.Name, patch)
	if err != nil {
		klog.V(2).Infof("Failed to patch node conditions for %s: %v", node.Name, err)
		return
	}
	klog.V(4).Infof("Patched node conditions for %s", node.Name)
}

// conditionsChanged returns true if any desired condition differs from
// the current conditions in type+status.
func conditionsChanged(current []v1.NodeCondition, desired []v1.NodeCondition) bool {
	for _, d := range desired {
		found := false
		for _, c := range current {
			if c.Type == d.Type {
				found = true
				if c.Status != d.Status || c.Reason != d.Reason {
					return true
				}
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

// buildConditionPatch creates a JSON strategic merge patch for node status
// conditions.
func buildConditionPatch(conditions []v1.NodeCondition) []byte {
	type statusPatch struct {
		Status struct {
			Conditions []v1.NodeCondition `json:"conditions"`
		} `json:"status"`
	}

	p := statusPatch{}
	p.Status.Conditions = conditions

	data, err := json.Marshal(p)
	if err != nil {
		klog.V(2).Infof("Failed to marshal condition patch: %v", err)
		return nil
	}
	return data
}
