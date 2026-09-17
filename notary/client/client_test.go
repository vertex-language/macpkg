package client_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vertex-language/macpkg/notary/client"
)

func generateTestKey(t *testing.T) []byte {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})
}

func TestClientSubmitAndPoll(t *testing.T) {
	keyPEM := generateTestKey(t)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/upload-target" {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		switch {
		case r.Method == "POST" && r.URL.Path == "/submissions":
			resp := map[string]any{
				"data": map[string]any{
					"id":   "sub-12345",
					"type": "submissions",
					"attributes": map[string]any{
						"uploadUrl": srv.URL + "/upload-target",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case r.Method == "PUT" && r.URL.Path == "/upload-target":
			w.WriteHeader(http.StatusOK)

		case r.Method == "GET" && r.URL.Path == "/submissions/sub-12345":
			resp := map[string]any{
				"data": map[string]any{
					"id":   "sub-12345",
					"type": "submissions",
					"attributes": map[string]any{
						"status":      "Accepted",
						"createdDate": time.Now().Format(time.RFC3339),
						"name":        "test.dmg",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case r.Method == "GET" && r.URL.Path == "/submissions/sub-12345/logs":
			resp := map[string]any{
				"data": map[string]any{
					"id":   "sub-12345",
					"type": "submission-logs",
					"attributes": map[string]any{
						"developerLogUrl": "https://example.com/log.json",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := client.New("test-issuer", "test-key-id", keyPEM)
	c.BaseURL = srv.URL

	ctx := context.Background()
	subResp, err := c.Submit(ctx, "test.dmg", []byte("fake dmg content"))
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	if subResp.Data.ID != "sub-12345" {
		t.Errorf("expected id sub-12345, got %s", subResp.Data.ID)
	}

	status, err := c.GetStatus(ctx, "sub-12345")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.Data.Attributes.Status != "Accepted" {
		t.Errorf("expected status Accepted, got %s", status.Data.Attributes.Status)
	}

	logURL, err := c.GetLogs(ctx, "sub-12345")
	if err != nil {
		t.Fatalf("GetLogs failed: %v", err)
	}
	if logURL != "https://example.com/log.json" {
		t.Errorf("expected log url https://example.com/log.json, got %s", logURL)
	}

	waitStatus, err := c.Wait(ctx, "sub-12345", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if waitStatus.Data.Attributes.Status != "Accepted" {
		t.Errorf("expected wait status Accepted, got %s", waitStatus.Data.Attributes.Status)
	}
}
