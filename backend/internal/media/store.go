package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Store 是受控对象存储。生产可换成 S3，P0 用本地目录 + HMAC 签名 URL。
type Store struct {
	Root   string
	Secret string
	Public string
}

func NewStore(root, secret, publicBase string) *Store {
	if root == "" {
		root = filepath.Join(os.TempDir(), "tokenhub-media")
	}
	if secret == "" {
		secret = "dev-media-sign-key"
	}
	_ = os.MkdirAll(root, 0o755)
	return &Store{Root: root, Secret: secret, Public: strings.TrimRight(publicBase, "/")}
}

func (s *Store) Put(key, contentType string, data []byte) error {
	_ = contentType
	path := filepath.Join(s.Root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) Read(key string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(key)))
}

func (s *Store) Delete(key string) error {
	return os.Remove(filepath.Join(s.Root, filepath.FromSlash(key)))
}

func (s *Store) Sign(key string, ttl time.Duration) (string, time.Time, error) {
	exp := time.Now().UTC().Add(ttl)
	mac := hmac.New(sha256.New, []byte(s.Secret))
	_, _ = mac.Write([]byte(key + "|" + strconv.FormatInt(exp.Unix(), 10)))
	sig := hex.EncodeToString(mac.Sum(nil))
	url := fmt.Sprintf("/v1/media/objects?key=%s&exp=%d&sig=%s", url.QueryEscape(key), exp.Unix(), sig)
	if s.Public != "" && !strings.HasPrefix(s.Public, "http://localhost") && !strings.HasPrefix(s.Public, "http://127.0.0.1") {
		url = s.Public + url
	}
	return url, exp, nil
}

func (s *Store) Verify(key, sig string, expUnix int64) bool {
	if time.Now().UTC().Unix() > expUnix {
		return false
	}
	mac := hmac.New(sha256.New, []byte(s.Secret))
	_, _ = mac.Write([]byte(key + "|" + strconv.FormatInt(expUnix, 10)))
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(sig))
}

func (s *Store) CallbackSign(eventID, jobID string) string {
	mac := hmac.New(sha256.New, []byte(s.Secret))
	_, _ = mac.Write([]byte(eventID + "|" + jobID))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Store) CallbackValid(eventID, jobID, sig string) bool {
	return hmac.Equal([]byte(s.CallbackSign(eventID, jobID)), []byte(sig))
}
