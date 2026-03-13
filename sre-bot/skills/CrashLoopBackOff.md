---
description: Runbook for diagnosing pods in CrashLoopBackOff state
---

# SRE Skill: CrashLoopBackOff Analysis

When a user asks "Why is my pod crashing?" or when a CrashLoopBackOff alert fires, follow these steps autonomously using the Kubernetes MCP server:

1. **Describe the Pod:** Run `kubectl describe pod <pod-name> -n <namespace>`. 
   - Check the `Events` section for immediate scheduling errors (like missing PVCs or failed probes).
2. **Check Current Logs:** Run `kubectl logs <pod-name> -n <namespace>`.
3. **Check Previous Logs:** Often the crash metadata is in the *previous* container invocation. Run `kubectl logs <pod-name> -n <namespace> --previous`.
4. **Analysis:** Summarize the root cause from the standard error output (e.g., Database Connection Failure, Missing Environment Variable).
5. **Action:** Formulate a concise summary of the issue. Do not guess—only report what you observed in the cluster.
