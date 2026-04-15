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
	"testing"

	nico "github.com/NVIDIA/ncx-infra-controller-rest/sdk/standard"
	"k8s.io/component-base/metrics/testutil"
)

func TestHealthCacheMetrics(t *testing.T) {
	hasFaultMgmt := true
	machineID := "machine-metrics-test"

	cloud := &NicoCloud{
		nicoClient: &mockNicoClient{
			getHealthEvents: func(ctx context.Context, org string, mid string) ([]nico.FaultEvent, *http.Response, error) {
				return []nico.FaultEvent{}, &http.Response{StatusCode: 200}, nil
			},
		},
		orgName:                  "test-org",
		faultManagementAvailable: &hasFaultMgmt,
	}

	instance := &nico.Instance{}
	instance.MachineId = *nico.NewNullableString(&machineID)

	hitsBefore, _ := testutil.GetCounterMetricValue(healthCacheHits)
	missesBefore, _ := testutil.GetCounterMetricValue(healthCacheMisses)

	// First call: cache miss
	cloud.machineHealthLabels(context.Background(), instance)
	missesAfter, _ := testutil.GetCounterMetricValue(healthCacheMisses)
	if missesAfter != missesBefore+1 {
		t.Errorf("expected cache miss, misses=%v want=%v", missesAfter, missesBefore+1)
	}

	// Second call: cache hit
	cloud.machineHealthLabels(context.Background(), instance)
	hitsAfter, _ := testutil.GetCounterMetricValue(healthCacheHits)
	if hitsAfter != hitsBefore+1 {
		t.Errorf("expected cache hit, hits=%v want=%v", hitsAfter, hitsBefore+1)
	}
}

func TestNodesUnhealthyMetric(t *testing.T) {
	hasFaultMgmt := true
	machineID := "machine-unhealthy-test"

	callCount := 0
	cloud := &NicoCloud{
		nicoClient: &mockNicoClient{
			getHealthEvents: func(ctx context.Context, org string, mid string) ([]nico.FaultEvent, *http.Response, error) {
				callCount++
				if callCount == 1 {
					return []nico.FaultEvent{
						{Component: ptr("gpu"), Classification: ptr("gpu-xid-48"), State: ptr("open")},
					}, &http.Response{StatusCode: 200}, nil
				}
				return []nico.FaultEvent{}, &http.Response{StatusCode: 200}, nil
			},
		},
		orgName:                  "test-org",
		faultManagementAvailable: &hasFaultMgmt,
	}

	instance := &nico.Instance{}
	instance.MachineId = *nico.NewNullableString(&machineID)

	before, _ := testutil.GetGaugeMetricValue(nodesUnhealthy)

	// First call: machine is unhealthy
	cloud.machineHealthLabels(context.Background(), instance)
	after, _ := testutil.GetGaugeMetricValue(nodesUnhealthy)
	if after != before+1 {
		t.Errorf("expected nodesUnhealthy to increment, got %v (before=%v)", after, before)
	}

	// Expire the cache to force re-fetch
	if cached, ok := cloud.machineHealthCache.Load(machineID); ok {
		entry := cached.(*machineHealthCacheEntry)
		entry.expiresAt = entry.expiresAt.Add(-machineHealthCacheTTL * 2)
	}

	// Second call: machine is now healthy
	cloud.machineHealthLabels(context.Background(), instance)
	after2, _ := testutil.GetGaugeMetricValue(nodesUnhealthy)
	if after2 != before {
		t.Errorf("expected nodesUnhealthy to return to %v, got %v", before, after2)
	}
}
