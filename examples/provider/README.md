# Provider Configuration

This directory contains examples of `ProviderConfig` resources for the ZoneHero Crossplane provider.

## Overview

A `ProviderConfig` resource is used to configure the ZoneHero provider, specifically specifying the credentials required to authenticate with the ZoneHero API.

## Files

- `config.yaml`: A standard `ProviderConfig` that references a Kubernetes Secret named `zonehero-creds` in the `crossplane-system` namespace.
- `config-test-partition.yaml`: An example `ProviderConfig` for testing with a specific partition, referencing `zonehero-creds-test-partition`.

## Usage

1. **Create the Credentials Secret:**
   Before applying a `ProviderConfig`, ensure you have created the corresponding Kubernetes Secret containing your ZoneHero credentials. See the `../storeconfig` directory for examples.

2. **Apply the ProviderConfig:**
   Apply the configuration to your cluster:
   ```bash
   kubectl apply -f config.yaml
   ```

   Or for the test partition configuration:
   ```bash
   kubectl apply -f config-test-partition.yaml
   ```

## Configuration Reference

The `ProviderConfig` spec allows you to specify where the provider should look for credentials.

```yaml
apiVersion: zonehero.footprintit.net/v1beta1
kind: ProviderConfig
metadata:
  name: zonehero-provider
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: zonehero-creds
      key: credentials
