package jwt_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/vertex-language/macpkg/notary/jwt"
)

func TestGenerateAndVerifyJWT(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})

	issuerID := "12345678-abcd-1234-abcd-123456789abc"
	keyID := "ABCDE12345"

	token, err := jwt.Generate(issuerID, keyID, pemBytes, 10*time.Minute)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts in JWT, got %d", len(parts))
	}

	enc := base64.RawURLEncoding
	headerBytes, err := enc.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatal(err)
	}
	if header["alg"] != "ES256" || header["kid"] != keyID {
		t.Errorf("unexpected header: %+v", header)
	}

	claimsBytes, err := enc.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["iss"] != issuerID || claims["aud"] != "appstoreconnect-v1" {
		t.Errorf("unexpected claims: %+v", claims)
	}

	// Verify signature
	sigBytes, err := enc.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(sigBytes) != 64 {
		t.Fatalf("expected 64 byte signature, got %d", len(sigBytes))
	}

	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])

	signingInput := parts[0] + "." + parts[1]
	digest := sha256.Sum256([]byte(signingInput))

	if !ecdsa.Verify(&priv.PublicKey, digest[:], r, s) {
		t.Errorf("ES256 signature verification failed")
	}
}
