// Package mast is a stdlib-only client for MobSF's REST API, so SHIELD can run a
// Mobile Application Security Testing (MAST) scan as part of the analyze →
// protect → verify loop. MobSF (GPL-3.0) runs as a separate service — we call it
// over HTTP and never link its code, so this package (and the whole engine) stays
// Apache-2.0 and dependency-free. See deploy/mobsf/ to run MobSF locally.
package mast

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Client talks to a MobSF instance. APIKey is the REST API key shown in the MobSF
// UI (or the MOBSF_API_KEY env). A zero HTTP uses a sensible default.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// New builds a Client. baseURL defaults to http://localhost:8000.
func New(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
	}
}

// Uploaded is MobSF's response to an upload.
type Uploaded struct {
	FileName string `json:"file_name"`
	Hash     string `json:"hash"`
	ScanType string `json:"scan_type"`
}

// Finding is one MAST result (an appsec item).
type Finding struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Section     string `json:"section"`
}

// Report is the subset of a MobSF scan we care about for the differential.
type Report struct {
	Hash          string    `json:"-"`
	SecurityScore int       `json:"security_score"`
	High          []Finding `json:"high"`
	Warning       []Finding `json:"warning"`
	Info          []Finding `json:"info"`
	Secure        []Finding `json:"secure"`
}

// reportEnvelope matches MobSF's report_json: the appsec block holds the score
// and findings; older builds expose security_score at the top level.
type reportEnvelope struct {
	AppSec        *Report `json:"appsec"`
	SecurityScore *int    `json:"security_score"`
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// Upload posts a file to MobSF and returns its hash.
func (c *Client) Upload(ctx context.Context, filename string, r io.Reader) (Uploaded, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return Uploaded{}, err
	}
	if _, err := io.Copy(fw, r); err != nil {
		return Uploaded{}, err
	}
	if err := mw.Close(); err != nil {
		return Uploaded{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/upload", &body)
	if err != nil {
		return Uploaded{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", c.APIKey)

	var out Uploaded
	if err := c.do(req, &out); err != nil {
		return Uploaded{}, err
	}
	if out.Hash == "" {
		return Uploaded{}, fmt.Errorf("mobsf upload: empty hash in response")
	}
	return out, nil
}

// Scan triggers static analysis for an uploaded hash and returns the report.
func (c *Client) Scan(ctx context.Context, hash string) (Report, error) {
	return c.form(ctx, "/api/v1/scan", hash)
}

// ReportJSON fetches the JSON report for an already-scanned hash.
func (c *Client) ReportJSON(ctx context.Context, hash string) (Report, error) {
	return c.form(ctx, "/api/v1/report_json", hash)
}

// ScanFile is the convenience loop: upload → scan → parse the report.
func (c *Client) ScanFile(ctx context.Context, path string) (Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return Report{}, err
	}
	defer f.Close()
	up, err := c.Upload(ctx, path, f)
	if err != nil {
		return Report{}, err
	}
	return c.Scan(ctx, up.Hash)
}

func (c *Client) form(ctx context.Context, path, hash string) (Report, error) {
	form := url.Values{"hash": {hash}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path,
		strings.NewReader(form.Encode()))
	if err != nil {
		return Report{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", c.APIKey)

	var env reportEnvelope
	if err := c.do(req, &env); err != nil {
		return Report{}, err
	}
	var rep Report
	if env.AppSec != nil {
		rep = *env.AppSec
	}
	if env.SecurityScore != nil && rep.SecurityScore == 0 {
		rep.SecurityScore = *env.SecurityScore
	}
	rep.Hash = hash
	return rep, nil
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mobsf %s: status %d: %s", req.URL.Path, resp.StatusCode,
			strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("mobsf %s: decode response: %w", req.URL.Path, err)
	}
	return nil
}
