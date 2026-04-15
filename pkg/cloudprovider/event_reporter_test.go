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
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	nico "github.com/NVIDIA/ncx-infra-controller-rest/sdk/standard"
	"github.com/google/uuid"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fabiendupont/cloud-provider-nvidia-ncx-infra-controller/pkg/providerid"
)

func makeTestNode(ready v1.ConditionStatus, unschedulable bool) *v1.Node {
	instanceID := uuid.New()
	pid := providerid.NewProviderID("test-org", "test-tenant", "test-site", instanceID)

	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Spec: v1.NodeSpec{
			ProviderID:    pid.String(),
			Unschedulable: unschedulable,
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: ready,
					Reason: "KubeletReady",
				},
			},
		},
	}
}

func TestHandleNodeUpdate_ReadyToNotReady(t *testing.T) {
	var ingestCalled atomic.Int32

	reporter := &nodeEventReporter{
		nicoClient: &mockNicoClient{
			ingestHealthEvent: func(
				ctx context.Context, org string, event nico.FaultIngestionRequest,
			) (*nico.FaultEvent, *http.Response, error) {
				ingestCalled.Add(1)
				if event.Classification == nil || *event.Classification != "node-not-ready" {
					t.Errorf("classification = %v, want %q", event.Classification, "node-not-ready")
				}
				if event.Source != "k8s-ccm" {
					t.Errorf("source = %q, want %q", event.Source, "k8s-ccm")
				}
				return nil, &http.Response{StatusCode: 202}, nil
			},
		},
		orgName: "test-org",
		retry:   retryConfig{maxRetries: 0, initialBackoff: time.Millisecond},
	}

	oldNode := makeTestNode(v1.ConditionTrue, false)
	newNode := makeTestNode(v1.ConditionFalse, false)
	// Use the same provider ID
	newNode.Spec.ProviderID = oldNode.Spec.ProviderID

	reporter.handleNodeUpdate(oldNode, newNode)

	if ingestCalled.Load() != 1 {
		t.Errorf("IngestHealthEvent called %d times, want 1", ingestCalled.Load())
	}
}

func TestHandleNodeUpdate_Cordoned(t *testing.T) {
	var ingestCalled atomic.Int32

	reporter := &nodeEventReporter{
		nicoClient: &mockNicoClient{
			ingestHealthEvent: func(
				ctx context.Context, org string, event nico.FaultIngestionRequest,
			) (*nico.FaultEvent, *http.Response, error) {
				ingestCalled.Add(1)
				if event.Classification == nil || *event.Classification != "node-cordoned" {
					t.Errorf("classification = %v, want %q", event.Classification, "node-cordoned")
				}
				return nil, &http.Response{StatusCode: 202}, nil
			},
		},
		orgName: "test-org",
		retry:   retryConfig{maxRetries: 0, initialBackoff: time.Millisecond},
	}

	oldNode := makeTestNode(v1.ConditionTrue, false)
	newNode := makeTestNode(v1.ConditionTrue, true)
	newNode.Spec.ProviderID = oldNode.Spec.ProviderID

	reporter.handleNodeUpdate(oldNode, newNode)

	if ingestCalled.Load() != 1 {
		t.Errorf("IngestHealthEvent called %d times, want 1", ingestCalled.Load())
	}
}

func TestHandleNodeUpdate_SkipsNicoFault(t *testing.T) {
	var ingestCalled atomic.Int32

	reporter := &nodeEventReporter{
		nicoClient: &mockNicoClient{
			ingestHealthEvent: func(
				ctx context.Context, org string, event nico.FaultIngestionRequest,
			) (*nico.FaultEvent, *http.Response, error) {
				ingestCalled.Add(1)
				return nil, &http.Response{StatusCode: 202}, nil
			},
		},
		orgName: "test-org",
		retry:   retryConfig{maxRetries: 0, initialBackoff: time.Millisecond},
	}

	oldNode := makeTestNode(v1.ConditionTrue, false)
	newNode := makeTestNode(v1.ConditionFalse, false)
	newNode.Spec.ProviderID = oldNode.Spec.ProviderID
	// Set NICo health label indicating fault originated from NICo
	newNode.Labels = map[string]string{LabelHealthy: "false"}

	reporter.handleNodeUpdate(oldNode, newNode)

	if ingestCalled.Load() != 0 {
		t.Errorf("IngestHealthEvent should not be called when NICo fault label is set, called %d times", ingestCalled.Load())
	}
}

func TestHandleNodeUpdate_Deduplication(t *testing.T) {
	var ingestCalled atomic.Int32

	reporter := &nodeEventReporter{
		nicoClient: &mockNicoClient{
			ingestHealthEvent: func(
				ctx context.Context, org string, event nico.FaultIngestionRequest,
			) (*nico.FaultEvent, *http.Response, error) {
				ingestCalled.Add(1)
				return nil, &http.Response{StatusCode: 202}, nil
			},
		},
		orgName: "test-org",
		retry:   retryConfig{maxRetries: 0, initialBackoff: time.Millisecond},
	}

	oldNode := makeTestNode(v1.ConditionTrue, false)
	newNode := makeTestNode(v1.ConditionFalse, false)
	newNode.Spec.ProviderID = oldNode.Spec.ProviderID

	// First call should report
	reporter.handleNodeUpdate(oldNode, newNode)
	if ingestCalled.Load() != 1 {
		t.Fatalf("first call: IngestHealthEvent called %d times, want 1", ingestCalled.Load())
	}

	// Second call within dedup window should be skipped
	reporter.handleNodeUpdate(oldNode, newNode)
	if ingestCalled.Load() != 1 {
		t.Errorf("second call: IngestHealthEvent called %d times, want 1 (dedup)", ingestCalled.Load())
	}
}

func TestHandleNodeUpdate_NoTransition(t *testing.T) {
	var ingestCalled atomic.Int32

	reporter := &nodeEventReporter{
		nicoClient: &mockNicoClient{
			ingestHealthEvent: func(
				ctx context.Context, org string, event nico.FaultIngestionRequest,
			) (*nico.FaultEvent, *http.Response, error) {
				ingestCalled.Add(1)
				return nil, &http.Response{StatusCode: 202}, nil
			},
		},
		orgName: "test-org",
		retry:   retryConfig{maxRetries: 0, initialBackoff: time.Millisecond},
	}

	// Both nodes are healthy - no transition
	oldNode := makeTestNode(v1.ConditionTrue, false)
	newNode := makeTestNode(v1.ConditionTrue, false)
	newNode.Spec.ProviderID = oldNode.Spec.ProviderID

	reporter.handleNodeUpdate(oldNode, newNode)

	if ingestCalled.Load() != 0 {
		t.Errorf("IngestHealthEvent should not be called on no transition, called %d times", ingestCalled.Load())
	}
}

func TestGetNodeConditionStatus(t *testing.T) {
	node := &v1.Node{
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{Type: v1.NodeReady, Status: v1.ConditionTrue},
				{Type: v1.NodeMemoryPressure, Status: v1.ConditionFalse},
			},
		},
	}

	if got := getNodeConditionStatus(node, v1.NodeReady); got != v1.ConditionTrue {
		t.Errorf("Ready = %v, want True", got)
	}
	if got := getNodeConditionStatus(node, v1.NodeDiskPressure); got != v1.ConditionUnknown {
		t.Errorf("DiskPressure = %v, want Unknown", got)
	}
}
