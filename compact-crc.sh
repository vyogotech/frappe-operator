#!/bin/bash

# compact-crc.sh
# Script to scale down non-essential OpenShift CRC components to save memory.
# Note: This will cause the cluster status to appear 'Degraded'.

echo "🚀 Starting CRC memory optimization..."

# Helper function to scale down if deployment exists
scale_down() {
    local type=$1
    local name=$2
    local ns=$3
    echo "  - Scaling down $type/$name in $ns"
    oc scale "$type/$name" -n "$ns" --replicas=0 --timeout=5s 2>/dev/null || echo "    (Skipped: $name not found or already stopped)"
}

echo "📦 Stopping Console..."
scale_down deployment console openshift-console
scale_down deployment downloads openshift-console

echo "📊 Stopping Monitoring (this saves the most memory)..."
# Scale operator first so it doesn't undo our changes
scale_down deployment cluster-monitoring-operator openshift-monitoring
scale_down statefulset prometheus-k8s openshift-monitoring
scale_down statefulset alertmanager-main openshift-monitoring
scale_down deployment grafana openshift-monitoring
scale_down deployment prometheus-adapter openshift-monitoring
scale_down deployment kube-state-metrics openshift-monitoring
scale_down deployment openshift-state-metrics openshift-monitoring
scale_down deployment telemeter-client openshift-monitoring
scale_down deployment thanos-querier openshift-monitoring

echo "🛡️ Stopping Telemetry and Samples..."
scale_down deployment insights-operator openshift-insights
scale_down deployment cluster-samples-operator openshift-cluster-samples-operator
scale_down deployment marketplace-operator openshift-marketplace

# Optional: Image Registry
# echo "🖼️ Stopping Image Registry..."
# scale_down deployment image-registry openshift-image-registry

echo "✨ Done! Use 'oc get pods -A' to verify pods are terminating."
echo "💡 To restore a service, run: oc scale deployment/<name> -n <ns> --replicas=1"
