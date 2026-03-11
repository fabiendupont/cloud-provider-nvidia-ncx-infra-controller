# Cloud Provider for NVIDIA Carbide

Kubernetes Cloud Controller Manager (CCM) for NVIDIA Carbide bare-metal infrastructure platform.

## Overview

This repository implements the Kubernetes Cloud Provider interface for NVIDIA Carbide, enabling native integration between Kubernetes and the NVIDIA Carbide bare-metal infrastructure platform.

The CCM binary is built using the standard `k8s.io/cloud-provider/app` framework and targets Kubernetes 1.34+.

### Implemented Interfaces

| Interface | Status | Description |
|-----------|--------|-------------|
| **InstancesV2** | Implemented | Node lifecycle management, metadata, addresses |
| **Zones** | Implemented | Zone and region mapping from Carbide site data |
| **LoadBalancer** | Not implemented | Use [MetalLB](https://metallb.universe.tf/) or kube-vip instead |
| **Routes** | Not implemented | Not applicable for bare-metal |

### What Does This Provider Do?

1. **Node Controller**: Manages node lifecycle
   - Initializes nodes with provider IDs (`nvidia-carbide://org/tenant/site/instance-id`)
   - Labels nodes with zone and region information derived from site location
   - Updates node addresses based on NVIDIA Carbide instance network interfaces
   - Removes nodes that have been terminated in NVIDIA Carbide
   - Sets instance type labels from the Carbide InstanceType name
   - Optionally labels nodes with machine health status

2. **Zone/Region Mapping**: Maps NVIDIA Carbide sites to Kubernetes topology
   - Queries the Site API for location data (country, state, city)
   - Zone: `{country}-{state}-{site-name}` (lowercase, hyphen-separated)
   - Region: `{country}-{state}` (lowercase, hyphen-separated)
   - Falls back to site-ID-based identifiers if location data is unavailable
   - Site lookups are cached to reduce API calls

3. **Instance Type Resolution**: Resolves machine types from the Carbide API
   - Looks up the InstanceType by ID from the instance metadata
   - Sets `node.kubernetes.io/instance-type` to the InstanceType name (e.g., `dgx-h100`)
   - Falls back to `nvidia-carbide-instance` if the lookup fails

4. **Node Address Classification**: Differentiates network interfaces
   - Physical interfaces (CIN/InfiniBand) are skipped as not Kubernetes-routable
   - The first non-physical interface's first IP is used as `NodeInternalIP`
   - Hostname is always added as `NodeHostName`

5. **Machine Health Labels** (optional): Exposes hardware health status
   - `nvidia-carbide.io/healthy`: `"true"` or `"false"`
   - `nvidia-carbide.io/health-alert-count`: number of active alerts
   - Enables external remediation controllers (e.g., RHWA NodeHealthCheck)

## Architecture

```
+----------------------------------------------------------+
|            Kubernetes Control Plane                       |
|  +----------------------------------------------------+  |
|  |   kube-controller-manager (built-in controllers)   |  |
|  +----------------------------------------------------+  |
|                                                          |
|  +----------------------------------------------------+  |
|  |   NVIDIA Carbide Cloud Controller Manager (CCM)    |  |
|  |   +------------------------------------------+     |  |
|  |   |  Node Controller                         |     |  |
|  |   |  - Initialize nodes with provider IDs    |     |  |
|  |   |  - Update node addresses                 |     |  |
|  |   |  - Set zone/region/instance-type labels   |     |  |
|  |   |  - Remove terminated nodes               |     |  |
|  |   +------------------------------------------+     |  |
|  +------------------+---------------------------------+  |
+---------------------+------------------------------------+
                      |
                      v
+----------------------------------------------------------+
|            Kubernetes API Server (Node Objects)           |
+----------------------------------------------------------+
                      |
                      v
+----------------------------------------------------------+
|         NVIDIA Carbide REST API                          |
|    Instance, Site, InstanceType, Machine APIs            |
+----------------------------------------------------------+
                      |
                      v
+----------------------------------------------------------+
|            NVIDIA Carbide Platform                       |
|       (Bare-Metal Infrastructure Management)             |
+----------------------------------------------------------+
```

## Dependencies

- **github.com/nvidia/bare-metal-manager-rest/sdk/standard** - Auto-generated REST API client
- **k8s.io/cloud-provider** - Kubernetes cloud provider framework
- **k8s.io/component-base** - Kubernetes component utilities

## Prerequisites

1. **Kubernetes cluster** (v1.34+) deployed on NVIDIA Carbide infrastructure
2. **NVIDIA Carbide API credentials**: endpoint URL, org name, token, site UUID, tenant UUID
3. **Control plane nodes** with network access to NVIDIA Carbide API

## Installation

### Option A: OLM (OpenShift)

Apply the File Based Catalog, then install from OperatorHub:

```bash
kubectl apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: nvidia-carbide-catalog
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: ghcr.io/fabiendupont/cloud-provider-nvidia-carbide-catalog:v0.1.0
  displayName: NVIDIA Carbide Cloud Provider
EOF
```

The operator appears in OperatorHub as **Cloud Provider NVIDIA Carbide**.

### Option B: Manual (kubectl)

```bash
# Build and push Docker image
make docker-build docker-push IMG=your-registry/cloud-provider-nvidia-carbide:latest

# Deploy RBAC and cloud controller manager
make deploy
```

### Configure Credentials

Create the cloud config secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: nvidia-carbide-cloud-config
  namespace: kube-system
stringData:
  cloud-config: |
    endpoint: "https://api.carbide.nvidia.com"
    orgName: "your-org-name"
    token: "your-api-token"
    siteId: "550e8400-e29b-41d4-a716-446655440000"
    tenantId: "660e8400-e29b-41d4-a716-446655440001"
```

### Configure Kubelet

Kubelets should be started with the cloud provider flag:

```bash
kubelet \
  --cloud-provider=external \
  --provider-id=nvidia-carbide://org/tenant/site/instance-id \
  ...
```

### Verify

```bash
kubectl get pods -n kube-system -l app=nvidia-carbide-cloud-controller-manager
kubectl get nodes -o custom-columns=NAME:.metadata.name,PROVIDER-ID:.spec.providerID
```

## Configuration Reference

### Cloud Config File

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `endpoint` | string | Yes | NVIDIA Carbide API endpoint URL |
| `orgName` | string | Yes | Organization name in NVIDIA Carbide |
| `token` | string | Yes | API authentication token |
| `siteId` | string | Yes | Site UUID where cluster is deployed |
| `tenantId` | string | Yes | Tenant UUID for the cluster |

### Environment Variables

Environment variables override cloud config file values:

- `NVIDIA_CARBIDE_ENDPOINT` - API endpoint
- `NVIDIA_CARBIDE_ORG_NAME` - Organization name
- `NVIDIA_CARBIDE_TOKEN` - Authentication token
- `NVIDIA_CARBIDE_SITE_ID` - Site UUID
- `NVIDIA_CARBIDE_TENANT_ID` - Tenant UUID

### Command Line Flags

The cloud controller manager accepts standard Kubernetes CCM flags:

```bash
--cloud-provider=nvidia-carbide    # Cloud provider name (required)
--cloud-config=/path/to/config     # Path to cloud config file
--use-service-account-credentials  # Use service account for cloud API
--leader-elect                     # Enable leader election (multi-replica)
--leader-elect-resource-name       # Leader election lock name
--v=2                              # Log verbosity level
```

## Usage

### Node Lifecycle

When a new node joins the cluster:

1. Kubelet starts with `--cloud-provider=external` and `--provider-id=nvidia-carbide://...`
2. CCM Node Controller detects the new node
3. CCM queries NVIDIA Carbide API for instance metadata
4. CCM updates node with:
   - Provider ID
   - Node addresses (`NodeInternalIP` from the first non-physical interface)
   - Instance type label from the Carbide InstanceType name
   - Zone labels (`topology.kubernetes.io/zone` = `{country}-{state}-{site-name}`)
   - Region labels (`topology.kubernetes.io/region` = `{country}-{state}`)
   - Health labels (`nvidia-carbide.io/healthy`, `nvidia-carbide.io/health-alert-count`)

When an instance is terminated in NVIDIA Carbide:

1. CCM periodically checks instance status
2. If instance is in "Terminating", "Terminated", or "Error" state
3. CCM marks the node as shutdown
4. Kubernetes evicts pods and eventually removes the node

### Zone-Aware Scheduling

With zone information from NVIDIA Carbide, you can use zone-aware features:

**Pod topology spread:**
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
spec:
  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: topology.kubernetes.io/zone
      whenUnsatisfiable: DoNotSchedule
      labelSelector:
        matchLabels:
          app: my-app
```

**Volume topology:**
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: local-storage
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
allowedTopologies:
  - matchLabelExpressions:
      - key: topology.kubernetes.io/zone
        values:
          - us-california-santa-clara-dc1
```

## Development

### Building

```bash
go build ./cmd/nvidia-carbide-cloud-controller-manager/   # Build binary
go test ./...                                              # Run tests
make docker-build                                          # Build Docker image
```

### Project Structure

```
cloud-provider-nvidia-carbide/
├── cmd/nvidia-carbide-cloud-controller-manager/  # CCM entry point (real binary)
├── pkg/cloudprovider/                            # Cloud provider implementation
│   ├── nvidia_carbide_cloud.go                   # Provider registration and client
│   ├── instances.go                              # InstancesV2 implementation
│   ├── zones.go                                  # Zones implementation
│   ├── health.go                                 # Machine health labels
│   └── loadbalancer.go                           # LoadBalancer (not implemented)
├── pkg/providerid/                               # Provider ID parsing
├── test/
│   ├── integration/                              # Integration tests (Ginkgo)
│   └── e2e/                                      # End-to-end tests
├── deploy/                                       # Kubernetes manifests
├── bundle/                                       # OLM bundle (CSV)
├── catalog/                                      # File Based Catalog for OLM
└── Dockerfile                                    # Container build
```

## Troubleshooting

### Nodes Don't Have Provider IDs

**Symptoms:**
- `kubectl get nodes -o yaml` shows `spec.providerID` is empty
- CCM logs show "node has no provider ID"

**Solutions:**
1. Ensure kubelet is started with `--cloud-provider=external`
2. Ensure kubelet is started with `--provider-id=nvidia-carbide://org/tenant/site/instance-id`
3. Verify the provider ID format matches NVIDIA Carbide instance IDs

### CCM Can't Connect to NVIDIA Carbide API

**Symptoms:**
- CCM logs show connection errors or authentication failures
- Nodes not being initialized with metadata

**Solutions:**
1. Verify cloud config credentials are correct
2. Check network connectivity from control plane to NVIDIA Carbide API
3. Verify API token has not expired
4. Check CCM logs for specific error messages

### Nodes Stuck in "NotReady" State

**Symptoms:**
- Nodes appear in cluster but remain "NotReady"
- CCM can't fetch instance metadata

**Solutions:**
1. Verify instance actually exists in NVIDIA Carbide (check instance UUID)
2. Check instance status in NVIDIA Carbide is not "Error" or "Terminating"
3. Verify siteID and tenantID in cloud config match instance location
4. Check instance has network interfaces with IP addresses

### Permission Errors

**Symptoms:**
- CCM logs show "forbidden" or permission errors
- CCM can't update node status

**Solutions:**
```bash
# Verify RBAC is deployed
kubectl get clusterrole system:cloud-controller-manager
kubectl get clusterrolebinding system:cloud-controller-manager

# Verify service account can update nodes
kubectl auth can-i update nodes \
  --as=system:serviceaccount:kube-system:cloud-controller-manager
```

## Comparison with Other Providers

| Feature | NVIDIA Carbide | AWS | Azure | GCP | OpenStack |
|---------|------------|-----|-------|-----|-----------|
| Node Management | Yes | Yes | Yes | Yes | Yes |
| Zone Support | Yes | Yes | Yes | Yes | Yes |
| Load Balancer | No (use MetalLB) | Yes | Yes | Yes | Yes |
| Routes | No | Yes | Yes | Yes | Yes |
| Bare Metal | Yes | No | No | No | Yes |

## License

Apache 2.0

## Related Projects

- [cluster-api-provider-nvidia-carbide](../cluster-api-provider-nvidia-carbide) - Cluster API provider for NVIDIA Carbide
- [machine-api-provider-nvidia-carbide](../machine-api-provider-nvidia-carbide) - OpenShift Machine API provider for NVIDIA Carbide
- [bare-metal-manager-rest](../bare-metal-manager-rest) - REST API and SDK

## Contributing

Contributions are welcome! Please submit issues and pull requests to the GitHub repository.

## References

- [Kubernetes Cloud Provider Documentation](https://kubernetes.io/docs/concepts/architecture/cloud-controller/)
- [Cloud Provider Interface](https://github.com/kubernetes/cloud-provider)
- [Developing Cloud Controller Manager](https://kubernetes.io/docs/tasks/administer-cluster/developing-cloud-controller-manager/)
