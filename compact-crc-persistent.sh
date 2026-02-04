#!/bin/bash

# compact-crc-persistent.sh
# Script to scale down non-essential OpenShift CRC components AND prevent them from restarting.

echo "🚀 Starting PERSISTENT CRC memory optimization..."

# 1. Scale down Monitoring Operator first (so it doesn't fight back)
echo "📊 Disabling Monitoring..."
oc scale deployment/cluster-monitoring-operator -n openshift-monitoring --replicas=0 2>/dev/null

# 2. Set ManagementState to Removed for specific operators
echo "📦 Setting Console to Removed..."
oc patch console.operator.openshift.io cluster --type=merge -p '{"spec": {"managementState": "Removed"}}' 2>/dev/null

echo "🛡️ Setting Samples to Removed..."
oc patch configs.samples.operator.openshift.io cluster --type=merge -p '{"spec": {"managementState": "Removed"}}' 2>/dev/null

echo "🛒 Setting Marketplace to Unmanaged..."
# For Marketplace, we often just want to stop the pods
oc patch operatorhub cluster --type=merge -p '{"spec": {"sources": []}}' 2>/dev/null

# 3. Scale down everything else that might be left
echo "🧹 Cleaning up remaining pods..."
oc scale deployment/insights-operator -n openshift-insights --replicas=0 2>/dev/null
oc scale statefulset/prometheus-k8s -n openshift-monitoring --replicas=0 2>/dev/null
oc scale statefulset/alertmanager-main -n openshift-monitoring --replicas=0 2>/dev/null
oc scale deployment/grafana -n openshift-monitoring --replicas=0 2>/dev/null

echo "✨ Done! Operators are now in 'Removed' or 'Unmanaged' states."
echo "💡 To restore, set managementState back to 'Managed' and replicas to 1."
