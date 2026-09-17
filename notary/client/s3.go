package client

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type S3Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Bucket          string
	Object          string
	Region          string
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func getSignatureKey(secret, dateStamp, regionName, serviceName string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, regionName)
	kService := hmacSHA256(kRegion, serviceName)
	kSigning := hmacSHA256(kService, "aws4_request")
	return kSigning
}

// UploadToS3 uploads data to AWS S3 using SigV4 credentials provided by Apple Notary API.
func UploadToS3(ctx context.Context, httpClient *http.Client, creds S3Credentials, data []byte) error {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	region := creds.Region
	if region == "" {
		region = "us-west-2" // Apple Notary S3 default region
	}
	service := "s3"

	host := fmt.Sprintf("%s.s3.amazonaws.com", creds.Bucket)
	endpoint := fmt.Sprintf("https://%s/%s", host, creds.Object)

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	contentHash := sha256.Sum256(data)
	contentHashHex := hex.EncodeToString(contentHash[:])

	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Host", host)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-security-token", creds.SessionToken)
	req.Header.Set("x-amz-content-sha256", contentHashHex)

	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\nx-amz-security-token:%s\n",
		host, contentHashHex, amzDate, creds.SessionToken)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date;x-amz-security-token"

	canonicalURI := "/" + creds.Object
	if !strings.HasPrefix(canonicalURI, "/") {
		canonicalURI = "/" + canonicalURI
	}

	canonicalRequest := fmt.Sprintf("PUT\n%s\n\n%s\n%s\n%s",
		canonicalURI, canonicalHeaders, signedHeaders, contentHashHex)

	canonicalRequestHash := sha256.Sum256([]byte(canonicalRequest))
	canonicalRequestHashHex := hex.EncodeToString(canonicalRequestHash[:])

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate, credentialScope, canonicalRequestHashHex)

	signingKey := getSignatureKey(creds.SecretAccessKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKeyID, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("s3 upload error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3 upload returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
