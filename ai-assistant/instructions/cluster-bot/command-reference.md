# ci-chat-bot Command Reference

All commands are sent via DM to @cluster-bot in Slack.

## AI Assistant

### ask
```
ask <question>
```
Ask the AI assistant a question about cluster-bot commands, options, workflows, and more.

**Examples:**
- `ask how do I launch a cluster on GCP with FIPS?`
- `ask what platforms are supported?`
- `ask how do I use workflow-launch with custom parameters?`

## Cluster Launching

### launch
```
launch <image_or_version_or_prs> <options>
```
Launch an OpenShift cluster using a known image, version, or PR(s).

**Input formats for `<image_or_version_or_prs>`:**
- `nightly` - Latest OCP nightly build
- `ci` - Latest CI build
- `prerelease` - Latest pre-release build
- `4.19` - Major.minor version (resolves to next stable nightly)
- `4.19.0-0.nightly` - Specific stream name
- `4.19.0-0.ci` - CI stream
- Direct image pull spec from https://amd64.ocp.releases.ci.openshift.org
- `openshift/installer#123` - Launch from PR
- `4.19,openshift/installer#123,openshift/mco#456` - Version + multiple PRs

**Options** (comma-delimited):
- Platform: `aws`, `gcp`, `azure`, `vsphere`, `metal`, `ovirt`, `openstack`, `hypershift-hosted`, `nutanix`, `alibaba`, `hypershift-hosted-powervs`, `azure-stackhub`
- Architecture: `amd64` (default), `arm64`, `multi`
- Parameters: see "Platforms and Parameters" document

**Default behavior:** If no platform is specified, the bot defaults to `hypershift-hosted` for supported versions, otherwise `aws`. If no architecture is specified, defaults to `amd64` (or `multi` for hypershift-hosted).

**Examples:**
- `launch 4.19` - Default (hypershift-hosted, amd64)
- `launch 4.19 aws` - AWS, amd64
- `launch 4.19 gcp,arm64` - GCP, ARM
- `launch nightly aws,compact,fips` - Nightly on AWS, compact, FIPS
- `launch 4.19,openshift/installer#7160 gcp,techpreview` - PR testing
- `launch 4.19,openshift/installer#123,openshift/mco#456 aws,multi-zone` - Multi-PR

### workflow-launch
```
workflow-launch <name> <image_or_version_or_prs> <parameters>
```
Launch a cluster using a custom workflow from the openshift/release repository.

**Examples:**
- `workflow-launch openshift-e2e-gcp-windows-node 4.19 gcp`

## ROSA (Red Hat OpenShift Service on AWS)

### rosa create
```
rosa create <version> <duration>
```
Create a ROSA cluster. Only GA OpenShift versions are supported.

**Examples:**
- `rosa create 4.19 3h` - 3-hour cluster
  - `rosa create 4.18 8h` - 8-hour cluster (maximum)

### rosa lookup
```
rosa lookup <version>
```
Find OpenShift versions with the provided prefix that are supported in ROSA.

**Examples:**
- `rosa lookup 4.19`

### rosa describe
```
rosa describe <cluster_name>
```
Display details of a ROSA cluster.

**Examples:**
- `rosa describe s9h9g-9b6nj-x94`

## Testing

### test
```
test <name> <image_or_version_or_prs> <options>
```
Run a test suite from an image, release, or built PRs.

**Supported test suites:**
- `e2e` - End-to-end conformance tests
- `e2e-serial` - Serial end-to-end tests
- `e2e-all` - All end-to-end tests
- `e2e-disruptive` - Disruptive tests
- `e2e-disruptive-all` - All disruptive tests
- `e2e-builds` - Build-related tests
- `e2e-image-ecosystem` - Image ecosystem tests
- `e2e-image-registry` - Image registry tests
- `e2e-network-stress` - Network stress tests

**Examples:**
- `test e2e 4.19 aws` - Run e2e tests on AWS
- `test e2e-serial 4.19 gcp` - Serial tests on GCP

### test upgrade
```
test upgrade <from> <to> <options>
```
Run upgrade tests between two release images.

**Upgrade test options** (pass as `test=NAME` in options):
- `e2e-upgrade` (default)
- `e2e-upgrade-all`
- `e2e-upgrade-partial`
- `e2e-upgrade-rollback`

**Examples:**
- `test upgrade 4.17 4.19 aws` - Standard upgrade test
- `test upgrade 4.17 4.19 aws,test=e2e-upgrade-all` - All upgrade tests

### workflow-test
```
workflow-test <name> <image_or_version_or_prs> <parameters>
```
Run a test using a custom workflow.

**Examples:**
- `workflow-test openshift-e2e-gcp 4.19`

### workflow-upgrade
```
workflow-upgrade <name> <from_image_or_version_or_prs> <to_image_or_version_or_prs> <parameters>
```
Run a custom upgrade workflow.

**Examples:**
- `workflow-upgrade openshift-upgrade-azure-ovn 4.17 4.19 azure`

## Building

### build
```
build <version>,<pullrequest>
```
Create a new release image from one or more pull requests. Preserved for 12 hours.

**Examples:**
- `build 4.19,openshift/installer#123`
- `build 4.19,openshift/origin#49563,openshift/kubernetes#731`

### catalog build
```
catalog build <version>,<pullrequest> <bundle_name>
```
Create an operator bundle and catalog from a pull request.

**Examples:**
- `catalog build 4.19,openshift/aws-efs-csi-driver-operator#75 aws-efs-csi-driver-operator-bundle`

## Cluster Management

### list
```
list
```
See all active clusters.

### done
```
done
```
Terminate your running cluster.

### auth
```
auth
```
Get credentials for the cluster you most recently requested.

### refresh
```
refresh
```
Retry fetching credentials if the cluster is marked as failed.

### version
```
version
```
Report the bot version.

### lookup
```
lookup <image_or_version_or_prs> <architecture>
```
Get info about a version.

**Examples:**
- `lookup 4.19 arm64`

## MCE (Multi-Cluster Engine) - Private Commands

MCE commands require special authorization.

### mce create
```
mce create <image_or_version_or_prs> <duration> <platform>
```
Create a new cluster using Hive and MCE.

**Examples:**
- `mce create 4.16.7 6h aws`

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
