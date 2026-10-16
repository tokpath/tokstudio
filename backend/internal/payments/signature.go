package payments

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
)

func privateRSA(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, ErrNotConfigured
	}
	if key, e := x509.ParsePKCS1PrivateKey(block.Bytes); e == nil {
		if key.N.BitLen() < 2048 {
			return nil, ErrNotConfigured
		}
		return key, nil
	}
	parsed, e := x509.ParsePKCS8PrivateKey(block.Bytes)
	if e != nil {
		return nil, ErrNotConfigured
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.N.BitLen() < 2048 {
		return nil, ErrNotConfigured
	}
	return key, nil
}
func publicRSA(raw string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, ErrNotConfigured
	}
	if key, e := x509.ParsePKCS1PublicKey(block.Bytes); e == nil {
		if key.N.BitLen() < 2048 {
			return nil, ErrNotConfigured
		}
		return key, nil
	}
	if cert, e := x509.ParseCertificate(block.Bytes); e == nil {
		key, ok := cert.PublicKey.(*rsa.PublicKey)
		if ok && key.N.BitLen() >= 2048 {
			return key, nil
		}
	}
	parsed, e := x509.ParsePKIXPublicKey(block.Bytes)
	if e != nil {
		return nil, ErrNotConfigured
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok || key.N.BitLen() < 2048 {
		return nil, ErrNotConfigured
	}
	return key, nil
}
func signRSA(private, message string) (string, error) {
	key, e := privateRSA(private)
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256([]byte(message))
	signature, e := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if e != nil {
		return "", e
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}
func verifyRSA(public, message, signature string) bool {
	key, e := publicRSA(public)
	if e != nil {
		return false
	}
	raw, e := base64.StdEncoding.DecodeString(signature)
	if e != nil {
		return false
	}
	sum := sha256.Sum256([]byte(message))
	return rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], raw) == nil
}
