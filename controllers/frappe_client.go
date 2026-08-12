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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FrappeClient handles communication with Frappe Framework REST APIs
type FrappeClient struct {
	BaseURL    string
	HostHeader string
	Username   string
	Password   string
	HTTPClient *http.Client
	SID        string
}

// NewFrappeClient creates a new Frappe API client
func NewFrappeClient(baseURL, username, password string) *FrappeClient {
	baseURL = strings.TrimSuffix(baseURL, "/")
	return &FrappeClient{
		BaseURL:  baseURL,
		Username: username,
		Password: password,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type frappeGenericResponse struct {
	Message json.RawMessage `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (c *FrappeClient) newRequest(ctx context.Context, method, urlStr string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return nil, err
	}
	if c.HostHeader != "" {
		req.Host = c.HostHeader
		req.Header.Set("Host", c.HostHeader)
	}
	if c.SID != "" {
		req.Header.Set("Cookie", fmt.Sprintf("sid=%s", c.SID))
	}
	return req, nil
}

// Authenticate logs in using Administrator credentials and sets session cookie
func (c *FrappeClient) Authenticate(ctx context.Context) error {
	loginURL := fmt.Sprintf("%s/api/method/login", c.BaseURL)
	data := url.Values{}
	data.Set("usr", c.Username)
	data.Set("pwd", c.Password)

	req, err := c.newRequest(ctx, http.MethodPost, loginURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "sid" && cookie.Value != "" && cookie.Value != "Guest" {
			c.SID = cookie.Value
			break
		}
	}
	return nil
}

// EnsureRole creates or updates a Frappe Role
func (c *FrappeClient) EnsureRole(ctx context.Context, roleName string, deskAccess, disabled bool) error {
	if c.SID == "" {
		if err := c.Authenticate(ctx); err != nil {
			return err
		}
	}

	roleURL := fmt.Sprintf("%s/api/resource/Role/%s", c.BaseURL, url.PathEscape(roleName))
	req, err := c.newRequest(ctx, http.MethodGet, roleURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	deskAccessVal := 0
	if deskAccess {
		deskAccessVal = 1
	}
	disabledVal := 0
	if disabled {
		disabledVal = 1
	}

	payload := map[string]interface{}{
		"role_name":   roleName,
		"desk_access": deskAccessVal,
		"disabled":    disabledVal,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		// Create Role
		createURL := fmt.Sprintf("%s/api/resource/Role", c.BaseURL)
		req, err := c.newRequest(ctx, http.MethodPost, createURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		createResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer createResp.Body.Close()
		if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(createResp.Body)
			return fmt.Errorf("failed to create role %s: status %d: %s", roleName, createResp.StatusCode, string(body))
		}
		return nil
	}

	if resp.StatusCode == http.StatusOK {
		// Update Role
		req, err := c.newRequest(ctx, http.MethodPut, roleURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		updateResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer updateResp.Body.Close()
		if updateResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(updateResp.Body)
			return fmt.Errorf("failed to update role %s: status %d: %s", roleName, updateResp.StatusCode, string(body))
		}
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed to check role %s: status %d: %s", roleName, resp.StatusCode, string(body))
}

// EnsureUser creates or updates a Frappe User and assigns roles
func (c *FrappeClient) EnsureUser(ctx context.Context, email, firstName, lastName, userType string, roles []string, sendPasswordReset bool) error {
	if c.SID == "" {
		if err := c.Authenticate(ctx); err != nil {
			return err
		}
	}

	if userType == "" {
		userType = "System User"
	}

	userURL := fmt.Sprintf("%s/api/resource/User/%s", c.BaseURL, url.PathEscape(email))
	req, err := c.newRequest(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Build role assignments slice for Frappe child table
	roleObjects := make([]map[string]string, 0, len(roles))
	for _, role := range roles {
		roleObjects = append(roleObjects, map[string]string{
			"role": role,
		})
	}

	sendWelcomeEmail := 0
	if sendPasswordReset {
		sendWelcomeEmail = 1
	}

	payload := map[string]interface{}{
		"email":              email,
		"first_name":         firstName,
		"last_name":          lastName,
		"user_type":          userType,
		"roles":              roleObjects,
		"send_welcome_email": sendWelcomeEmail,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		// Create User
		createURL := fmt.Sprintf("%s/api/resource/User", c.BaseURL)
		req, err := c.newRequest(ctx, http.MethodPost, createURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		createResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer createResp.Body.Close()

		if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(createResp.Body)
			return fmt.Errorf("failed to create user %s: status %d: %s", email, createResp.StatusCode, string(body))
		}
		return nil
	}

	if resp.StatusCode == http.StatusOK {
		// Update User
		req, err := c.newRequest(ctx, http.MethodPut, userURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		updateResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer updateResp.Body.Close()

		if updateResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(updateResp.Body)
			return fmt.Errorf("failed to update user %s: status %d: %s", email, updateResp.StatusCode, string(body))
		}
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed to check user %s: status %d: %s", email, resp.StatusCode, string(body))
}

// GenerateAPIKeys generates an API key and secret for a user
func (c *FrappeClient) GenerateAPIKeys(ctx context.Context, email string) (string, string, error) {
	if c.SID == "" {
		if err := c.Authenticate(ctx); err != nil {
			return "", "", err
		}
	}

	keysURL := fmt.Sprintf("%s/api/method/frappe.core.doctype.user.user.generate_keys", c.BaseURL)
	payload := map[string]string{
		"user": email,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, err := c.newRequest(ctx, http.MethodPost, keysURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to generate keys for user %s: status %d: %s", email, resp.StatusCode, string(bodyBytes))
	}

	var res frappeGenericResponse
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", "", err
	}

	var keyData struct {
		APIKey    string `json:"api_key"`
		APISecret string `json:"api_secret"`
	}

	target := res.Message
	if len(target) == 0 {
		target = res.Data
	}

	if err := json.Unmarshal(target, &keyData); err != nil {
		return "", "", fmt.Errorf("failed to parse generated key response: %w", err)
	}

	return keyData.APIKey, keyData.APISecret, nil
}

// InstallApp installs a Frappe application on the site via REST API
func (c *FrappeClient) InstallApp(ctx context.Context, appName string) error {
	if c.SID == "" {
		if err := c.Authenticate(ctx); err != nil {
			return err
		}
	}

	installURL := fmt.Sprintf("%s/api/method/frappe.installer.install_app", c.BaseURL)
	payload := map[string]string{
		"name": appName,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, err := c.newRequest(ctx, http.MethodPost, installURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to install app %s: status %d: %s", appName, resp.StatusCode, string(body))
	}

	return nil
}

// UninstallApp uninstalls a Frappe application from the site via REST API
func (c *FrappeClient) UninstallApp(ctx context.Context, appName string) error {
	if c.SID == "" {
		if err := c.Authenticate(ctx); err != nil {
			return err
		}
	}

	uninstallURL := fmt.Sprintf("%s/api/method/frappe.installer.uninstall_app", c.BaseURL)
	payload := map[string]string{
		"app_name": appName,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, err := c.newRequest(ctx, http.MethodPost, uninstallURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to uninstall app %s: status %d: %s", appName, resp.StatusCode, string(body))
	}

	return nil
}

// EnsureCustomField creates or updates a Frappe Custom Field
func (c *FrappeClient) EnsureCustomField(ctx context.Context, dt, fieldname, label, fieldtype, options, insertAfter string, reqd, readOnly, hidden int, defaultVal string) error {
	if c.SID == "" {
		if err := c.Authenticate(ctx); err != nil {
			return err
		}
	}

	name := fmt.Sprintf("%s-%s", dt, fieldname)
	customFieldURL := fmt.Sprintf("%s/api/resource/Custom Field/%s", c.BaseURL, url.PathEscape(name))
	req, err := c.newRequest(ctx, http.MethodGet, customFieldURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload := map[string]interface{}{
		"dt":           dt,
		"fieldname":    fieldname,
		"label":        label,
		"fieldtype":    fieldtype,
		"options":      options,
		"insert_after": insertAfter,
		"reqd":         reqd,
		"read_only":    readOnly,
		"hidden":       hidden,
		"default":      defaultVal,
	}
	payloadBytes, _ := json.Marshal(payload)

	if resp.StatusCode == http.StatusNotFound {
		createURL := fmt.Sprintf("%s/api/resource/Custom Field", c.BaseURL)
		req, err := c.newRequest(ctx, http.MethodPost, createURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		createResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer createResp.Body.Close()
		if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(createResp.Body)
			return fmt.Errorf("failed to create custom field %s: status %d: %s", name, createResp.StatusCode, string(body))
		}
		return nil
	}

	if resp.StatusCode == http.StatusOK {
		req, err := c.newRequest(ctx, http.MethodPut, customFieldURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		updateResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer updateResp.Body.Close()
		if updateResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(updateResp.Body)
			return fmt.Errorf("failed to update custom field %s: status %d: %s", name, updateResp.StatusCode, string(body))
		}
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed to check custom field %s: status %d: %s", name, resp.StatusCode, string(body))
}

// EnsurePropertySetter creates or updates a Frappe Property Setter
func (c *FrappeClient) EnsurePropertySetter(ctx context.Context, docType, fieldName, property, propertyType, value string) error {
	if c.SID == "" {
		if err := c.Authenticate(ctx); err != nil {
			return err
		}
	}

	if propertyType == "" {
		propertyType = "Check"
	}

	name := fmt.Sprintf("%s-%s-%s", docType, fieldName, property)
	if fieldName == "" {
		name = fmt.Sprintf("%s-main-%s", docType, property)
	}

	propURL := fmt.Sprintf("%s/api/resource/Property Setter/%s", c.BaseURL, url.PathEscape(name))
	req, err := c.newRequest(ctx, http.MethodGet, propURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload := map[string]interface{}{
		"doc_type":      docType,
		"field_name":    fieldName,
		"property":      property,
		"property_type": propertyType,
		"value":         value,
	}
	if fieldName != "" {
		payload["doctype_or_field"] = "DocField"
	} else {
		payload["doctype_or_field"] = "DocType"
	}
	payloadBytes, _ := json.Marshal(payload)

	if resp.StatusCode == http.StatusNotFound {
		createURL := fmt.Sprintf("%s/api/resource/Property Setter", c.BaseURL)
		req, err := c.newRequest(ctx, http.MethodPost, createURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		createResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer createResp.Body.Close()
		if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(createResp.Body)
			return fmt.Errorf("failed to create property setter %s: status %d: %s", name, createResp.StatusCode, string(body))
		}
		return nil
	}

	if resp.StatusCode == http.StatusOK {
		req, err := c.newRequest(ctx, http.MethodPut, propURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		updateResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer updateResp.Body.Close()
		if updateResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(updateResp.Body)
			return fmt.Errorf("failed to update property setter %s: status %d: %s", name, updateResp.StatusCode, string(body))
		}
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed to check property setter %s: status %d: %s", name, resp.StatusCode, string(body))
}

// EnsureServerScript creates or updates a Frappe Server Script
func (c *FrappeClient) EnsureServerScript(ctx context.Context, name, scriptType, referenceDoctype, eventType, apiMethod, script string, disabled bool) error {
	if c.SID == "" {
		if err := c.Authenticate(ctx); err != nil {
			return err
		}
	}

	disabledVal := 0
	if disabled {
		disabledVal = 1
	}

	scriptURL := fmt.Sprintf("%s/api/resource/Server Script/%s", c.BaseURL, url.PathEscape(name))
	req, err := c.newRequest(ctx, http.MethodGet, scriptURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload := map[string]interface{}{
		"name":              name,
		"script_type":       scriptType,
		"reference_doctype": referenceDoctype,
		"doctype_event":     eventType,
		"api_method":        apiMethod,
		"script":            script,
		"disabled":          disabledVal,
	}
	payloadBytes, _ := json.Marshal(payload)

	if resp.StatusCode == http.StatusNotFound {
		createURL := fmt.Sprintf("%s/api/resource/Server Script", c.BaseURL)
		req, err := c.newRequest(ctx, http.MethodPost, createURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		createResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer createResp.Body.Close()
		if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(createResp.Body)
			return fmt.Errorf("failed to create server script %s: status %d: %s", name, createResp.StatusCode, string(body))
		}
		return nil
	}

	if resp.StatusCode == http.StatusOK {
		req, err := c.newRequest(ctx, http.MethodPut, scriptURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		updateResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer updateResp.Body.Close()
		if updateResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(updateResp.Body)
			return fmt.Errorf("failed to update server script %s: status %d: %s", name, updateResp.StatusCode, string(body))
		}
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed to check server script %s: status %d: %s", name, resp.StatusCode, string(body))
}

// EnsureClientScript creates or updates a Frappe Client Script
func (c *FrappeClient) EnsureClientScript(ctx context.Context, name, dt, script string, enabled bool) error {
	if c.SID == "" {
		if err := c.Authenticate(ctx); err != nil {
			return err
		}
	}

	enabledVal := 1
	if !enabled {
		enabledVal = 0
	}

	scriptURL := fmt.Sprintf("%s/api/resource/Client Script/%s", c.BaseURL, url.PathEscape(name))
	req, err := c.newRequest(ctx, http.MethodGet, scriptURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload := map[string]interface{}{
		"dt":      dt,
		"script":  script,
		"enabled": enabledVal,
	}
	payloadBytes, _ := json.Marshal(payload)

	if resp.StatusCode == http.StatusNotFound {
		createURL := fmt.Sprintf("%s/api/resource/Client Script", c.BaseURL)
		req, err := c.newRequest(ctx, http.MethodPost, createURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		createResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer createResp.Body.Close()
		if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(createResp.Body)
			return fmt.Errorf("failed to create client script %s: status %d: %s", name, createResp.StatusCode, string(body))
		}
		return nil
	}

	if resp.StatusCode == http.StatusOK {
		req, err := c.newRequest(ctx, http.MethodPut, scriptURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		updateResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer updateResp.Body.Close()
		if updateResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(updateResp.Body)
			return fmt.Errorf("failed to update client script %s: status %d: %s", name, updateResp.StatusCode, string(body))
		}
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed to check client script %s: status %d: %s", name, resp.StatusCode, string(body))
}

// EnsureWebhook creates or updates a Frappe Webhook
func (c *FrappeClient) EnsureWebhook(ctx context.Context, name, webhookDoctype, webhookEvent, requestUrl, requestStructure string, enabled bool) error {
	if c.SID == "" {
		if err := c.Authenticate(ctx); err != nil {
			return err
		}
	}

	enabledVal := 1
	if !enabled {
		enabledVal = 0
	}
	if requestStructure == "" {
		requestStructure = "JSON"
	}

	webhookURL := fmt.Sprintf("%s/api/resource/Webhook/%s", c.BaseURL, url.PathEscape(name))
	req, err := c.newRequest(ctx, http.MethodGet, webhookURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload := map[string]interface{}{
		"webhook_doctype":   webhookDoctype,
		"webhook_event":     webhookEvent,
		"request_url":       requestUrl,
		"request_structure": requestStructure,
		"enabled":           enabledVal,
	}
	payloadBytes, _ := json.Marshal(payload)

	if resp.StatusCode == http.StatusNotFound {
		createURL := fmt.Sprintf("%s/api/resource/Webhook", c.BaseURL)
		req, err := c.newRequest(ctx, http.MethodPost, createURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		createResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer createResp.Body.Close()
		if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(createResp.Body)
			return fmt.Errorf("failed to create webhook %s: status %d: %s", name, createResp.StatusCode, string(body))
		}
		return nil
	}

	if resp.StatusCode == http.StatusOK {
		req, err := c.newRequest(ctx, http.MethodPut, webhookURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		updateResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer updateResp.Body.Close()
		if updateResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(updateResp.Body)
			return fmt.Errorf("failed to update webhook %s: status %d: %s", name, updateResp.StatusCode, string(body))
		}
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed to check webhook %s: status %d: %s", name, resp.StatusCode, string(body))
}

// EnsureUserPermission creates or updates a Frappe User Permission
func (c *FrappeClient) EnsureUserPermission(ctx context.Context, user, allow, forValue string, applyToAllDoctypes bool) error {
	if c.SID == "" {
		if err := c.Authenticate(ctx); err != nil {
			return err
		}
	}

	applyAllVal := 1
	if !applyToAllDoctypes {
		applyAllVal = 0
	}

	name := fmt.Sprintf("%s-%s-%s", user, allow, forValue)
	permURL := fmt.Sprintf("%s/api/resource/User Permission/%s", c.BaseURL, url.PathEscape(name))
	req, err := c.newRequest(ctx, http.MethodGet, permURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload := map[string]interface{}{
		"user":                  user,
		"allow":                 allow,
		"for_value":             forValue,
		"apply_to_all_doctypes": applyAllVal,
	}
	payloadBytes, _ := json.Marshal(payload)

	if resp.StatusCode == http.StatusNotFound {
		createURL := fmt.Sprintf("%s/api/resource/User Permission", c.BaseURL)
		req, err := c.newRequest(ctx, http.MethodPost, createURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		createResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer createResp.Body.Close()
		if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(createResp.Body)
			return fmt.Errorf("failed to create user permission %s: status %d: %s", name, createResp.StatusCode, string(body))
		}
		return nil
	}

	if resp.StatusCode == http.StatusOK {
		req, err := c.newRequest(ctx, http.MethodPut, permURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		updateResp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer updateResp.Body.Close()
		if updateResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(updateResp.Body)
			return fmt.Errorf("failed to update user permission %s: status %d: %s", name, updateResp.StatusCode, string(body))
		}
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed to check user permission %s: status %d: %s", name, resp.StatusCode, string(body))
}
