package notary_test

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
	"testing"
	"time"

	"github.com/vertex-language/macpkg/notary"
	"github.com/vertex-language/macpkg/vfs"
)

func generateKey(t *testing.T) []byte {
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

func TestNotarySubmitWait(t *testing.T) {
	keyPEM := generateKey(t)
	mem := vfs.NewMemFS()
	_ = mem.WriteFile("MyApp.zip", []byte("fake zip"), 0644)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/submissions":
			resp := map[string]any{
				"data": map[string]any{
					"id":   "sub-999",
					"type": "submissions",
					"attributes": map[string]any{
						"uploadUrl": srv.URL + "/upload",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case r.Method == "PUT" && r.URL.Path == "/upload":
			w.WriteHeader(http.StatusOK)
		case r.Method == "GET" && r.URL.Path == "/submissions/sub-999":
			resp := map[string]any{
				"data": map[string]any{
					"id":   "sub-999",
					"type": "submissions",
					"attributes": map[string]any{
						"status":      "Accepted",
						"createdDate": time.Now().Format(time.RFC3339),
						"name":        "MyApp.zip",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case r.Method == "GET" && r.URL.Path == "/submissions/sub-999/logs":
			resp := map[string]any{
				"data": map[string]any{
					"id":   "sub-999",
					"type": "submission-logs",
					"attributes": map[string]any{
						"developerLogUrl": "https://example.com/log",
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

	cfg := notary.Config{
		IssuerID:      "test-issuer-id",
		KeyID:         "test-key-id",
		PrivateKeyPEM: keyPEM,
		Target:        "MyApp.zip",
		Wait:          true,
		Timeout:       5 * time.Second,
		FS:            mem,
		BaseURL:       srv.URL,
	}

	sub, err := notary.Submit(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	if sub.ID != "sub-999" || sub.Status != "Accepted" {
		t.Errorf("unexpected submission: %+v", sub)
	}
	if sub.LogURL != "https://example.com/log" {
		t.Errorf("unexpected log url: %s", sub.LogURL)
	}
}
