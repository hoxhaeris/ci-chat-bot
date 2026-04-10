# ci-chat-bot Help Categories

Type `help` in DM to @cluster-bot for an overview.
Type `help <category>` for detailed help on a specific topic.

Available categories: launch, rosa, test, build, manage, mce

## Help: AI Assistant

### ask
```
ask <question>
```
Ask the AI assistant a question about cluster-bot commands, options, workflows, and more.

**Examples:**
- `ask how do I launch a cluster on GCP with FIPS?`
- `ask what platforms are supported?`

## Help: Launch

### Quick Reference - Common Configurations:
- Basic AWS: `launch 4.19 aws`
- Compact cluster: `launch 4.19 aws,compact`
- ARM on GCP: `launch 4.19 gcp,arm64`
- Secure cluster: `launch 4.19 aws,fips,private`
- Test environment: `launch 4.19 metal,compact,techpreview`

### Important Notes:
- Must contain an OpenShift version
- Options can be omitted (defaults: `hypershift-hosted`, `amd64`)
- Options is a comma-delimited list including platform, architecture, and variants
- Order doesn't matter except for readability

### Option Guidelines:
1. Start with platform (aws, gcp, etc.)
2. Add architecture if not default (arm64, multi)
3. Add networking if needed (sdn, kuryr, ipv6)
4. Add size/features last (compact, fips, techpreview)
5. Bot will validate combinations and inform about conflicts

### Examples (Simple to Complex):
- `launch 4.19` - Default: latest 4.19 on hypershift-hosted amd64
- `launch nightly aws` - Latest nightly build
- `launch 4.19 gcp,arm64` - Different platform + architecture
- `launch ci azure,compact,ovn` - CI build + size + networking
- `launch 4.19 aws,arm64,fips,private` - Security-focused cluster
- `launch 4.19.0-0.nightly metal,single-node,techpreview` - Advanced config
- `launch 4.19,openshift/installer#123 vsphere,multi,techpreview` - PR testing

## Help: ROSA

### rosa create
```
rosa create <version> <duration>
```
Create a ROSA cluster on AWS with automatic teardown.

### rosa lookup
```
rosa lookup <version>
```
Find OpenShift versions with the provided prefix that are supported in ROSA.

### rosa describe
```
rosa describe <cluster_name>
```
Get detailed information about a ROSA cluster.

### Common Options:
- Duration: `1h`, `3h`, `8h` (maximum 8 hours)
- Versions: Latest stable releases
- Automatic cleanup after expiration

### Examples:
- `rosa create 4.19 3h` - 3-hour cluster
- `rosa create 4.18 8h` - 8-hour cluster (maximum)
- `rosa lookup 4.19` - Find supported ROSA versions
- `rosa describe my-cluster` - Cluster details

## Help: Test

### test
```
test <name> <image_or_version_or_prs> <options>
```

### Available Test Suites:
- `e2e` - End-to-end conformance tests
- `e2e-serial` - Serial end-to-end tests
- `e2e-all` - All end-to-end tests
- `e2e-disruptive` - Disruptive tests
- `e2e-disruptive-all` - All disruptive tests
- `e2e-builds` - Build-related tests
- `e2e-image-ecosystem` - Image ecosystem tests
- `e2e-image-registry` - Image registry tests
- `e2e-network-stress` - Network stress tests

### test upgrade
```
test upgrade <from> <to> <options>
```

### Upgrade Test Options (pass as `test=NAME`):
- `e2e-upgrade` (default)
- `e2e-upgrade-all`
- `e2e-upgrade-partial`
- `e2e-upgrade-rollback`

### Workflow Commands:
- `workflow-test <name> <image> <parameters>` - Test using custom workflows
- `workflow-upgrade <name> <from> <to> <parameters>` - Custom upgrade workflows

### Examples:
- `test e2e 4.19 aws` - Run e2e tests on AWS
- `test e2e-serial 4.19 gcp` - Run serial tests
- `test upgrade 4.17 4.19 aws` - Test upgrade path
- `test upgrade 4.17 4.19 aws,test=e2e-upgrade-all` - All upgrade tests
- `workflow-test openshift-e2e-gcp 4.19` - Run GCP workflow
- `workflow-upgrade openshift-upgrade-azure-ovn 4.17 4.19 azure` - Custom upgrade

## Help: Build

### build
```
build <version>,<organization>/<repository>#<pr_number>
```

### catalog build
```
catalog build <version>,<organization>/<repository>#<pr_number> <bundle_name>
```

### Examples:
- `build 4.19,openshift/installer#123`
- `build 4.19,openshift/origin#49563,openshift/kubernetes#731,openshift/machine-api-operator#831`
- `catalog build 4.19,openshift/aws-efs-csi-driver-operator#84 aws-efs-csi-driver-operator-bundle`

## Help: Manage

### list
Show active clusters (all or for specific user).

### done
Terminate and cleanup cluster.

### auth
Get cluster credentials and connection info.

### refresh
Retry fetching cluster credentials in case of an error.

### lookup
```
lookup <version specifier>
```
Find version corresponding to version specifier.

### version
Show cluster-bot version information.

## Help: MCE (Private)

MCE commands require special authorization.

### mce create
```
mce create <image_or_version_or_prs> <duration> <platform>
```
Create a new cluster using Hive and MCE.

**Example:** `mce create 4.16.7 6h aws`

### mce auth
```
mce auth <name>
```
Print kubeconfig and kubeadmin password for specified MCE cluster.

### mce delete
```
mce delete <cluster_name>
```
Delete a previously created MCE cluster.

### mce list
```
mce list <all>
```
List active MCE clusters. Append `all` to list clusters for all users.

### mce lookup
```
mce lookup
```
List available versions for MCE clusters.
