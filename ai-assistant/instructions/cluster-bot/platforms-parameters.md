# Platforms, Architectures, and Parameters

## Supported Platforms (13)

| Platform | Description |
|----------|-------------|
| `aws` | Amazon Web Services (most common, default fallback) |
| `gcp` | Google Cloud Platform |
| `azure` | Microsoft Azure |
| `vsphere` | VMware vSphere |
| `metal` | Bare metal / IPI metal |
| `ovirt` | oVirt/Red Hat Virtualization |
| `openstack` | OpenStack |
| `hypershift-hosted` | HyperShift Hosted Control Planes (default for supported versions) |
| `nutanix` | Nutanix |
| `alibaba` | Alibaba Cloud |
| `hypershift-hosted-powervs` | HyperShift on IBM Power Virtual Server |
| `azure-stackhub` | Azure Stack Hub |

### Default Platform Selection:
- If no platform specified and version supports HyperShift: `hypershift-hosted`
- Otherwise: `aws`
- HyperShift requires `multi` architecture

## Supported Architectures (3)

| Architecture | Description |
|-------------|-------------|
| `amd64` | x86_64 (default for non-HyperShift) |
| `arm64` | ARM 64-bit |
| `multi` | Multi-architecture (required for HyperShift, default for HyperShift) |

### Architecture Notes:
- `hypershift-hosted` requires `multi` architecture
- Default is `amd64` for all other platforms

## Supported Parameters (30+)

### Networking
| Parameter | Description |
|-----------|-------------|
| `ovn` | OVN-Kubernetes networking (default) |
| `ovn-hybrid` | OVN-Kubernetes with hybrid networking |
| `sdn` | OpenShift SDN (legacy) |
| `kuryr` | Kuryr networking (OpenStack) |
| `proxy` | Proxy configuration |

### IP Version
| Parameter | Description |
|-----------|-------------|
| `ipv4` | IPv4 only (default) |
| `ipv6` | IPv6 only |
| `dualstack` | Dual-stack IPv4+IPv6 |
| `dualstack-primaryv6` | Dual-stack with IPv6 primary |

### Cluster Size
| Parameter | Description | Maps to |
|-----------|-------------|---------|
| `compact` | 3-node compact cluster | `SIZE_VARIANT=compact` |
| `large` | Large cluster | `SIZE_VARIANT=large` |
| `xlarge` | Extra-large cluster | `SIZE_VARIANT=xlarge` |
| `single-node` | Single-node OpenShift (SNO) | - |

### Security & Compliance
| Parameter | Description |
|-----------|-------------|
| `fips` | FIPS 140-2 mode |
| `private` | Private cluster |
| `rt` | Real-time kernel |
| `no-capabilities` | No additional capabilities |

### Installation
| Parameter | Description |
|-----------|-------------|
| `upi` | User-provisioned infrastructure |
| `preserve-bootstrap` | Keep bootstrap node (`OPENSHIFT_INSTALL_PRESERVE_BOOTSTRAP=true`) |

### Runtime
| Parameter | Description |
|-----------|-------------|
| `techpreview` | Technology Preview features |

### Infrastructure
| Parameter | Description |
|-----------|-------------|
| `mirror` | Mirror registry |
| `shared-vpc` | Shared VPC |
| `no-spot` | Disable spot instances |
| `virtualization-support` | Enable virtualization support |

### Zones
| Parameter | Description |
|-----------|-------------|
| `multi-zone` | Multi-availability zone |
| `multi-zone-techpreview` | Multi-zone with tech preview |

### Special
| Parameter | Description |
|-----------|-------------|
| `bundle` | Bundle parameter |
| `nfv` | Network Functions Virtualization |
| `test` | Test parameter |
| `static` | Static IP configuration |

## Supported Tests (9)

| Test Suite | Description |
|-----------|-------------|
| `e2e` | End-to-end conformance tests |
| `e2e-serial` | Serial end-to-end tests |
| `e2e-all` | All end-to-end tests |
| `e2e-disruptive` | Disruptive tests |
| `e2e-disruptive-all` | All disruptive tests |
| `e2e-builds` | Build-related tests |
| `e2e-image-ecosystem` | Image ecosystem tests |
| `e2e-image-registry` | Image registry tests |
| `e2e-network-stress` | Network stress tests |

## Supported Upgrade Tests (4)

| Upgrade Test | Description |
|-------------|-------------|
| `e2e-upgrade` | Standard upgrade test (default) |
| `e2e-upgrade-all` | All upgrade tests |
| `e2e-upgrade-partial` | Partial upgrade test |
| `e2e-upgrade-rollback` | Upgrade rollback test |

Upgrade tests are passed via `test=NAME` in options:
```
test upgrade 4.17 4.19 aws,test=e2e-upgrade-all
```

## Important: Not All Combinations Are Valid

Not all parameter + platform combinations have a backing prow job. If no matching job exists, the bot responds with `configuration error, unable to find prow job matching ...`. The combinations in the table below are known to work. For any other combination, advise the user it may not be supported and suggest they try it or use `workflow-launch` with a specific workflow from the openshift/release repository.

## Common Platform + Parameter Combinations

| Use Case | Command |
|----------|---------|
| Quick test cluster | `launch 4.19` (defaults to hypershift-hosted) |
| Standard AWS | `launch 4.19 aws` |
| ARM on GCP | `launch 4.19 gcp,arm64` |
| Compact FIPS | `launch 4.19 aws,compact,fips` |
| Private cluster | `launch 4.19 aws,private` |
| Bare metal SNO | `launch 4.19 metal,single-node` |
| IPv6 testing | `launch 4.19 aws,ipv6` |
| Tech preview | `launch 4.19 aws,techpreview` |
| Large GCP cluster | `launch 4.19 gcp,large` |
| Multi-zone Azure | `launch 4.19 azure,multi-zone` |
