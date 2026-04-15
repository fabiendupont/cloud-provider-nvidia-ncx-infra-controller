# Cloud Provider NVIDIA NCX Infrastructure Controller (CCM)

Kubernetes Cloud Controller Manager for NICo (NVIDIA NCX
Infrastructure Controller). Correlates OpenShift/K8s nodes with
NICo instances, provides zone/region mapping, and surfaces
machine health data as node labels, NodeConditions, and
bidirectional health event reporting.

## Build and test

```bash
go build ./...
go test ./... -v
# Integration tests (require envtest binaries)
go test ./test/integration/ -v
# E2E tests (require live NICo API)
NICO_API_ENDPOINT=https://... NICO_TOKEN=... go test ./test/e2e/ -v
```

## Key files

- `cmd/nico-cloud-controller-manager/main.go` — entry point
- `pkg/cloudprovider/nico_cloud.go` — provider registration,
  config, NicoClientInterface, SDK client wrapper
- `pkg/cloudprovider/instances.go` — InstancesV2 (metadata,
  exists, shutdown), zone resolution, site caching
- `pkg/cloudprovider/zones.go` — Zones interface
- `pkg/cloudprovider/health.go` — health labels from fault API
  or JSONB fallback, capability detection, site capability labels
- `pkg/cloudprovider/conditions.go` — NicoHealthy and
  NicoFaultRemediation NodeConditions, strategic merge patch
- `pkg/cloudprovider/retry.go` — generic retryDo[T] with
  exponential backoff and error classification
- `pkg/cloudprovider/metrics.go` — Prometheus metric definitions
- `pkg/cloudprovider/event_reporter.go` — node informer that
  reports Ready=False and cordon events back to NICo
- `pkg/providerid/providerid.go` — provider ID parsing
- `test/` — integration and e2e tests

## SDK

Uses `github.com/NVIDIA/ncx-infra-controller-rest` (official SDK,
local replace directive). Uses the SDK's generated `HealthAPI`,
`MetadataAPI`, `InstanceAPI`, `SiteAPI`, `MachineAPI`, and
`InstanceTypeAPI` clients. Auth via JWT bearer token in context.

## Configuration

YAML config (`--cloud-config`) with env var overrides:

| Field | Env var | Required |
|-------|---------|----------|
| `endpoint` | `NICO_ENDPOINT` | yes |
| `orgName` | `NICO_ORG_NAME` | yes |
| `token` | `NICO_TOKEN` | yes |
| `siteId` | `NICO_SITE_ID` | yes |
| `tenantId` | `NICO_TENANT_ID` | yes |
| `apiName` | `NICO_API_NAME` | no |
| `maxRetries` | `NICO_MAX_RETRIES` | no (default: 3) |
| `initialBackoffSeconds` | `NICO_INITIAL_BACKOFF_SECONDS` | no (default: 1) |

## Health integration

Health data flows bidirectionally between NICo and Kubernetes:

**NICo → K8s (inbound):**
- Labels: `nico.io/healthy`, `nico.io/health-alert-count`,
  `nico.io/fault-component`, `nico.io/fault-classification`,
  `nico.io/fault-state`
- NodeConditions: `NicoHealthy`, `NicoFaultRemediation`
- Dual-path: structured fault API (NEP-0007) with JSONB fallback
- Capability detection via `MetadataAPI.GetCapabilities()`
- 2-minute TTL cache on health data

**K8s → NICo (outbound):**
- Reports `Ready=False` and cordon transitions to NICo via
  `HealthAPI.IngestFaultEvent()`
- 10-minute deduplication window
- Feedback loop prevention: skips reporting when
  `nico.io/healthy=false` label is already set by the CCM
- Guarded behind `fault-management` capability check

## Retry and observability

All NICo API calls go through `retryDo[T]`:
- 429/502/503/504 → transient (retry with exponential backoff)
- 400/401/403/404 → terminal (no retry)

Prometheus metrics (subsystem `nico_ccm`):
- `api_latency_seconds` — histogram by endpoint
- `api_errors_total` — counter by endpoint and error type
- `nodes_managed` — gauge
- `nodes_unhealthy` — gauge
- `health_cache_hits_total` / `health_cache_misses_total`

## Current status

v0.2.0, alpha. NEP-0007 alignment complete. All 6 roadmap items
implemented with full test coverage.

## Design constraints

- Existing label-based health integration preserved
- All fault-management features guarded behind capability checks
  (graceful degradation if NICo doesn't support NEP-0007 yet)
- Follow existing code style (klog, sync.Map caching, etc.)
- CCM is stateless — no local persistence
- All changes must pass `go build ./...` and `go test ./...`
