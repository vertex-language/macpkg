package jwt

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// Generate creates a signed Apple App Store Connect API JWT using ES256.
func Generate(issuerID, keyID string, privateKeyPEM []byte, expiry time.Duration) (string, error) {
	key, err := parseECPrivateKey(privateKeyPEM)
	if err != nil {
		return "", fmt.Errorf("jwt: parse private key: %w", err)
	}

	header := map[string]string{
		"alg": "ES256",
		"kid": keyID,
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	if expiry <= 0 || expiry > 20*time.Minute {
		// Apple imposes a maximum 20-minute expiration
		expiry = 15 * time.Minute
	}

	claims := map[string]any{
		"iss": issuerID,
		"iat": now.Unix(),
		"exp": now.Add(expiry).Unix(),
		"aud": "appstoreconnect-v1",
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	enc := base64.RawURLEncoding
	headerB64 := enc.EncodeToString(headerJSON)
	claimsB64 := enc.EncodeToString(claimsJSON)

	signingInput := headerB64 + "." + claimsB64
	digest := sha256.Sum256([]byte(signingInput))

	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", fmt.Errorf("jwt: ecdsa sign: %w", err)
	}

	// Format signature as raw R || S (each 32 bytes for P-256)
	curveBits := key.Curve.Params().BitSize
	keyBytes := (curveBits + 7) / 8

	rBytes := r.Bytes()
	sBytes := s.Bytes()

	sigBytes := make([]byte, 2*keyBytes)
	copy(sigBytes[keyBytes-len(rBytes):keyBytes], rBytes)
	copy(sigBytes[2*keyBytes-len(sBytes):], sBytes)

	sigB64 := enc.EncodeToString(sigBytes)
	return signingInput + "." + sigB64, nil
}

func parseECPrivateKey(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	var block *pem.Block
	rest := pemBytes
	for {
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "PRIVATE KEY" || block.Type == "EC PRIVATE KEY" {
			break
		}
	}
	if block == nil {
		return nil, errors.New("no PEM private key block found")
	}

	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if ec, ok := k.(*ecdsa.PrivateKey); ok {
			return ec, nil
		}
		return nil, errors.New("PKCS#8 key is not ECDSA")
	}

	if ec, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return ec, nil
	}

	return nil, errors.New("failed to parse ECDSA private key")
}
