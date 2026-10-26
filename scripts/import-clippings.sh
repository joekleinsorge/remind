#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo 'Usage: scripts/import-clippings.sh "My Clippings.txt" [namespace]' >&2
  exit 2
fi

clippings_source=$1
remind_namespace=${2:-remind}
if [[ ! -f "$clippings_source" || ! -r "$clippings_source" ]]; then
  echo 'The clippings file must exist and be readable.' >&2
  exit 2
fi

# Stream through the pod's loopback API. The running server owns all disk writes.
kubectl exec -i --namespace "$remind_namespace" deployment/remind -c remind \
  -- /app/remind --import=- < "$clippings_source"
