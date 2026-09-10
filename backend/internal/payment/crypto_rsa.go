package payment

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
)

func parseRSAPrivateKey(pemRaw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(normalizePEM(pemRaw, "RSA PRIVATE KEY")))
	if block == nil {
		return nil, ErrProviderFailed
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, ErrProviderFailed
	}
	return key, nil
}

func parseRSAPublicKey(pemRaw string) (*rsa.PublicKey, error) {
	raw := strings.TrimSpace(pemRaw)
	types := []string{"PUBLIC KEY", "RSA PUBLIC KEY"}
	if strings.Contains(raw, "BEGIN RSA PUBLIC KEY") {
		types = []string{"RSA PUBLIC KEY", "PUBLIC KEY"}
	}
	for _, typ := range types {
		block, _ := pem.Decode([]byte(normalizePEM(raw, typ)))
		if block == nil {
			continue
		}
		if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
			if pub, ok := key.(*rsa.PublicKey); ok {
				return pub, nil
			}
		}
		if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
			return key, nil
		}
	}
	return nil, ErrProviderFailed
}

func normalizePEM(raw, typ string) string {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "BEGIN") {
		return s
	}
	return "-----BEGIN " + typ + "-----\n" + s + "\n-----END " + typ + "-----\n"
}
