# Cloud Provider NVIDIA NCX Infrastructure Controller (CCM)

Kubernetes Cloud Controller Manager for NICo (NVIDIA NCX
Infrastructure Controller). Correlates OpenShift/K8s nodes with
NICo instances, provides zone/region mapping, and surfaces
machine health data as node labels and conditions.

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
- `pkg/cloudprovider/` — InstancesV2 + Zones implementation
- `pkg/providerid/providerid.go` — provider ID parsing
- `pkg/config/` — YAML + env var config loading
- `test/` — integration and e2e tests

## SDK

Uses `github.com/NVIDIA/ncx-infra-controller-rest v1.2.0`
(official SDK, no forks). Auth via JWT bearer token in context.

## Current status

v0.1.0, alpha. All 9 hardening tasks complete. Production-ready
for early adopter deployments.

---

## Work to do

The following changes align this CCM with the NCP reference
architecture vision, specifically NEP-0007 (Health Provider —
Fault Management and Service Events) in the NICo REST repo at
`~/Code/github.com/NVIDIA/ncx-infra-controller-rest/docs/enhancements/0007-fault-management-provider.md`.

### 1. Unify provider ID scheme

**Current:** `nico://org/tenant/site/instance-id`
**Target:** `nico://org/tenant/site/instance-id` (already correct)

The CCM already uses the `nico://` scheme. No change needed here,
but verify that the parsing in `pkg/providerid/providerid.go`
accepts provider IDs written by the CAPI and MAPI providers.
Both of those repos are being updated to use `nico://` as well
(they currently use `ncx-infra://` and `nvidia-carbide://`).

Acceptance criteria:
- `ParseProviderID("nico://org/tenant/site/uuid")` works
- Backward compat for legacy 3-segment format preserved
- Add test cases for the unified format

### 2. Replace JSONB health parsing with structured fault API

**Current:** The CCM reads `machine.health` JSONB via
`GetMachine()`, parses it into health labels (`nico.io/healthy`,
`nico.io/health-alert-count`), and caches with 2-minute TTL.

**Target:** Call the structured health events API instead:
`GET /v2/org/{org}/carbide/health/events?machine_id={id}&state=open`

This requires:
- A new method in the NICo client wrapper to call the health
  events endpoint (the SDK may not have this yet — if not,
  use a raw HTTP call via the existing client)
- Replace `getMachineHealth()` with a call to the health events
  endpoint
- Parse the response into health labels (same label names for
  backward compat)
- Add component-level labels when faults exist:
  `nico.io/fault-component: gpu` (from fault_event.component)
  `nico.io/fault-classification: gpu-xid-48` (from
  fault_event.classification)
  `nico.io/fault-state: remediating` (from fault_event.state)

If the health events endpoint is not yet available (NEP-0007 not
implemented), fall back to the current JSONB parsing. Use the
`/v2/org/{org}/carbide/capabilities` endpoint to check if
`fault-management` feature is available before calling the new API.

### 3. Surface faults as NodeConditions, not just labels

**Current:** Health data is exposed only as node labels. External
controllers (RHWA, NHC) must watch labels to detect faults.

**Target:** Set proper `NodeCondition` entries that RHWA/NHC can
natively watch:

```go
// When an open critical fault exists on the machine:
NodeCondition{
    Type:    "NicoHealthy",
    Status:  ConditionFalse,
    Reason:  "FaultDetected",
    Message: "GPU fault: gpu-xid-48 (remediating)",
}

// When remediation is in progress:
NodeCondition{
    Type:    "NicoFaultRemediation",
    Status:  ConditionTrue,
    Reason:  "RemediationInProgress",
    Message: "Automated GPU reset in progress, ETA 15 min",
}

// When healthy (no open faults):
NodeCondition{
    Type:    "NicoHealthy",
    Status:  ConditionTrue,
    Reason:  "NoFaults",
    Message: "All health checks passing",
}
```

Implementation:
- Add condition management in `InstanceMetadata()` or a new
  `syncNodeConditions()` method
- Use `node.Status.Conditions` patch (the CCM framework
  supports this via the node controller)
- Keep the existing labels for backward compatibility
- Add the conditions alongside the labels

### 4. Add error classification and retry logic

**Current:** No retry/backoff on NICo API failures. Relies on
context timeout only.

**Target:**
- Classify HTTP responses: 429/503/timeout → transient (retry
  with exponential backoff), 400/404 → terminal (log and skip)
- Add a simple retry wrapper around NICo API calls with
  configurable max retries (default: 3) and initial backoff
  (default: 1s)
- Add Prometheus counter `nico_ccm_api_errors_total` with labels
  for endpoint and error type (transient/terminal)

### 5. Add Prometheus metrics

**Current:** No custom Prometheus metrics.

**Target:**
- `nico_ccm_api_latency_seconds` (histogram) — NICo API call
  duration by endpoint
- `nico_ccm_api_errors_total` (counter) — errors by endpoint
  and type
- `nico_ccm_nodes_managed` (gauge) — number of nodes the CCM
  is managing
- `nico_ccm_nodes_unhealthy` (gauge) — nodes with open faults
- `nico_ccm_health_cache_hits_total` / `_misses_total` (counter)

### 6. Report K8s health events back to NICo

**Current:** One-way flow: NICo → CCM → labels/conditions.

**Target:** When the CCM detects that a node has been cordoned or
has `Ready=False` for reasons not already tracked by NICo, POST
to `POST /v2/org/{org}/carbide/health/events/ingest` with:

```json
{
  "source": "k8s-ccm",
  "severity": "warning",
  "component": "node",
  "classification": "node-not-ready",
  "message": "Node {name} has Ready=False, reason: {reason}",
  "machine_id": "{resolved from provider ID}",
  "detected_at": "{timestamp}"
}
```

This closes the health feedback loop: NICo sees K8s-level node
problems even if no NICo-level fault was detected.

Guard this behind the `fault-management` capability check. If the
feature is not available, skip silently.

## Design constraints

- Do not break existing label-based health integration
- All new features must be guarded behind capability checks
  (graceful degradation if NICo doesn't support NEP-0007 yet)
- Follow the existing code style (klog, sync.Map caching, etc.)
- Keep the CCM stateless — no local persistence
- All changes must pass `go build ./...` and `go test ./...`
