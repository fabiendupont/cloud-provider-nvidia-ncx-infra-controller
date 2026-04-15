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
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNodeConditionsFromHealth_NilLabels(t *testing.T) {
	conditions := nodeConditionsFromHealth(nil)
	if conditions != nil {
		t.Errorf("expected nil conditions for nil labels, got %v", conditions)
	}
}

func TestNodeConditionsFromHealth_Healthy(t *testing.T) {
	labels := map[string]string{LabelHealthy: "true"}
	conditions := nodeConditionsFromHealth(labels)

	if len(conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(conditions))
	}

	healthy := conditions[0]
	if healthy.Type != ConditionNicoHealthy {
		t.Errorf("type = %q, want %q", healthy.Type, ConditionNicoHealthy)
	}
	if healthy.Status != v1.ConditionTrue {
		t.Errorf("status = %v, want True", healthy.Status)
	}
	if healthy.Reason != "NoFaults" {
		t.Errorf("reason = %q, want %q", healthy.Reason, "NoFaults")
	}

	remediation := conditions[1]
	if remediation.Type != ConditionNicoFaultRemediation {
		t.Errorf("type = %q, want %q", remediation.Type, ConditionNicoFaultRemediation)
	}
	if remediation.Status != v1.ConditionFalse {
		t.Errorf("status = %v, want False", remediation.Status)
	}
}

func TestNodeConditionsFromHealth_FaultOpen(t *testing.T) {
	labels := map[string]string{
		LabelHealthy:             "false",
		LabelFaultComponent:      "gpu",
		LabelFaultClassification: "gpu-xid-48",
		LabelFaultState:          "open",
	}
	conditions := nodeConditionsFromHealth(labels)

	if len(conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(conditions))
	}

	healthy := conditions[0]
	if healthy.Status != v1.ConditionFalse {
		t.Errorf("NicoHealthy status = %v, want False", healthy.Status)
	}
	if healthy.Reason != "FaultDetected" {
		t.Errorf("reason = %q, want %q", healthy.Reason, "FaultDetected")
	}
	if healthy.Message != "gpu fault: gpu-xid-48 (open)" {
		t.Errorf("message = %q", healthy.Message)
	}

	remediation := conditions[1]
	if remediation.Status != v1.ConditionFalse {
		t.Errorf("NicoFaultRemediation status = %v, want False", remediation.Status)
	}
}

func TestNodeConditionsFromHealth_FaultRemediating(t *testing.T) {
	labels := map[string]string{
		LabelHealthy:             "false",
		LabelFaultComponent:      "gpu",
		LabelFaultClassification: "gpu-xid-48",
		LabelFaultState:          "remediating",
	}
	conditions := nodeConditionsFromHealth(labels)

	if len(conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(conditions))
	}

	remediation := conditions[1]
	if remediation.Status != v1.ConditionTrue {
		t.Errorf("NicoFaultRemediation status = %v, want True", remediation.Status)
	}
	if remediation.Reason != "RemediationInProgress" {
		t.Errorf("reason = %q, want %q", remediation.Reason, "RemediationInProgress")
	}
}

func TestSyncNodeConditions_NilKubeClient(t *testing.T) {
	cloud := &NicoCloud{}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node"}}
	labels := map[string]string{LabelHealthy: "true"}

	// Should not panic with nil kubeClient
	cloud.syncNodeConditions(context.Background(), node, labels)
}

func TestSyncNodeConditions_PatchesNode(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status:     v1.NodeStatus{},
	}

	client := fake.NewClientset(node)
	cloud := &NicoCloud{kubeClient: client}

	labels := map[string]string{
		LabelHealthy:             "false",
		LabelFaultComponent:      "gpu",
		LabelFaultClassification: "gpu-xid-48",
		LabelFaultState:          "remediating",
	}

	cloud.syncNodeConditions(context.Background(), node, labels)

	// Fetch the node to check conditions
	updated, err := client.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}

	var foundHealthy, foundRemediation bool
	for _, c := range updated.Status.Conditions {
		switch c.Type {
		case ConditionNicoHealthy:
			foundHealthy = true
			if c.Status != v1.ConditionFalse {
				t.Errorf("NicoHealthy status = %v, want False", c.Status)
			}
		case ConditionNicoFaultRemediation:
			foundRemediation = true
			if c.Status != v1.ConditionTrue {
				t.Errorf("NicoFaultRemediation status = %v, want True", c.Status)
			}
		}
	}

	if !foundHealthy {
		t.Error("NicoHealthy condition not found on node")
	}
	if !foundRemediation {
		t.Error("NicoFaultRemediation condition not found on node")
	}
}

func TestSyncNodeConditions_SkipsWhenUnchanged(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{Type: ConditionNicoHealthy, Status: v1.ConditionTrue, Reason: "NoFaults"},
				{Type: ConditionNicoFaultRemediation, Status: v1.ConditionFalse, Reason: "NoRemediation"},
			},
		},
	}

	client := fake.NewClientset(node)
	cloud := &NicoCloud{kubeClient: client}

	labels := map[string]string{LabelHealthy: "true"}

	// Get action count before
	actionsBefore := len(client.Actions())

	cloud.syncNodeConditions(context.Background(), node, labels)

	// No new actions should have been taken (no patch needed)
	actionsAfter := len(client.Actions())
	// Only the initial Create from NewSimpleClientset counts, no Patch should happen
	if actionsAfter != actionsBefore {
		t.Errorf("expected no new actions (conditions unchanged), got %d new actions", actionsAfter-actionsBefore)
	}
}

func TestConditionsChanged(t *testing.T) {
	tests := []struct {
		name    string
		current []v1.NodeCondition
		desired []v1.NodeCondition
		want    bool
	}{
		{
			name:    "no current conditions",
			current: nil,
			desired: []v1.NodeCondition{{Type: ConditionNicoHealthy, Status: v1.ConditionTrue}},
			want:    true,
		},
		{
			name:    "same status and reason",
			current: []v1.NodeCondition{{Type: ConditionNicoHealthy, Status: v1.ConditionTrue, Reason: "NoFaults"}},
			desired: []v1.NodeCondition{{Type: ConditionNicoHealthy, Status: v1.ConditionTrue, Reason: "NoFaults"}},
			want:    false,
		},
		{
			name:    "status changed",
			current: []v1.NodeCondition{{Type: ConditionNicoHealthy, Status: v1.ConditionTrue, Reason: "NoFaults"}},
			desired: []v1.NodeCondition{{Type: ConditionNicoHealthy, Status: v1.ConditionFalse, Reason: "FaultDetected"}},
			want:    true,
		},
		{
			name:    "reason changed",
			current: []v1.NodeCondition{{Type: ConditionNicoHealthy, Status: v1.ConditionFalse, Reason: "FaultDetected"}},
			desired: []v1.NodeCondition{{Type: ConditionNicoHealthy, Status: v1.ConditionFalse, Reason: "DifferentFault"}},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := conditionsChanged(tt.current, tt.desired)
			if got != tt.want {
				t.Errorf("conditionsChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}
