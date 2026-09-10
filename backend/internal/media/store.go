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

// Store 是受控对象存储。未配置 S3 时用本地目录；配齐后对象进 S3/MinIO。
// 下载仍走 HMAC 签名的 /v1/media/objects，由 API 从后端读出，浏览器不必直连桶。
type Store struct {
	Root   string
	Secret string
	Public string
	blob   blobStore
}

type StoreOptions struct {
	Root   string
	Secret string
	Public string
	S3     S3Options
}

func NewStore(root, secret, publicBase string) *Store {
	store, _ := OpenStore(StoreOptions{Root: root, Secret: secret, Public: publicBase})
	return store
}

func OpenStore(opts StoreOptions) (*Store, error) {
	root := opts.Root
	if root == "" {
		root = filepath.Join(os.TempDir(), "tokenhub-media")
	}
	secret := opts.Secret
	if secret == "" {
		secret = "dev-media-sign-key"
	}
	store := &Store{Root: root, Secret: secret, Public: strings.TrimRight(opts.Public, "/")}
	if opts.S3.Ready() {
		blob, err := newS3Blob(opts.S3)
		if err != nil {
			return nil, err
		}
		store.blob = blob
		return store, nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) UsingS3() bool {
	return s != nil && s.blob != nil
}

func (s *Store) Put(key, contentType string, data []byte) error {
	if s.blob != nil {
		return s.blob.Put(key, contentType, data)
	}
	_ = contentType
	path := filepath.Join(s.Root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) Read(key string) ([]byte, error) {
	if s.blob != nil {
		return s.blob.Read(key)
	}
	return os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(key)))
}

func (s *Store) Delete(key string) error {
	if s.blob != nil {
		return s.blob.Delete(key)
	}
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
