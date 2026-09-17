package notary

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/vertex-language/macpkg/notary/client"
	"github.com/vertex-language/macpkg/notary/staple"
	"github.com/vertex-language/macpkg/vfs"
)

// Config configures Apple Notarization submission.
type Config struct {
	IssuerID       string        // App Store Connect API Issuer ID (UUID)
	KeyID          string        // App Store Connect API Key ID (e.g. 2X9R4NN92Z)
	PrivateKeyPath string        // Path to AuthKey_<KeyID>.p8 private key file
	PrivateKeyPEM  []byte        // Raw private key PEM bytes (optional alternative to file)
	Target         string        // Path to .zip, .dmg, or .pkg artifact to submit
	Wait           bool          // Wait for notarization to finish
	Timeout        time.Duration // Maximum wait time (default: 30m)
	FS             vfs.FS        // Filesystem (nil => RealFS)
	BaseURL        string        // Custom API endpoint (empty => Apple production)
	HTTPClient     *http.Client  // Custom HTTP client
}

// Submission contains notarization submission details.
type Submission struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Target      string    `json:"target"`
	CreatedDate time.Time `json:"created_date"`
	LogURL      string    `json:"log_url,omitempty"`
}

// Submit sends an artifact to Apple's Notary REST API v2.
func Submit(ctx context.Context, cfg Config) (*Submission, error) {
	if cfg.IssuerID == "" {
		return nil, fmt.Errorf("notary: IssuerID is required")
	}
	if cfg.KeyID == "" {
		return nil, fmt.Errorf("notary: KeyID is required")
	}
	if cfg.Target == "" {
		return nil, fmt.Errorf("notary: Target artifact path is required")
	}

	fsys := cfg.FS
	if fsys == nil {
		fsys = vfs.RealFS("")
	}

	keyPEM := cfg.PrivateKeyPEM
	if len(keyPEM) == 0 && cfg.PrivateKeyPath != "" {
		var err error
		keyPEM, err = fsys.ReadFile(cfg.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read private key %s: %w", cfg.PrivateKeyPath, err)
		}
	}
	if len(keyPEM) == 0 {
		return nil, fmt.Errorf("notary: PrivateKeyPath or PrivateKeyPEM is required")
	}

	data, err := fsys.ReadFile(cfg.Target)
	if err != nil {
		return nil, fmt.Errorf("read target %s: %w", cfg.Target, err)
	}

	c := client.New(cfg.IssuerID, cfg.KeyID, keyPEM)
	if cfg.BaseURL != "" {
		c.BaseURL = cfg.BaseURL
	}
	if cfg.HTTPClient != nil {
		c.HTTPClient = cfg.HTTPClient
	}

	filename := filepath.Base(cfg.Target)
	subResp, err := c.Submit(ctx, filename, data)
	if err != nil {
		return nil, fmt.Errorf("submit to notary: %w", err)
	}

	submissionID := subResp.Data.ID
	sub := &Submission{
		ID:          submissionID,
		Status:      "In Progress",
		Target:      cfg.Target,
		CreatedDate: time.Now().UTC(),
	}

	if cfg.Wait {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Minute
		}
		waitCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		st, err := c.Wait(waitCtx, submissionID, 5*time.Second)
		if err != nil {
			return sub, fmt.Errorf("wait for notarization: %w", err)
		}

		sub.Status = st.Data.Attributes.Status
		sub.CreatedDate = st.Data.Attributes.CreatedDate

		logURL, _ := c.GetLogs(ctx, submissionID)
		sub.LogURL = logURL

		if sub.Status != "Accepted" {
			return sub, fmt.Errorf("notarization ended with status %s: %s", sub.Status, logURL)
		}
	}

	return sub, nil
}

// Staple attaches a notarization ticket to cfg.Target.
func Staple(ctx context.Context, cfg staple.Config) error {
	return staple.Staple(ctx, cfg)
}
