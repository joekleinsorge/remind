#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 || ! $1 =~ ^sha256:[a-f0-9]{64}$ ]]; then
  echo 'Usage: scripts/package-deployment.sh sha256:<64 hex characters> output-directory' >&2
  exit 2
fi

remind_digest=$1
bundle_dir=$2
repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
mkdir -p "$bundle_dir/remind" "$bundle_dir/external-secrets"
for resource in configmap deployment service route networkpolicy pvc; do
  cp "$repo_dir/k8s/$resource.yaml" "$bundle_dir/remind/"
done

# Freshlab validates the raw manifest as well as rendered Kustomize output.
sed "s|image: ghcr.io/joekleinsorge/remind[^[:space:]]*|image: ghcr.io/joekleinsorge/remind@$remind_digest|" \
  "$bundle_dir/remind/deployment.yaml" > "$bundle_dir/remind/deployment.yaml.tmp"
mv "$bundle_dir/remind/deployment.yaml.tmp" "$bundle_dir/remind/deployment.yaml"

# The Freshlab apps project creates namespaces separately and uses volume backups.
# Rebuild the image stanza so existing Renovate pins cannot override this build.
awk '
  /^images:/ {exit}
  /^  - namespace.yaml$/ {next}
  /^  - backup-cronjob.yaml$/ {next}
  {print}
' "$repo_dir/k8s/kustomization.yaml" > "$bundle_dir/remind/kustomization.yaml"
cat >> "$bundle_dir/remind/kustomization.yaml" <<EOF
images:
  - name: ghcr.io/joekleinsorge/remind
    digest: $remind_digest
EOF
cp "$repo_dir/k8s/external-secrets/"{sendgrid,sso}.yaml "$bundle_dir/external-secrets/"
cat > "$bundle_dir/external-secrets/kustomization.yaml" <<'EOF'
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - sendgrid.yaml
  - sso.yaml
EOF
cp "$repo_dir/scripts/import-clippings.sh" "$bundle_dir/"
kubectl kustomize "$bundle_dir/remind" > "$bundle_dir/remind-rendered.yaml"
