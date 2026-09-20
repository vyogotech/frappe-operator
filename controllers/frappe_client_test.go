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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFrappeClient_Authenticate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/method/login" {
			if r.FormValue("usr") == "Administrator" && r.FormValue("pwd") == "secretpass" {
				http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-session-id"})
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"message": "Logged In"}`))
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	ctx := context.Background()
	client := NewFrappeClient(ts.URL, "Administrator", "secretpass")
	err := client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("expected successful auth, got error: %v", err)
	}
	if client.SID != "test-session-id" {
		t.Errorf("expected sid test-session-id, got %s", client.SID)
	}
}

func TestFrappeClient_EnsureRole(t *testing.T) {
	roleExists := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/method/login" {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-sid"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/resource/Role/TestRole" {
			if r.Method == http.MethodGet {
				if roleExists {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"data": {"name": "TestRole"}}`))
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
				return
			}
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data": {"name": "TestRole"}}`))
				return
			}
		}
		if r.URL.Path == "/api/resource/Role" && r.Method == http.MethodPost {
			roleExists = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data": {"name": "TestRole"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	ctx := context.Background()
	client := NewFrappeClient(ts.URL, "Administrator", "pass")
	// Test Creation
	err := client.EnsureRole(ctx, "TestRole", true, false)
	if err != nil {
		t.Fatalf("EnsureRole create failed: %v", err)
	}

	// Test Update
	err = client.EnsureRole(ctx, "TestRole", true, false)
	if err != nil {
		t.Fatalf("EnsureRole update failed: %v", err)
	}
}

func TestFrappeClient_EnsureUser_And_GenerateAPIKeys(t *testing.T) {
	userExists := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/method/login" {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-sid"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/resource/User/test%40example.com" || r.URL.Path == "/api/resource/User/test@example.com" {
			if r.Method == http.MethodGet {
				if userExists {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"data": {"name": "test@example.com"}}`))
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
				return
			}
		}
		if r.URL.Path == "/api/resource/User" && r.Method == http.MethodPost {
			userExists = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data": {"name": "test@example.com"}}`))
			return
		}
		if r.URL.Path == "/api/method/frappe.core.doctype.user.user.generate_keys" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message": {"api_key": "key123", "api_secret": "secret456"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	ctx := context.Background()
	client := NewFrappeClient(ts.URL, "Administrator", "pass")

	err := client.EnsureUser(ctx, "test@example.com", "Test", "User", "System User", []string{"System Manager"}, false)
	if err != nil {
		t.Fatalf("EnsureUser failed: %v", err)
	}

	apiKey, apiSecret, err := client.GenerateAPIKeys(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("GenerateAPIKeys failed: %v", err)
	}

	if apiKey != "key123" || apiSecret != "secret456" {
		t.Errorf("expected key123/secret456, got %s/%s", apiKey, apiSecret)
	}
}

// Prompt-named doctypes need the name in the create payload, and Frappe's
// webhook event field is `webhook_docevent`.
func TestFrappeClient_CreatePayloadsCarryNameAndDocevent(t *testing.T) {
	var bodies = map[string]map[string]interface{}{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/method/login":
			_, _ = w.Write([]byte(`{"message":"Logged In"}`))
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost:
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			bodies[r.URL.Path] = body
			_, _ = w.Write([]byte(`{"data":{}}`))
		}
	}))
	defer ts.Close()
	c := NewFrappeClient(ts.URL, "Administrator", "pw")
	if err := c.EnsureClientScript(context.Background(), "banner", "Probe Record", "x", true); err != nil {
		t.Fatalf("client script: %v", err)
	}
	if err := c.EnsureWebhook(context.Background(), "hook", "Probe Record", "on_update", "http://x", "JSON", true); err != nil {
		t.Fatalf("webhook: %v", err)
	}
	if bodies["/api/resource/Client Script"]["name"] != "banner" {
		t.Fatalf("client script payload lacks name: %v", bodies["/api/resource/Client Script"])
	}
	wh := bodies["/api/resource/Webhook"]
	if wh["name"] != "hook" || wh["webhook_docevent"] != "on_update" {
		t.Fatalf("webhook payload must carry name and webhook_docevent: %v", wh)
	}
}

// User Permission is hash-named: existence is decided by a filtered list, an
// existing one is updated by its real name, and a 409 on create is success.
func TestFrappeClient_EnsureUserPermissionIsIdempotent(t *testing.T) {
	posts, puts := 0, 0
	exists := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/method/login":
			_, _ = w.Write([]byte(`{"message":"Logged In"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/resource/User Permission":
			if !strings.Contains(r.URL.RawQuery, "filters=") {
				t.Errorf("lookup must filter by fields, got %s", r.URL.RawQuery)
			}
			if exists {
				_, _ = w.Write([]byte(`{"data":[{"name":"u06fsnkpl1"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[]}`))
			}
		case r.Method == http.MethodPost:
			posts++
			exists = true
			_, _ = w.Write([]byte(`{"data":{"name":"u06fsnkpl1"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/resource/User Permission/u06fsnkpl1":
			puts++
			_, _ = w.Write([]byte(`{"data":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()
	c := NewFrappeClient(ts.URL, "Administrator", "pw")
	for i := 0; i < 2; i++ {
		if err := c.EnsureUserPermission(context.Background(), "u@x", "Probe Record", "seed-00002", true); err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
	}
	if posts != 1 || puts != 1 {
		t.Fatalf("want one create then one update by real name, got posts=%d puts=%d", posts, puts)
	}
}
