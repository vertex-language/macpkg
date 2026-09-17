package staple

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/vertex-language/macpkg/pkg/xar"
	"github.com/vertex-language/macpkg/vfs"
)

// CloudKitLookupURL is Apple's public CloudKit database for notarization tickets.
const CloudKitLookupURL = "https://api.apple-cloudkit.com/database/1/com.apple.gk.pki/production/public/records/lookup"

// FetchTicket queries Apple CloudKit for a notarization ticket corresponding to hash.
func FetchTicket(ctx context.Context, httpClient *http.Client, fileHashHex string) ([]byte, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	reqBody := map[string]any{
		"records": []any{
			map[string]string{
				"recordName": "2/" + strings.ToLower(fileHashHex),
			},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", CloudKitLookupURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudkit request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cloudkit returned HTTP %d: %s", resp.StatusCode, string(b))
	}

	var ckResp struct {
		Records []struct {
			RecordName string `json:"recordName"`
			Fields     map[string]struct {
				Value any `json:"value"`
			} `json:"fields"`
		} `json:"records"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ckResp); err != nil {
		return nil, fmt.Errorf("decode cloudkit response: %w", err)
	}

	if len(ckResp.Records) == 0 {
		return nil, fmt.Errorf("ticket not found in CloudKit for hash %s", fileHashHex)
	}

	// Look for signedProperties or ticket field
	rec := ckResp.Records[0]
	if field, ok := rec.Fields["signedProperties"]; ok {
		if s, ok := field.Value.(string); ok {
			return base64.StdEncoding.DecodeString(s)
		}
	}
	if field, ok := rec.Fields["ticket"]; ok {
		if s, ok := field.Value.(string); ok {
			return base64.StdEncoding.DecodeString(s)
		}
	}

	return nil, fmt.Errorf("cloudkit record did not contain ticket payload")
}

// Config configures ticket stapling.
type Config struct {
	Target     string // Path to .app bundle, .pkg flat package, or .dmg disk image
	Ticket     []byte // Raw ticket bytes (optional; if empty, fetches from CloudKit)
	TicketPath string // Path to ticket file (optional)
	FS         vfs.FS // Filesystem
	HTTPClient *http.Client
}

// Staple attaches a notarization ticket to the target package or bundle.
func Staple(ctx context.Context, cfg Config) error {
	if cfg.Target == "" {
		return fmt.Errorf("staple: Target is required")
	}

	fsys := cfg.FS
	if fsys == nil {
		fsys = vfs.RealFS("")
	}

	ticketData := cfg.Ticket
	if len(ticketData) == 0 && cfg.TicketPath != "" {
		var err error
		ticketData, err = fsys.ReadFile(cfg.TicketPath)
		if err != nil {
			return fmt.Errorf("read ticket file: %w", err)
		}
	}

	// If no ticket provided, compute hash of file and fetch from CloudKit
	if len(ticketData) == 0 {
		data, err := fsys.ReadFile(cfg.Target)
		if err != nil {
			return fmt.Errorf("read target for hash: %w", err)
		}
		h := sha256.Sum256(data)
		hashHex := hex.EncodeToString(h[:])
		ticketData, err = FetchTicket(ctx, cfg.HTTPClient, hashHex)
		if err != nil {
			return fmt.Errorf("fetch ticket from CloudKit: %w", err)
		}
	}

	ext := strings.ToLower(filepath.Ext(cfg.Target))
	switch {
	case strings.HasSuffix(cfg.Target, ".app"):
		return stapleApp(fsys, cfg.Target, ticketData)
	case ext == ".pkg":
		return staplePkg(fsys, cfg.Target, ticketData)
	case ext == ".dmg":
		return stapleDMG(fsys, cfg.Target, ticketData)
	default:
		return fmt.Errorf("staple: unsupported target format: %s", cfg.Target)
	}
}

func stapleApp(fsys vfs.FS, appDir string, ticket []byte) error {
	ticketPath := path.Join(appDir, "Contents", "Resources", "ticket.bin")
	return fsys.WriteFile(ticketPath, ticket, 0644)
}

func staplePkg(fsys vfs.FS, pkgPath string, ticket []byte) error {
	pkgData, err := fsys.ReadFile(pkgPath)
	if err != nil {
		return err
	}

	// Extract existing XAR files
	files, err := xar.Extract(pkgData)
	if err != nil {
		return fmt.Errorf("extract pkg xar: %w", err)
	}

	var entries []xar.FileEntry
	for name, content := range files {
		entries = append(entries, xar.FileEntry{Name: name, Data: content})
	}
	entries = append(entries, xar.FileEntry{Name: "ticket.bin", Data: ticket})

	// Re-archive
	newPkgData, err := xar.Archive(entries)
	if err != nil {
		return fmt.Errorf("repack pkg xar: %w", err)
	}

	return fsys.WriteFile(pkgPath, newPkgData, 0644)
}

func stapleDMG(fsys vfs.FS, dmgPath string, ticket []byte) error {
	// For DMG, append ticket container or write embedded ticket
	dmgData, err := fsys.ReadFile(dmgPath)
	if err != nil {
		return err
	}

	// Append ticket marker block before koly trailer if UDIF or append
	buf := new(bytes.Buffer)
	buf.Write(dmgData)
	buf.WriteString("\n<!-- TICKET -->\n")
	buf.Write(ticket)

	return fsys.WriteFile(dmgPath, buf.Bytes(), 0644)
}

var _ = os.ErrNotExist
