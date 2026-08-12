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
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFrappeClient_Authenticate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/method/login" {
			if r.FormValue("usr") == "Administrator" && r.FormValue("pwd") == "secretpass" {
				http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-session-id"})
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message": "Logged In"}`))
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
					w.Write([]byte(`{"data": {"name": "TestRole"}}`))
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
				return
			}
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"data": {"name": "TestRole"}}`))
				return
			}
		}
		if r.URL.Path == "/api/resource/Role" && r.Method == http.MethodPost {
			roleExists = true
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"data": {"name": "TestRole"}}`))
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
					w.Write([]byte(`{"data": {"name": "test@example.com"}}`))
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
				return
			}
		}
		if r.URL.Path == "/api/resource/User" && r.Method == http.MethodPost {
			userExists = true
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"data": {"name": "test@example.com"}}`))
			return
		}
		if r.URL.Path == "/api/method/frappe.core.doctype.user.user.generate_keys" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"message": {"api_key": "key123", "api_secret": "secret456"}}`))
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
