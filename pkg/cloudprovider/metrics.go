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
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

const metricsSubsystem = "nico_ccm"

var (
	apiLatency = metrics.NewHistogramVec(
		&metrics.HistogramOpts{
			Subsystem:      metricsSubsystem,
			Name:           "api_latency_seconds",
			Help:           "Latency of NICo API calls in seconds.",
			Buckets:        []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			StabilityLevel: metrics.ALPHA,
		},
		[]string{"endpoint"},
	)

	apiErrors = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Subsystem:      metricsSubsystem,
			Name:           "api_errors_total",
			Help:           "Total NICo API errors by endpoint and error type.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{"endpoint", "error_type"},
	)

	nodesManaged = metrics.NewGauge(
		&metrics.GaugeOpts{
			Subsystem:      metricsSubsystem,
			Name:           "nodes_managed",
			Help:           "Number of nodes managed by the CCM.",
			StabilityLevel: metrics.ALPHA,
		},
	)

	nodesUnhealthy = metrics.NewGauge(
		&metrics.GaugeOpts{
			Subsystem:      metricsSubsystem,
			Name:           "nodes_unhealthy",
			Help:           "Number of nodes with open faults.",
			StabilityLevel: metrics.ALPHA,
		},
	)

	healthCacheHits = metrics.NewCounter(
		&metrics.CounterOpts{
			Subsystem:      metricsSubsystem,
			Name:           "health_cache_hits_total",
			Help:           "Health cache hit count.",
			StabilityLevel: metrics.ALPHA,
		},
	)

	healthCacheMisses = metrics.NewCounter(
		&metrics.CounterOpts{
			Subsystem:      metricsSubsystem,
			Name:           "health_cache_misses_total",
			Help:           "Health cache miss count.",
			StabilityLevel: metrics.ALPHA,
		},
	)
)

func init() {
	legacyregistry.MustRegister(apiLatency)
	legacyregistry.MustRegister(apiErrors)
	legacyregistry.MustRegister(nodesManaged)
	legacyregistry.MustRegister(nodesUnhealthy)
	legacyregistry.MustRegister(healthCacheHits)
	legacyregistry.MustRegister(healthCacheMisses)
}
