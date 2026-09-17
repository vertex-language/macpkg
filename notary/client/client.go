package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/vertex-language/macpkg/notary/jwt"
)

// BaseURL is Apple's production Notary API v2 endpoint.
const BaseURL = "https://appstoreconnect.apple.com/notary/v2"

// Client is a pure-Go client for the Apple Notary REST API v2.
type Client struct {
	IssuerID      string
	KeyID         string
	PrivateKeyPEM []byte
	BaseURL       string
	HTTPClient    *http.Client
}

// New creates a new Notary API client.
func New(issuerID, keyID string, privateKeyPEM []byte) *Client {
	return &Client{
		IssuerID:      issuerID,
		KeyID:         keyID,
		PrivateKeyPEM: privateKeyPEM,
		BaseURL:       BaseURL,
		HTTPClient:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) getBaseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return BaseURL
}

func (c *Client) getHTTPClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Client) authHeader() (string, error) {
	token, err := jwt.Generate(c.IssuerID, c.KeyID, c.PrivateKeyPEM, 15*time.Minute)
	if err != nil {
		return "", err
	}
	return "Bearer " + token, nil
}

// SubmissionResponse is returned by Apple when starting a submission.
type SubmissionResponse struct {
	Data struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			AwsAccessKeyID     string `json:"awsAccessKeyId"`
			AwsSecretAccessKey string `json:"awsSecretAccessKey"`
			AwsSessionToken    string `json:"awsSessionToken"`
			Bucket             string `json:"bucket"`
			Object             string `json:"object"`
			UploadURL          string `json:"uploadUrl"`
		} `json:"attributes"`
	} `json:"data"`
}

// StatusResponse is returned by Apple when querying submission status.
type StatusResponse struct {
	Data struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			Status      string    `json:"status"`
			CreatedDate time.Time `json:"createdDate"`
			Name        string    `json:"name"`
			Message     string    `json:"message"`
		} `json:"attributes"`
	} `json:"data"`
}

// LogsResponse is returned by Apple when querying submission logs.
type LogsResponse struct {
	Data struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			DeveloperLogURL string `json:"developerLogUrl"`
		} `json:"attributes"`
	} `json:"data"`
}

// Submit starts a notarization submission and uploads the payload.
func (c *Client) Submit(ctx context.Context, filename string, data []byte) (*SubmissionResponse, error) {
	auth, err := c.authHeader()
	if err != nil {
		return nil, fmt.Errorf("generate auth header: %w", err)
	}

	h256 := sha256.Sum256(data)
	hashHex := hex.EncodeToString(h256[:])

	reqBody := map[string]any{
		"submissionName": filepath.Base(filename),
		"sha256":         hashHex,
		"notifications":  []any{},
	}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := c.getBaseURL() + "/submissions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")

	hc := c.getHTTPClient()
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("submit request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("submit returned HTTP %d: %s", resp.StatusCode, string(b))
	}

	var subResp SubmissionResponse
	if err := json.NewDecoder(resp.Body).Decode(&subResp); err != nil {
		return nil, fmt.Errorf("decode submission response: %w", err)
	}

	// Upload payload to S3
	attrs := subResp.Data.Attributes
	if attrs.UploadURL != "" {
		putReq, err := http.NewRequestWithContext(ctx, "PUT", attrs.UploadURL, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		putResp, err := hc.Do(putReq)
		if err != nil {
			return nil, fmt.Errorf("upload to url: %w", err)
		}
		defer putResp.Body.Close()
		if putResp.StatusCode < 200 || putResp.StatusCode >= 300 {
			b, _ := io.ReadAll(putResp.Body)
			return nil, fmt.Errorf("upload returned HTTP %d: %s", putResp.StatusCode, string(b))
		}
	} else if attrs.AwsAccessKeyID != "" {
		creds := S3Credentials{
			AccessKeyID:     attrs.AwsAccessKeyID,
			SecretAccessKey: attrs.AwsSecretAccessKey,
			SessionToken:    attrs.AwsSessionToken,
			Bucket:          attrs.Bucket,
			Object:          attrs.Object,
		}
		if err := UploadToS3(ctx, hc, creds, data); err != nil {
			return nil, fmt.Errorf("upload to s3: %w", err)
		}
	} else {
		return nil, fmt.Errorf("submission response missing upload targets")
	}

	return &subResp, nil
}

// GetStatus checks the current status of a submission.
func (c *Client) GetStatus(ctx context.Context, submissionID string) (*StatusResponse, error) {
	auth, err := c.authHeader()
	if err != nil {
		return nil, fmt.Errorf("generate auth header: %w", err)
	}

	url := fmt.Sprintf("%s/submissions/%s", c.getBaseURL(), submissionID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)

	resp, err := c.getHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status returned HTTP %d: %s", resp.StatusCode, string(b))
	}

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode status response: %w", err)
	}

	return &status, nil
}

// GetLogs returns the developer log URL for a submission.
func (c *Client) GetLogs(ctx context.Context, submissionID string) (string, error) {
	auth, err := c.authHeader()
	if err != nil {
		return "", fmt.Errorf("generate auth header: %w", err)
	}

	url := fmt.Sprintf("%s/submissions/%s/logs", c.getBaseURL(), submissionID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", auth)

	resp, err := c.getHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("logs returned HTTP %d: %s", resp.StatusCode, string(b))
	}

	var logs LogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return "", fmt.Errorf("decode logs response: %w", err)
	}

	return logs.Data.Attributes.DeveloperLogURL, nil
}

// Wait polls a submission until it reaches Accepted, Invalid, or Rejected.
func (c *Client) Wait(ctx context.Context, submissionID string, pollInterval time.Duration) (*StatusResponse, error) {
	// Initial check before waiting
	st, err := c.GetStatus(ctx, submissionID)
	if err == nil {
		s := strings.ToLower(st.Data.Attributes.Status)
		if s == "accepted" || s == "invalid" || s == "rejected" {
			return st, nil
		}
	}

	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			st, err := c.GetStatus(ctx, submissionID)
			if err != nil {
				return nil, err
			}
			s := strings.ToLower(st.Data.Attributes.Status)
			if s == "accepted" || s == "invalid" || s == "rejected" {
				return st, nil
			}
		}
	}
}
