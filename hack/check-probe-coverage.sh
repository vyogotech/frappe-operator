#!/usr/bin/env bash
# Every CRD this operator ships must be exercised by the probe app
# (github.com/vyogotech/frappe-operator-probe): each Kind needs a manifest
# under the probe's manifests/ and a phase in probe.py. Stubs that are not
# implemented yet are listed, with a reason, in hack/probe-coverage-exceptions.txt.
#
#   hack/check-probe-coverage.sh [path-to-probe-checkout]   (default ../frappe-operator-probe)
set -euo pipefail
cd "$(dirname "$0")/.."
PROBE_DIR=${1:-${PROBE_DIR:-../frappe-operator-probe}}
EXCEPTIONS=hack/probe-coverage-exceptions.txt

if [ ! -d "$PROBE_DIR/manifests" ]; then
  echo "probe checkout not found at $PROBE_DIR (git clone https://github.com/vyogotech/frappe-operator-probe)" >&2
  exit 2
fi

kinds=$(grep -h '^    kind:' config/crd/bases/*.yaml | awk '{print $2}' | sort -u)
covered=$(grep -h '^kind:' "$PROBE_DIR"/manifests/*.yaml | awk '{print $2}' | sort -u)

missing=()
for k in $kinds; do
  if grep -qx "$k" <<<"$covered"; then
    echo "ok       $k"
  elif grep -qE "^$k([[:space:]]|$)" "$EXCEPTIONS"; then
    echo "excepted $k  ($(grep -E "^$k[[:space:]]" "$EXCEPTIONS" | sed -E "s/^$k[[:space:]]+//"))"
  else
    missing+=("$k")
  fi
done

if [ ${#missing[@]} -gt 0 ]; then
  echo
  echo "ERROR: CRDs with no probe coverage: ${missing[*]}" >&2
  echo "Add manifests/NN-<kind>.yaml and a phase in probe.py to vyogotech/frappe-operator-probe," >&2
  echo "or (for an unimplemented scaffold) list the Kind with a reason in $EXCEPTIONS." >&2
  exit 1
fi
echo "all CRDs are covered by the probe"
