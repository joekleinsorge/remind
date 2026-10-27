# Managed secrets

This optional overlay uses [External Secrets Operator](https://external-secrets.io/latest/api/externalsecret/) with the `external-secrets.io/v1` API. Install the operator and configure a secret store separately. Applying the base `k8s` directory does not require the operator.

Before using this overlay:

1. Replace `replace-with-your-store` in both manifests with your existing `ClusterSecretStore` name. For a namespaced store, use `kind: SecretStore` and ensure it exists in `remind`.
2. Create a remote `remind/sendgrid` object with properties `apiKey`, `senderEmail`, `recipientEmail`, and `emailTime`. Use a local `HH:MM` time such as `05:00` to enable daily delivery; an empty string disables it.
3. Create a remote `remind/sso` object with `clientSecret` and `cookieSecret`. The client secret must match the Dex client. Use a URL-safe base64-encoded 32-byte value for the cookie secret.
4. Adjust `remoteRef.key` and `property` if your provider uses different paths or field names. All referenced properties must exist. If email is not wanted, remove `sendgrid.yaml` from the overlay resources.

Render the configured deployment and its managed secrets together before applying it:

```sh
kubectl kustomize deploy/external-secrets
```

After reconciliation, verify readiness without printing secret values:

```sh
kubectl get externalsecret -n remind
kubectl wait --for=condition=Ready externalsecret/remind-sso -n remind --timeout=120s
kubectl wait --for=condition=Ready externalsecret/remind-sendgrid -n remind --timeout=120s
```

To render only the ExternalSecret resources, use `kubectl kustomize k8s/external-secrets`. The generated secrets have the names and keys already consumed by the deployment. Reloader restarts the application when a secret rotates. This overlay uses `creationPolicy: Owner`; choose one controller to own each target secret. Remove any competing SOPS/KSOPS resource from its owning configuration before migrating it. `deletionPolicy: Retain` retains a target when a remote secret disappears; deleting an owner ExternalSecret can still garbage-collect its target.

Freshlab's existing SOPS setup remains supported. The publishing artifact includes standalone `external-secrets/` manifests as an optional alternative; customize the store and include that directory in your GitOps configuration only when using External Secrets. Do not commit plaintext credentials or provider authentication material.
