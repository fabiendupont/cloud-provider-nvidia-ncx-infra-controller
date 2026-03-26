# Cloud Provider for NCX Infrastructure Controller (NICo)

Kubernetes Cloud Controller Manager (CCM) for NVIDIA NCX Infrastructure Controller bare-metal platform.

## Overview

This CCM integrates Kubernetes with the NICo bare-metal platform. It implements the following cloud provider interfaces:

| Interface | Status | Description |
|-----------|--------|-------------|
| **InstancesV2** | Implemented | Node lifecycle, metadata, addresses, health labels |
| **Zones** | Implemented | Zone and region mapping from NICo site location data |
| **LoadBalancer** | Not implemented | Use [MetalLB](https://metallb.universe.tf/) or kube-vip |
| **Routes** | Not implemented | Not applicable for bare-metal |

### Node Metadata Behavior

- **Instance type**: Resolved from the NICo InstanceType API. Fallback: `nico-instance`.
- **Zone/region**: Derived from Site location data as `{country}-{state}-{site-name}` / `{country}-{state}`. Site lookups are cached.
- **Addresses**: The first IP from the first non-physical network interface is set as `NodeInternalIP`. Physical interfaces (CIN/InfiniBand) are skipped.
- **Health labels**: `nico.io/healthy` (`"true"` / `"false"`) and `nico.io/health-alert-count` (number of active alerts). Health lookups are cached with a 2-minute TTL.

### Provider ID Format

```
nico://org/tenant/site/instance-id
```

Legacy 3-segment format (`nico://org/site/instance-id`) is also supported for backward compatibility.

## Building

```bash
# Build binary
go build ./cmd/nico-cloud-controller-manager/

# Build container image
docker build -t ghcr.io/fabiendupont/cloud-provider-nvidia-ncx-infra-controller:latest .
```

## Deployment

### 1. Apply RBAC

```bash
kubectl apply -f deploy/rbac/serviceaccount.yaml
kubectl apply -f deploy/rbac/clusterrole.yaml
kubectl apply -f deploy/rbac/clusterrolebinding.yaml
```

### 2. Create the cloud-config Secret

Copy and edit the example:

```bash
cp deploy/secret.yaml.example deploy/secret.yaml
# Edit deploy/secret.yaml with your NICo API credentials
kubectl apply -f deploy/secret.yaml
```

### 3. Deploy the CCM

```bash
kubectl apply -f deploy/manifests/deployment.yaml
```

### 4. Configure Kubelet

Kubelets must be started with:

```bash
kubelet \
  --cloud-provider=external \
  --provider-id=nico://org/tenant/site/instance-id \
  ...
```

### 5. Verify

```bash
kubectl get pods -n kube-system -l app=nico-cloud-controller-manager
kubectl get nodes -o custom-columns=NAME:.metadata.name,PROVIDER-ID:.spec.providerID
```

## Configuration

### Cloud Config File (YAML)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `endpoint` | string | Yes | NICo API endpoint URL |
| `orgName` | string | Yes | Organization name |
| `token` | string | Yes | API authentication token |
| `siteId` | string | Yes | Site UUID where cluster is deployed |
| `tenantId` | string | Yes | Tenant UUID |

### Environment Variable Overrides

Environment variables take precedence over the config file:

| Variable | Config Field |
|----------|-------------|
| `NICO_ENDPOINT` | `endpoint` |
| `NICO_ORG_NAME` | `orgName` |
| `NICO_TOKEN` | `token` |
| `NICO_SITE_ID` | `siteId` |
| `NICO_TENANT_ID` | `tenantId` |

## Node Lifecycle

When a new node joins the cluster:

1. Kubelet starts with `--cloud-provider=external` and `--provider-id=nico://...`
2. CCM detects the uninitialized node
3. CCM queries the NICo API for instance metadata
4. CCM sets node addresses, instance type, zone/region labels, and health labels
5. CCM removes the `node.cloudprovider.kubernetes.io/uninitialized` taint

When an instance is terminated in NICo:

1. CCM periodically checks instance status
2. If status is `Terminating`, `Terminated`, or `Error`, the node is marked as shutdown
3. Kubernetes evicts pods and removes the node

## Development

```bash
go test ./pkg/... ./test/integration/...      # Unit and integration tests
go test -tags=e2e ./test/e2e/...              # E2E tests (requires live NICo API)
go vet ./...                                   # Static analysis
```

### Project Structure

```
cloud-provider-nvidia-ncx-infra-controller/
├── cmd/nico-cloud-controller-manager/           # CCM entry point
├── pkg/cloudprovider/                           # Cloud provider implementation
│   ├── nico_cloud.go                            # Provider registration and client
│   ├── instances.go                             # InstancesV2 implementation
│   ├── zones.go                                 # Zones implementation
│   └── health.go                                # Machine health labels and caching
├── pkg/providerid/                              # Provider ID parsing
├── test/
│   ├── integration/                             # Integration tests (Ginkgo)
│   └── e2e/                                     # End-to-end tests
├── deploy/                                      # Kubernetes manifests
│   ├── manifests/deployment.yaml
│   ├── rbac/
│   └── secret.yaml.example
├── Dockerfile
└── go.mod
```

## License

Apache 2.0
