/*
Copyright 2023 Vyogo Technologies.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// shellQuote wraps a value in single quotes so it is passed to bench verbatim, safe
// against shell metacharacters (admin-authored SiteConfig values still shouldn't be
// able to break out of the set-config argument).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// looksLikeJSON reports whether a CustomConfig value should be applied with `set-config -p`
// (parsed as an object/array/number/bool) rather than as a bare string.
func looksLikeJSON(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	var v interface{}
	if json.Unmarshal([]byte(t), &v) != nil {
		return false
	}
	// A bare word like `foo` isn't valid JSON, so only objects/arrays/numbers/bools/null
	// reach here; strings (quoted) also parse — treat those as JSON too so intent is kept.
	return true
}

// buildConfigPlan returns the ordered shell commands that apply the SiteConfig to
// site_config.json (via `bench set-config`), the env vars they need (secret-sourced
// values are injected via env, never inlined), and the list of keys applied.
func buildConfigPlan(sc *vyogotechv1.SiteConfig, domain string) (cmds []string, env []corev1.EnvVar, appliedKeys []string) {
	base := "bench --site " + shellQuote(domain) + " set-config"

	if sc.Spec.MaintenanceMode != nil {
		v := "0"
		if *sc.Spec.MaintenanceMode {
			v = "1"
		}
		cmds = append(cmds, base+" maintenance_mode "+v)
		appliedKeys = append(appliedKeys, "maintenance_mode")
	}

	if sc.Spec.MaxFileSize != nil {
		cmds = append(cmds, base+" max_file_size "+strconv.FormatInt(*sc.Spec.MaxFileSize, 10))
		appliedKeys = append(appliedKeys, "max_file_size")
	}

	if ref := sc.Spec.EncryptionKeySecretRef; ref != nil && ref.Name != "" {
		env = append(env, corev1.EnvVar{
			Name: "CFG_ENCRYPTION_KEY",
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: *ref, Key: "encryption_key",
			}},
		})
		cmds = append(cmds, base+` encryption_key "$CFG_ENCRYPTION_KEY"`)
		appliedKeys = append(appliedKeys, "encryption_key")
	}

	// CustomConfig — sorted for deterministic Job specs.
	custom := make([]string, 0, len(sc.Spec.CustomConfig))
	for k := range sc.Spec.CustomConfig {
		custom = append(custom, k)
	}
	sort.Strings(custom)
	for _, k := range custom {
		val := sc.Spec.CustomConfig[k]
		if looksLikeJSON(val) {
			cmds = append(cmds, base+" -p "+shellQuote(k)+" "+shellQuote(val))
		} else {
			cmds = append(cmds, base+" "+shellQuote(k)+" "+shellQuote(val))
		}
		appliedKeys = append(appliedKeys, k)
	}

	// ObjectStorage -> cloud_storage_settings. Non-secret fields are baked into a base
	// JSON (via env, so no shell quoting of user values); the access key/secret come from
	// the referenced Secret via env; a tiny python step merges them and calls set-config.
	if os := sc.Spec.ObjectStorage; os != nil {
		folder := os.Folder
		if folder == "" {
			folder = domain
		}
		expiration := int64(120)
		if os.PresignedURLExpirySeconds != nil {
			expiration = *os.PresignedURLExpirySeconds
		}
		akKey := os.AccessKeyKey
		if akKey == "" {
			akKey = "access_key"
		}
		skKey := os.SecretKeyKey
		if skKey == "" {
			skKey = "secret"
		}
		baseJSON, _ := json.Marshal(map[string]interface{}{
			"region":           os.Region,
			"endpoint_url":     os.EndpointURL,
			"bucket":           os.Bucket,
			"folder":           folder,
			"expiration":       expiration,
			"use_legacy_paths": 0,
		})
		env = append(env,
			corev1.EnvVar{Name: "CS_DOMAIN", Value: domain},
			corev1.EnvVar{Name: "CS_BASE_JSON", Value: string(baseJSON)},
			corev1.EnvVar{Name: "CS_ACCESS_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: os.CredentialsSecretRef, Key: akKey,
			}}},
			corev1.EnvVar{Name: "CS_SECRET", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: os.CredentialsSecretRef, Key: skKey,
			}}},
		)
		// cloud_storage's after_install hook shells out to `sudo apt-get` to ensure its OS
		// packages. Those are already baked into the bench image, but there is no sudo in
		// a Job pod — so drop a no-op sudo shim on PATH to let the hook complete cleanly.
		cmds = append(cmds, `mkdir -p /tmp/csbin && printf '#!/bin/sh\nexit 0\n' > /tmp/csbin/sudo && chmod +x /tmp/csbin/sudo && export PATH="/tmp/csbin:$PATH"`)
		// Activate the app on the site (it is baked into the bench image but must be
		// installed per site). Tolerate "already installed" so re-applies are idempotent,
		// then migrate to sync cloud_storage's File custom fields (its file-version child
		// table) — without this, the File upload hook raises and offload fails.
		cmds = append(cmds,
			"bench --site "+shellQuote(domain)+" install-app cloud_storage || echo 'cloud_storage already installed'",
			"bench --site "+shellQuote(domain)+" migrate",
		)
		cmds = append(cmds, `python3 -c 'import os,json,subprocess;`+
			`c=json.loads(os.environ["CS_BASE_JSON"]);`+
			`c["access_key"]=os.environ["CS_ACCESS_KEY"];c["secret"]=os.environ["CS_SECRET"];`+
			`subprocess.run(["bench","--site",os.environ["CS_DOMAIN"],"set-config","-p","cloud_storage_settings",json.dumps(c)],check=True)'`)
		appliedKeys = append(appliedKeys, "cloud_storage_settings")
	}

	return cmds, env, appliedKeys
}

// buildConfigJob assembles the Job that runs the config plan against the site's bench
// PVC. Returns nil if there is nothing to apply.
func buildConfigJob(sc *vyogotechv1.SiteConfig, bench *vyogotechv1.FrappeBench, domain, benchImage string) (*batchv1.Job, []string) {
	cmds, env, appliedKeys := buildConfigPlan(sc, domain)
	if len(cmds) == 0 {
		return nil, nil
	}

	script := "set -e\n" + strings.Join(cmds, "\n") + "\n"
	jobName := fmt.Sprintf("%s-apply-%d", sc.Name, sc.Generation)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)
	backoff := int32(3)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: sc.Namespace,
			Labels:    map[string]string{"app": "frappe", "siteconfig": sc.Name},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "frappe", "siteconfig": sc.Name}},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{{
						Name:            "config-runner",
						Image:           benchImage,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"bash", "-c", script},
						Env:             env,
						// Match the bench deployments' PVC layout: site data lives under the
						// "frappe-sites" subPath (assets under "frappe-sites/assets"). Mounting
						// the wrong subPath yields an empty sites dir and bench can't find
						// apps.txt / the site.
						VolumeMounts: []corev1.VolumeMount{
							{Name: "sites", MountPath: "/home/frappe/frappe-bench/sites", SubPath: "frappe-sites"},
							{Name: "sites", MountPath: "/home/frappe/frappe-bench/sites/assets", SubPath: "frappe-sites/assets"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name: "sites",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
						},
					}},
				},
			},
		},
	}
	return job, appliedKeys
}
