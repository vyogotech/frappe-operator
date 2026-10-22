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
	"os/exec"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func envByName(env []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range env {
		if env[i].Name == name {
			return &env[i]
		}
	}
	return nil
}

func TestBuildConfigPlan(t *testing.T) {
	tru := true
	var mfs int64 = 10485760
	sc := &vyogotechv1.SiteConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "sc1", Namespace: "t"},
		Spec: vyogotechv1.SiteConfigSpec{
			MaintenanceMode:        &tru,
			MaxFileSize:            &mfs,
			EncryptionKeySecretRef: &corev1.LocalObjectReference{Name: "enc"},
			CustomConfig:           map[string]string{"greeting": "hello world", "obj": `{"a":1}`},
			ObjectStorage: &vyogotechv1.ObjectStorageConfig{
				Bucket:               "b",
				EndpointURL:          "http://minio:9000",
				Region:               "us-east-1",
				CredentialsSecretRef: corev1.LocalObjectReference{Name: "s3creds"},
			},
		},
	}

	cmds, env, keys := buildConfigPlan(sc, "repro.localhost")
	joined := strings.Join(cmds, "\n")

	mustContain := []string{
		"set-config maintenance_mode 1",
		"set-config max_file_size 10485760",
		`set-config encryption_key "$CFG_ENCRYPTION_KEY"`,
		"set-config 'greeting' 'hello world'", // plain string (not JSON)
		"set-config -p 'obj' '{\"a\":1}'",     // JSON object -> parsed
		"/tmp/csbin/sudo",                     // no-op sudo shim for the install hook
		"install-app cloud_storage",           // objectStorage self-activates the app
		"--site 'repro.localhost' migrate",    // sync cloud_storage File custom fields
		"cloud_storage_settings",              // via the python assembler
		`--site 'repro.localhost'`,
	}
	for _, want := range mustContain {
		if !strings.Contains(joined, want) {
			t.Errorf("commands missing %q\n---\n%s", want, joined)
		}
	}

	// Encryption key must be sourced from the referenced Secret, never inlined.
	if e := envByName(env, "CFG_ENCRYPTION_KEY"); e == nil || e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil ||
		e.ValueFrom.SecretKeyRef.Name != "enc" || e.ValueFrom.SecretKeyRef.Key != "encryption_key" {
		t.Errorf("CFG_ENCRYPTION_KEY env not sourced from secret enc/encryption_key: %+v", e)
	}

	// S3 credentials from the secret; base JSON carries non-secret fields + defaulted folder.
	if e := envByName(env, "CS_ACCESS_KEY"); e == nil || e.ValueFrom == nil || e.ValueFrom.SecretKeyRef.Name != "s3creds" || e.ValueFrom.SecretKeyRef.Key != "access_key" {
		t.Errorf("CS_ACCESS_KEY not from s3creds/access_key: %+v", e)
	}
	if e := envByName(env, "CS_SECRET"); e == nil || e.ValueFrom == nil || e.ValueFrom.SecretKeyRef.Key != "secret" {
		t.Errorf("CS_SECRET not from s3creds/secret: %+v", e)
	}
	base := envByName(env, "CS_BASE_JSON")
	if base == nil || !strings.Contains(base.Value, `"folder":"repro.localhost"`) {
		t.Errorf("CS_BASE_JSON should default folder to the site domain: %+v", base)
	}
	if base == nil || strings.Contains(base.Value, "access_key") || strings.Contains(base.Value, "secret") {
		t.Errorf("CS_BASE_JSON must NOT contain credentials: %+v", base)
	}

	// Applied keys reported for status.
	for _, k := range []string{"maintenance_mode", "max_file_size", "encryption_key", "greeting", "obj", "cloud_storage_settings"} {
		found := false
		for _, ak := range keys {
			if ak == k {
				found = true
			}
		}
		if !found {
			t.Errorf("appliedKeys missing %q (got %v)", k, keys)
		}
	}
}

func TestBuildConfigPlan_Empty(t *testing.T) {
	sc := &vyogotechv1.SiteConfig{ObjectMeta: metav1.ObjectMeta{Name: "sc", Namespace: "t"}}
	cmds, _, keys := buildConfigPlan(sc, "s.local")
	if len(cmds) != 0 || len(keys) != 0 {
		t.Errorf("expected empty plan, got cmds=%v keys=%v", cmds, keys)
	}
}

func TestBuildConfigPlanSecretConfig(t *testing.T) {
	sc := &vyogotechv1.SiteConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "cp", Namespace: "vyogo-cloud"},
		Spec: vyogotechv1.SiteConfigSpec{
			SiteRef: &vyogotechv1.NamespacedName{Name: "cp"},
			SecretConfig: []vyogotechv1.SecretConfigEntry{
				{Key: "oidc_service_token", SecretKeyRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "platform-kc"}, Key: "OIDC_SERVICE_TOKEN"}},
				{Key: "agent_webhook_secret", SecretKeyRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "agent-webhook-secret"}, Key: "webhook-secret"}},
				// incomplete entry is skipped, not applied half-way
				{Key: "", SecretKeyRef: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "x"}, Key: "y"}},
			},
		},
	}
	cmds, env, keys := buildConfigPlan(sc, "fcloud-cp.vyogo.cloud")
	if len(keys) != 2 || keys[0] != "oidc_service_token" || keys[1] != "agent_webhook_secret" {
		t.Fatalf("applied keys = %v", keys)
	}
	e0 := envByName(env, "CFG_SECRET_0")
	if e0 == nil || e0.ValueFrom == nil || e0.ValueFrom.SecretKeyRef == nil ||
		e0.ValueFrom.SecretKeyRef.Name != "platform-kc" || e0.ValueFrom.SecretKeyRef.Key != "OIDC_SERVICE_TOKEN" {
		t.Fatalf("CFG_SECRET_0 not sourced from the Secret: %+v", e0)
	}
	if envByName(env, "CFG_SECRET_1") == nil {
		t.Fatalf("CFG_SECRET_1 missing")
	}
	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, `set-config -- 'oidc_service_token' "$CFG_SECRET_0"`) {
		t.Fatalf("expected set-config to read the env var, got:\n%s", joined)
	}
	if strings.Contains(joined, "platform-kc") || strings.Contains(joined, "webhook-secret") {
		t.Fatalf("secret names/values must not appear on the command line:\n%s", joined)
	}
}

func TestBuildConfigJobCarriesBenchPullSecrets(t *testing.T) {
	sc := &vyogotechv1.SiteConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "cp", Namespace: "vyogo-cloud"},
		Spec: vyogotechv1.SiteConfigSpec{
			SiteRef:      &vyogotechv1.NamespacedName{Name: "cp"},
			CustomConfig: map[string]string{"ignore_csrf": "1"},
		},
	}
	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "control-plane", Namespace: "vyogo-cloud"},
		Spec: vyogotechv1.FrappeBenchSpec{ImageConfig: &vyogotechv1.ImageConfig{
			Repository:  "ghcr.io/vyogotech/frappe-cloud-bench",
			Tag:         "version-16",
			PullSecrets: []corev1.LocalObjectReference{{Name: "ghcr-pull-secret"}},
		}},
	}
	job, _ := buildConfigJob(sc, bench, "fcloud-cp.vyogo.cloud", "ghcr.io/vyogotech/frappe-cloud-bench:version-16")
	if job == nil {
		t.Fatalf("expected a job")
	}
	ps := job.Spec.Template.Spec.ImagePullSecrets
	if len(ps) != 1 || ps[0].Name != "ghcr-pull-secret" {
		t.Fatalf("config Job must carry the bench image pull secrets, got %+v", ps)
	}
}

func TestBuildConfigJobHasBenchJobEnv(t *testing.T) {
	sc := &vyogotechv1.SiteConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "cp", Namespace: "vyogo-cloud"},
		Spec:       vyogotechv1.SiteConfigSpec{SiteRef: &vyogotechv1.NamespacedName{Name: "cp"}, CustomConfig: map[string]string{"ignore_csrf": "1"}},
	}
	job, _ := buildConfigJob(sc, &vyogotechv1.FrappeBench{}, "cp.example", "img")
	env := job.Spec.Template.Spec.Containers[0].Env
	// Without USER, `bench` dies at import with "No username set in the environment"
	// when the pod runs as a uid the image's /etc/passwd does not know.
	if e := envByName(env, "USER"); e == nil || e.Value != "frappe" {
		t.Fatalf("config Job must set USER=frappe, got %+v", e)
	}
	if e := envByName(env, "HOME"); e == nil || e.Value != "/home/frappe" {
		t.Fatalf("config Job must set HOME, got %+v", e)
	}
}

func TestJobsNeverOverwriteAppsTxtWithTheImageList(t *testing.T) {
	sc := &vyogotechv1.SiteConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "cp", Namespace: "vyogo-cloud"},
		Spec:       vyogotechv1.SiteConfigSpec{SiteRef: &vyogotechv1.NamespacedName{Name: "cp"}, CustomConfig: map[string]string{"k": "v"}},
	}
	job, _ := buildConfigJob(sc, &vyogotechv1.FrappeBench{}, "cp.example", "img")
	script := strings.Join(job.Spec.Template.Spec.Containers[0].Command, " ")
	if strings.Contains(script, "ls -1 apps > sites/apps.txt") {
		t.Fatalf("config Job overwrites apps.txt with the image list; SiteApp-installed apps would be dropped and their DocTypes deleted on migrate")
	}
	if !strings.Contains(script, "sites/apps/*/") {
		t.Fatalf("config Job must include apps installed on the shared volume in apps.txt")
	}
	if !strings.Contains(appsTxtSyncCmd, "ln -sf sites/apps.txt apps.txt") {
		t.Fatalf("bench root apps.txt link missing")
	}
}

func TestBuildCleanupJob_ScriptSyntax(t *testing.T) {
	sc := &vyogotechv1.SiteConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "sc1", Namespace: "default", Generation: 1},
		Spec:       vyogotechv1.SiteConfigSpec{SiteRef: &vyogotechv1.NamespacedName{Name: "site1"}},
	}
	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench1", Namespace: "default"},
	}

	job := buildCleanupJob(sc, bench, "site1.example.com", "img:latest", []string{"foo", "bar"})
	if job == nil {
		t.Fatal("expected non-nil cleanup job")
	}

	cmd := job.Spec.Template.Spec.Containers[0].Command
	if len(cmd) < 3 || cmd[0] != "bash" || cmd[1] != "-c" {
		t.Fatalf("unexpected container command: %v", cmd)
	}

	script := cmd[2]

	// 1. Verify bash syntax
	bashCmd := exec.Command("bash", "-n")
	bashCmd.Stdin = strings.NewReader(script)
	if out, err := bashCmd.CombinedOutput(); err != nil {
		t.Fatalf("cleanup job script has bash syntax errors: %v\nOutput:\n%s", err, string(out))
	}

	// 2. Extract and verify python script syntax with ast.parse
	const pyStart = "python3 - <<'EOF'\n"
	const pyEnd = "\nEOF"
	startIdx := strings.Index(script, pyStart)
	endIdx := strings.Index(script, pyEnd)
	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		t.Fatalf("failed to locate python heredoc in script:\n%s", script)
	}
	pyCode := script[startIdx+len(pyStart) : endIdx]

	pyCmd := exec.Command("python3", "-c", "import ast, sys; ast.parse(sys.stdin.read()); print('OK')")
	pyCmd.Stdin = strings.NewReader(pyCode)
	if out, err := pyCmd.CombinedOutput(); err != nil {
		t.Fatalf("cleanup job Python heredoc has syntax error: %v\nCode:\n%s\nOutput:\n%s", err, pyCode, string(out))
	}
}
