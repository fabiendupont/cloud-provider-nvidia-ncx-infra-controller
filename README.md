# Cloud Provider for NVIDIA Carbide

Kubernetes Cloud Controller Manager (CCM) for NVIDIA Carbide platform.

## Overview

This repository implements the Kubernetes Cloud Provider interface for NVIDIA Carbide, enabling native integration between Kubernetes and the NVIDIA Carbide bare-metal infrastructure platform.

### What is a Cloud Provider?

The Cloud Provider interface allows Kubernetes to interact with underlying cloud infrastructure. The Cloud Controller Manager (CCM) is a Kubernetes control plane component that runs cloud-specific control loops.

### What Does This Provider Do?

The NVIDIA Carbide Cloud Provider implements:

1. **Node Controller**: Manages node lifecycle
   - Initializes nodes with provider IDs (`nvidia-carbide://org/tenant/site/instance-id`)
   - Labels nodes with zone and region information
   - Updates node addresses based on NVIDIA Carbide instance network configuration
   - Removes nodes that have been terminated in NVIDIA Carbide

2. **Zone Support**: Provides zone and region information for scheduling
   - Maps NVIDIA Carbide sites to Kubernetes zones
   - Enables zone-aware pod scheduling and volume topology

3. **Instance Metadata**: Queries NVIDIA Carbide API for node/instance information
   - Checks if instances exist
   - Detects shutdown/terminated instances
   - Retrieves instance network configuration

**Note**: Load balancer and routes are not currently supported. Use external solutions like MetalLB or kube-vip for LoadBalancer services.

## Architecture

```
+----------------------------------------------------------+
|            Kubernetes Control Plane                      |
|  +----------------------------------------------------+  |
|  |   kube-controller-manager (built-in controllers)   |  |
|  +----------------------------------------------------+  |
|                                                          |
|  +----------------------------------------------------+  |
|  |   NVIDIA Carbide Cloud Controller Manager (CCM)        |  |
|  |   +------------------------------------------+     |  |
|  |   |  Node Controller                         |     |  |
|  |   |  - Initialize nodes with provider IDs    |     |  |
|  |   |  - Update node addresses                 |     |  |
|  |   |  - Remove terminated nodes               |     |  |
|  |   +------------------------------------------+     |  |
|  +------------------+---------------------------------+  |
+---------------------+------------------------------------+
                      | Watches Nodes
                      | Updates Node Status
                      v
+----------------------------------------------------------+
|            Kubernetes API Server                         |
|                   (Node Objects)                         |
+----------------------------------------------------------+
                      |
                      | Queries Instance Info
                      v
+----------------------------------------------------------+
|         NVIDIA Carbide REST API Client                       |
|         (github.com/NVIDIA/carbide-rest/client)          |
+----------------------------------------------------------+
                      |
                      v
+----------------------------------------------------------+
|            NVIDIA Carbide Platform            |
|       (Bare-Metal Infrastructure Management)             |
+----------------------------------------------------------+
```

## Dependencies

- **[github.com/NVIDIA/carbide-rest/client](../carbide-rest/client)** - Auto-generated REST API client
- **k8s.io/cloud-provider** - Kubernetes cloud provider framework
- **k8s.io/component-base** - Kubernetes component utilities

## Prerequisites

1. **Kubernetes cluster** (v1.28+) deployed on NVIDIA Carbide infrastructure
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
--cloud-provider=nvidia-carbide        # Cloud provider name (required)
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
   - Node addresses (InternalIP from NVIDIA Carbide interfaces)
   - Zone labels (`topology.kubernetes.io/zone`)
   - Region labels (`topology.kubernetes.io/region`)

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
          - nvidia-carbide-zone-site-123
```

## Development

### Building

```bash
make build          # Build binary
make test           # Run tests
make docker-build   # Build Docker image
make run            # Run locally (requires kubeconfig and cloud config)
```

### Release Artifacts

```bash
# OLM bundle image
make bundle-build bundle-push

# FBC catalog image
make catalog-build catalog-push
```

### Project Structure

```
cloud-provider-nvidia-carbide/
├── cmd/nvidia-carbide-cloud-controller-manager/  # CCM entry point
├── pkg/cloudprovider/                            # Cloud provider implementation
│   ├── nvidia_carbide_cloud.go                   # Main provider interface
│   ├── instances.go                              # InstancesV2 implementation
│   └── zones.go                                  # Zones implementation
├── pkg/providerid/                               # Provider ID parsing
├── deploy/                                       # Kubernetes manifests
│   ├── rbac/                                     # ServiceAccount, ClusterRole
│   └── manifests/                                # Deployment, Secret
├── bundle/                                       # OLM bundle (CSV)
├── catalog/                                      # File Based Catalog for OLM
├── config/                                       # Sample configurations
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

### High API Request Rate

**Symptoms:**
- NVIDIA Carbide API rate limiting errors
- CCM making excessive API calls

**Solutions:**
1. Increase sync period (default controller intervals)
2. Enable caching in cloud provider (if available)
3. Reduce node count or number of CCM replicas

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
- [carbide-rest](../carbide-rest) - REST API client library

## Contributing

Contributions are welcome! Please submit issues and pull requests to the GitHub repository.

## References

- [Kubernetes Cloud Provider Documentation](https://kubernetes.io/docs/concepts/architecture/cloud-controller/)
- [Cloud Provider Interface](https://github.com/kubernetes/cloud-provider)
- [Developing Cloud Controller Manager](https://kubernetes.io/docs/tasks/administer-cluster/developing-cloud-controller-manager/)
