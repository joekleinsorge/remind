# Remind on Freshlab

These manifests prepare the deployment; their presence does not mean the app is installed. Verify Argo CD health and the authenticated endpoint after reconciliation.

## Prerequisites

1. Run the image publishing workflow and download its `freshlab-remind` artifact. Copy the artifact's `remind/` directory to `apps/remind` in `joekleinsorge/freshlab`. The artifact is pinned to the published digest and omits Namespace resources because Freshlab's apps project does not allow them. The root ApplicationSet discovers this directory and uses `CreateNamespace=true`.
2. Add a `remind` namespace to the SOPS-managed `freshlab-secrets` namespace list, with `istio.io/dataplane-mode: none` and restricted Pod Security labels. It must exist before the secrets reconcile.
3. Create a SOPS-encrypted Secret named `remind-sso` in namespace `remind`, with keys `clientSecret` and `cookieSecret`. Generate a unique client secret and a base64-encoded 32-byte cookie secret; never commit plaintext. Register its encrypted file with KSOPS.
4. Add a Dex static client in `platform/dex/values.yaml`:

   ```yaml
   - id: remind-sso
     name: Remind
     redirectURIs:
       - https://remind.kleinsorge.dev/oauth2/callback
     secretEnv: REMIND_SSO_CLIENT_SECRET
   ```

   Add the matching `REMIND_SSO_CLIENT_SECRET` to the SOPS-managed `dex-secrets` Secret in the `dex` namespace. Dex and OAuth2 Proxy must receive the same client secret. The existing `freshlab-admins` group grants access.
5. Optional email: provision the SOPS-managed `remind-sendgrid` Secret with `SENDGRID_API_KEY`, `SENDER_EMAIL`, `RECIPIENT_EMAIL`. Include `REMIND_EMAIL_TIME: "05:00"` in that Secret to enable the integrated scheduler. The Secret is optional; no email is sent by default.

Alternatively, use the [External Secrets manifests](../k8s/external-secrets/README.md) included in the publishing artifact. Configure the existing store and remote paths first, then give External Secrets sole ownership of the two target secrets. Keep the shared Dex client secret synchronized in the Dex namespace through its existing secret management.

To import without an upload pod, run `scripts/import-clippings.sh "My Clippings.txt"` from this repo, or the artifact's `import-clippings.sh`. It streams into the running application's import API through `kubectl exec`; existing reviews are kept. The script requires pod exec access.

The Daily email tab and authenticated `/api/status` endpoint report persisted outcomes and last provider acceptance. For direct operator inspection, copy `/data/delivery-status.json` through your normal volume tooling; it contains timestamps and highlight IDs, with no email credentials. An `in_progress` outcome after a restart is ambiguous and must be investigated before attempting a resend.

The repository's Renovate configuration requests digest pins for images and GitHub Actions. Enable the bot for this repository to receive updates. Freshlab must manage its own digest updates in the deployment repository; copying a new published artifact updates its pinned app image. Use `scripts/package-deployment.sh sha256:<digest> output-directory` to reproduce a bundle locally from a known published digest.

Keep important state protected by Freshlab's existing Longhorn backup policy or configure a VolSync ReplicationSource with a dedicated encrypted repository Secret. The legacy backup CronJob only copies review state on the same PVC and is not an off-node backup of the web library; do not rely on it for disaster recovery.

## Activate

Commit the intended state to Freshlab and freshlab-secrets, let Argo CD reconcile, then verify:

```sh
kubectl get application remind -n argocd
kubectl rollout status deployment/remind -n remind
kubectl get pvc,httproute -n remind
kubectl logs deployment/remind -c remind -n remind
curl -I https://remind.kleinsorge.dev/
```

Unauthenticated access must redirect to Dex. Log in as an administrator, import your clippings, mark one highlight remembered, restart the pod, and confirm the library and review remain. Verify a non-administrator cannot enter. Health checks intentionally bypass login; library and API routes do not.

The OAuth2 sidecar reaches Dex over cluster DNS. The NetworkPolicy admits inbound traffic from the Istio gateway namespace only. The application itself listens on pod loopback. No direct application Service bypasses authentication.

If an older `remind` CronJob exists, suspend it before starting the web app:

```sh
kubectl patch cronjob remind -n remind --type merge -p '{"spec":{"suspend":true}}'
```

Persist that suspension in the owning Git repository. The retained legacy manifest in this project is suspended and excluded from the web app Kustomization.

## Recovery

Use the UI's **Export library** for a portable JSON snapshot of highlights and review state. To restore a full volume backup, stop the deployment, restore all four JSON files under `/data`, preserve UID/GID 65532 access, then start one replica. Validate JSON before restoring. The export's `highlights` array maps to `library.json`, and its `state` object maps to `remind-state.json`; retain the backed-up `email-day.json` to avoid replaying a daily email and `delivery-status.json` to preserve delivery history.

`remind-data` has `Prune=false`; removing the application must not delete the library. The deployment uses `Recreate` and exactly one replica so two writers never share the state volume. Do not run manual email jobs against the active volume.
