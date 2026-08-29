package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

func key32(material string) []byte {
	sum := sha256.Sum256([]byte(material))
	return sum[:]
}

// Seal 用应用层密钥做 AES-GCM。数据库只存密文。
func Seal(material, plaintext string) (string, error) {
	block, err := aes.NewCipher(key32(material))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(out), nil
}

func Open(material, ciphertext string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key32(material))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, payload := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
