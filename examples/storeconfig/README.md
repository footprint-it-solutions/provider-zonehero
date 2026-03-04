# Credential Storage Configuration

This directory contains examples for managing credentials and storage configurations used by the ZoneHero provider.

## Overview

Crossplane allows storing credentials and connection secrets in various backends. This directory provides examples for:
1. **Kubernetes Secrets:** Storing ZoneHero API credentials directly in a Kubernetes Secret.
2. **AWS SSM Parameter Store:** Retrieving provider credentials from AWS SSM Parameter Store using External Secrets Operator.
3. **Vault:** Configuring an external Vault server for storing connection details (via `StoreConfig`).

## Files

### Secrets (Credentials)

- `secret.yaml`: Example of a Kubernetes Secret containing ZoneHero API credentials.
- `test-secret.yaml`: A template for creating a secret with placeholders.
- `external-secret.yaml`: Example of retrieving credentials from AWS SSM Parameter Store using External Secrets Operator.

**Usage (Kubernetes Secret):**

Create a file named `zonehero-creds.json` with your credentials:

```json
{
  "api_key": "YOUR_API_KEY",
  "aws_profile": "default",
  "aws_region": "eu-west-1",
  "partition": "aws"
}
```

Then create the secret in the `crossplane-system` namespace:

```bash
kubectl create secret generic zonehero-creds -n crossplane-system --from-file=credentials=zonehero-creds.json
```

Alternatively, you can apply the YAML manifest directly after populating the base64 encoded data or using stringData (as shown in the examples), but ensure you **do not commit real credentials to version control**.

**Usage (AWS SSM with External Secrets):**

If you prefer to manage credentials in AWS SSM Parameter Store:

1. Create a parameter in SSM (e.g., `/zonehero/provider/credentials`) with the JSON credential content.
2. Install and configure [External Secrets Operator](https://external-secrets.io/) in your cluster.
3. Ensure the ServiceAccount used by External Secrets (e.g., `external-secrets-sa`) is annotated with an IAM Role that has permissions to read the SSM parameter.

   Example annotation:
   ```yaml
   apiVersion: v1
   kind: ServiceAccount
   metadata:
     name: external-secrets-sa
     namespace: crossplane-system
     annotations:
       eks.amazonaws.com/role-arn: arn:aws:iam::<YOUR_AWS_ACCOUNT_ID>:role/external-secrets
   ```

4. Apply `external-secret.yaml` (ensure you update the AWS region and service account details):
   ```bash
   kubectl apply -f external-secret.yaml
   ```
   This will create a `zonehero-creds` Secret in the `crossplane-system` namespace, synced from SSM.

### StoreConfig (External Secret Store)

- `vault.yaml`: Example `StoreConfig` to configure Vault as an external secret store.

This resource tells Crossplane how to connect to a Vault server to write connection secrets for managed resources.

**Usage:**

Apply the `StoreConfig` to your cluster:

```bash
kubectl apply -f vault.yaml
```

**Note:** Ensure you have a valid Vault token stored in the referenced secret (`vault-token` in `crossplane-system` namespace) before applying.
