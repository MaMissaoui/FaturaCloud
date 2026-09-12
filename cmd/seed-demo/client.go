package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

// Client is a minimal REST client for FaturaCloud's own API — the seeding
// tool deliberately drives the same HTTP surface a browser would (auth
// cookie, CSRF header, real validation) rather than calling db.Database
// methods in-process, so a seeding run doubles as an end-to-end exercise of
// the API layer, not just the persistence layer underneath it.
type Client struct {
	baseURL string
	http    *http.Client
}

// APIError carries enough context (method, path, status, server message) to
// diagnose a failed seed step without re-running with a debugger attached —
// seeding runs are long and unattended, so the error alone has to be enough.
type APIError struct {
	Method, Path string
	Status       int
	Message      string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s -> HTTP %d: %s", e.Method, e.Path, e.Status, e.Message)
}

func NewClient(baseURL string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		// 60s was too tight for one specific request: --reset's DELETE
		// /api/organizations/{id} cascades across an 18-month organization's
		// full history (clients/invoices/orders/deliveries, ...) and can run
		// past 60s server-side even though it does eventually complete —
		// found by a real --reset against a full PROD-sized organization
		// timing out client-side while the server carried on and finished
		// the delete anyway, leaving the tool believing the step had failed
		// when it had actually succeeded.
		http: &http.Client{Jar: jar, Timeout: 5 * time.Minute},
	}, nil
}

func (c *Client) do(method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s %s: marshal request: %w", method, path, err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("%s %s: build request: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		// Same stateless CSRF header src/api/client.ts sends on every
		// state-changing request — see CLAUDE.md's API section.
		req.Header.Set("X-CSRF-Protection", "1")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s %s: read response: %w", method, path, err)
	}

	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(respBody))
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			msg = errResp.Error
		}
		return &APIError{Method: method, Path: path, Status: resp.StatusCode, Message: msg}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("%s %s: decode response: %w", method, path, err)
		}
	}
	return nil
}

func (c *Client) Get(path string, out any) error { return c.do(http.MethodGet, path, nil, out) }
func (c *Client) Post(path string, body, out any) error {
	return c.do(http.MethodPost, path, body, out)
}
func (c *Client) Put(path string, body, out any) error { return c.do(http.MethodPut, path, body, out) }
func (c *Client) Patch(path string, body, out any) error {
	return c.do(http.MethodPatch, path, body, out)
}
func (c *Client) Delete(path string) error { return c.do(http.MethodDelete, path, nil, nil) }

// Login exchanges credentials for the httpOnly auth cookie, stored in the
// client's cookie jar for every subsequent request (same flow
// src/api/client.ts's login() drives).
func (c *Client) Login(email, password string) error {
	body := map[string]string{"email": email, "password": password}
	return c.Post("/api/auth/login", body, nil)
}
