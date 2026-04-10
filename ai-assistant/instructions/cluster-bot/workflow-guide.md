# Workflow Guide

## Overview

Workflows allow you to use custom CI workflows from the openshift/release repository to launch clusters, run tests, or perform upgrades. They provide more flexibility than the standard commands.

## Available Commands

### workflow-launch
```
workflow-launch <name> <image_or_version_or_prs> <parameters>
```
Launch a cluster using a specific workflow. The workflow must be defined in the openshift/release repository's workflow configuration.

### workflow-test
```
workflow-test <name> <image_or_version_or_prs> <parameters>
```
Run tests using a specific workflow.

### workflow-upgrade
```
workflow-upgrade <name> <from_image_or_version_or_prs> <to_image_or_version_or_prs> <parameters>
```
Run custom upgrade tests using a specific workflow.

## Parameters

The platform and architecture for workflow commands are determined by the workflow configuration in `openshift/release`, not by user input. The `<parameters>` field accepts double-quoted key=value pairs:

```
workflow-launch <name> <image_or_version_or_prs> "KEY1=VALUE1","KEY2=VALUE2"
```

For nested parameters within a single variable (semicolon-separated):
```
workflow-launch <name> <image_or_version_or_prs> "CLUSTER_PROFILE_VARIABLES=KEY1=VAL1;KEY2=VAL2"
```

## Examples

### Basic workflow launch:
```
workflow-launch openshift-e2e-gcp-windows-node 4.19 gcp
```

### Workflow test:
```
workflow-test openshift-e2e-gcp 4.19
```

### Workflow upgrade:
```
workflow-upgrade openshift-upgrade-azure-ovn 4.17 4.19 azure
```

### With custom parameters:
```
workflow-launch my-custom-workflow 4.19 "CUSTOM_PARAM=value"
```

### With nested parameters:
```
workflow-launch my-workflow 4.19 "CLUSTER_PROFILE_VARIABLES=FEATURE_SET=TechPreviewNoUpgrade;FEATURE_GATES=ExternalCloudProvider=true"
```

## Adding New Workflows

To add a new workflow:
1. Create a PR to the openshift/release repository
2. Add the workflow to `core-services/ci-chat-bot/workflows-config.yaml`
3. The workflow must define the platform and optionally the architecture

## Important Notes

- Workflows must be registered in the openshift/release repository before they can be used
- Each workflow has a specific platform and architecture defined
- The bot validates that the requested workflow exists in the configuration
- If a workflow is not found, the bot will show available workflows
- HyperShift hosted workflows automatically use the `multi` architecture
