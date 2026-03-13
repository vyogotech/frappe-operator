---
description: Runbook for diagnosing OOMKilled pods
---

# SRE Skill: OOMKilled Analysis

When a pod is continuously restarting and the exit reason is `OOMKilled`:

1. **Verify State:** Use `kubectl get pod <pod-name> -n <namespace> -o yaml` and check `status.containerStatuses` to confirm the `reason` is `OOMKilled`.
2. **Check Limits:** Analyze the `resources.limits.memory` configured for the container.
3. **Resolution:** Since the pod exceeded its hard memory limit, the Linux OOM killer terminated it.
4. **Action:** Recommend the SRE team increase the `resources.limits.memory` for that specific container, or suggest evaluating the application code for memory leaks.
