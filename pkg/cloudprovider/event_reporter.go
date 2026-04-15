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
	"sync"
	"time"

	nico "github.com/NVIDIA/ncx-infra-controller-rest/sdk/standard"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/fabiendupont/cloud-provider-nvidia-ncx-infra-controller/pkg/providerid"
)

const (
	// deduplicationWindow prevents re-reporting the same event within this period.
	deduplicationWindow = 10 * time.Minute

	// deduplicationCleanupInterval is how often expired dedup entries are cleaned.
	deduplicationCleanupInterval = 5 * time.Minute
)

// nodeEventReporter watches Kubernetes nodes and reports health-relevant
// transitions (Ready=False, cordoned) back to NICo via the health events
// ingest endpoint. It is a no-op when fault-management is not available.
type nodeEventReporter struct {
	nicoClient NicoClientInterface
	orgName    string
	retry      retryConfig
	// reportedEvents tracks recently reported events for deduplication.
	// Key: "{machineID}:{classification}", Value: time.Time of last report.
	reportedEvents sync.Map
}

// start begins watching nodes and reporting events. If hasFaultMgmt is false,
// this is a no-op. The reporter runs until stopCh is closed.
func (r *nodeEventReporter) start(
	kubeClient kubernetes.Interface,
	hasFaultMgmt bool,
	stopCh <-chan struct{},
) {
	if !hasFaultMgmt {
		klog.V(2).Info("Fault management not available, event reporter disabled")
		return
	}

	klog.Info("Starting node event reporter")

	factory := informers.NewSharedInformerFactory(kubeClient, 0)
	nodeInformer := factory.Core().V1().Nodes().Informer()

	_, _ = nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNode, ok1 := oldObj.(*v1.Node)
			newNode, ok2 := newObj.(*v1.Node)
			if !ok1 || !ok2 {
				return
			}
			r.handleNodeUpdate(oldNode, newNode)
		},
	})

	// Start cleanup goroutine
	go r.cleanupLoop(stopCh)

	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)
}

// handleNodeUpdate checks for health-relevant state transitions and reports
// them to NICo.
func (r *nodeEventReporter) handleNodeUpdate(oldNode, newNode *v1.Node) {
	// Skip nodes without a NICo provider ID
	if newNode.Spec.ProviderID == "" {
		return
	}

	// Skip if NICo already knows about the fault (avoids feedback loop).
	// If NicoHealthy=False is set as a label, the fault originated from NICo.
	if newNode.Labels != nil && newNode.Labels[LabelHealthy] == healthyFalse {
		return
	}

	// Check for Ready condition transition: True -> False
	oldReady := getNodeConditionStatus(oldNode, v1.NodeReady)
	newReady := getNodeConditionStatus(newNode, v1.NodeReady)

	if oldReady == v1.ConditionTrue && newReady == v1.ConditionFalse {
		reason := getNodeConditionReason(newNode, v1.NodeReady)
		r.reportEvent(newNode, "node-not-ready",
			"Node "+newNode.Name+" has Ready=False, reason: "+reason)
		return
	}

	// Check for cordon transition: false -> true
	if !oldNode.Spec.Unschedulable && newNode.Spec.Unschedulable {
		r.reportEvent(newNode, "node-cordoned",
			"Node "+newNode.Name+" has been cordoned")
		return
	}
}

// reportEvent sends a health event to NICo, subject to deduplication.
func (r *nodeEventReporter) reportEvent(node *v1.Node, classification, message string) {
	parsed, err := providerid.ParseProviderID(node.Spec.ProviderID)
	if err != nil {
		klog.V(4).Infof("Cannot parse provider ID for event reporting: %v", err)
		return
	}

	// The provider ID contains the instance ID, but we need the machine ID.
	// Use the instance ID as a reasonable proxy — the NICo backend resolves
	// the machine from the instance.
	machineID := parsed.InstanceID.String()

	dedupKey := machineID + ":" + classification
	if lastReport, ok := r.reportedEvents.Load(dedupKey); ok {
		if time.Since(lastReport.(time.Time)) < deduplicationWindow {
			klog.V(4).Infof("Skipping duplicate event %s for %s", classification, node.Name)
			return
		}
	}

	detectedAt := time.Now().UTC()
	event := *nico.NewFaultIngestionRequest("k8s-ccm", "warning", "node", message)
	event.Classification = &classification
	event.MachineId = &machineID
	event.DetectedAt = &detectedAt

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _, err = retryDo(ctx, "IngestHealthEvent", r.retry, func() (*nico.FaultEvent, *http.Response, error) {
		return r.nicoClient.IngestHealthEvent(ctx, r.orgName, event)
	})
	if err != nil {
		klog.V(2).Infof("Failed to report event %s for %s: %v", classification, node.Name, err)
		return
	}

	r.reportedEvents.Store(dedupKey, time.Now())
	klog.V(2).Infof("Reported event %s for node %s to NICo", classification, node.Name)
}

// cleanupLoop periodically removes expired deduplication entries.
func (r *nodeEventReporter) cleanupLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(deduplicationCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			r.reportedEvents.Range(func(key, value interface{}) bool {
				if time.Since(value.(time.Time)) > deduplicationWindow {
					r.reportedEvents.Delete(key)
				}
				return true
			})
		}
	}
}

// getNodeConditionStatus returns the status of a node condition by type.
func getNodeConditionStatus(node *v1.Node, condType v1.NodeConditionType) v1.ConditionStatus {
	for _, c := range node.Status.Conditions {
		if c.Type == condType {
			return c.Status
		}
	}
	return v1.ConditionUnknown
}

// getNodeConditionReason returns the reason of a node condition by type.
func getNodeConditionReason(node *v1.Node, condType v1.NodeConditionType) string {
	for _, c := range node.Status.Conditions {
		if c.Type == condType {
			return c.Reason
		}
	}
	return "Unknown"
}
