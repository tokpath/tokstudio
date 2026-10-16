package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

type DurableStore interface {
	VisitDurable(context.Context, func(string, string, []byte) error) error
	RequireEmpty(context.Context) error
	RestoreDurable(context.Context, string, string, []byte) error
}
type FileFact struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type,omitempty"`
}
type Manifest struct {
	Format           string     `json:"format"`
	Generation       string     `json:"generation"`
	CreatedAt        time.Time  `json:"created_at"`
	Files            []FileFact `json:"files"`
	ExcludedPrefixes []string   `json:"excluded_prefixes"`
}

func BuildArchive(ctx context.Context, dump string, store DurableStore, w io.Writer) error {
	if store == nil {
		return errors.New("private object store required")
	}
	tw := tar.NewWriter(w)
	m := Manifest{Format: "tokenhub-backup-v1", Generation: "tokenhub-operator-v2", CreatedAt: time.Now().UTC(), Files: []FileFact{}, ExcludedPrefixes: []string{"temporary-results/", "exports/"}}
	write := func(name, typ string, size int64, r io.Reader) error {
		if e := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: size, Typeflag: tar.TypeReg}); e != nil {
			return e
		}
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(tw, h), r)
		if e != nil {
			return e
		}
		if n != size {
			return io.ErrUnexpectedEOF
		}
		m.Files = append(m.Files, FileFact{Name: name, Size: size, SHA256: hex.EncodeToString(h.Sum(nil)), ContentType: typ})
		return nil
	}
	f, e := os.Open(dump)
	if e != nil {
		return e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return e
	}
	if e = write("database.dump", "application/octet-stream", info.Size(), f); e != nil {
		return e
	}
	if e = store.VisitDurable(ctx, func(key, typ string, data []byte) error {
		return write("objects/"+key, typ, int64(len(data)), bytes.NewReader(data))
	}); e != nil {
		return e
	}
	b, e := json.Marshal(m)
	if e != nil {
		return e
	}
	if e = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(b)), Typeflag: tar.TypeReg}); e != nil {
		return e
	}
	if _, e = tw.Write(b); e != nil {
		return e
	}
	return tw.Close()
}
func validObjectName(name string) bool {
	if strings.ContainsAny(name, "\\\r\n\x00") || strings.Contains(name, "..") || strings.Contains(name, "//") {
		return false
	}
	for _, p := range []string{"objects/branding/", "objects/invoices/", "objects/finance-evidence/"} {
		if strings.HasPrefix(name, p) && len(name) > len(p) {
			return true
		}
	}
	return false
}
func InspectArchive(path string) (Manifest, error) {
	var m Manifest
	f, e := os.Open(path)
	if e != nil {
		return m, e
	}
	defer f.Close()
	r := tar.NewReader(f)
	facts := map[string]FileFact{}
	manifestSeen := false
	for {
		h, e := r.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return m, e
		}
		if h.Typeflag != tar.TypeReg || manifestSeen || h.Size < 0 {
			return m, ErrInvalidArchive
		}
		if h.Name == "manifest.json" {
			if h.Size > 16<<20 {
				return m, ErrInvalidArchive
			}
			if e = json.NewDecoder(r).Decode(&m); e != nil {
				return m, ErrInvalidArchive
			}
			manifestSeen = true
			continue
		}
		if h.Name != "database.dump" && (!validObjectName(h.Name) || h.Size > 64<<20) {
			return m, ErrInvalidArchive
		}
		if _, ok := facts[h.Name]; ok {
			return m, ErrInvalidArchive
		}
		digest := sha256.New()
		n, e := io.Copy(digest, r)
		if e != nil {
			return m, e
		}
		if n != h.Size {
			return m, ErrInvalidArchive
		}
		facts[h.Name] = FileFact{Name: h.Name, Size: n, SHA256: hex.EncodeToString(digest.Sum(nil))}
	}
	if !manifestSeen || m.Format != "tokenhub-backup-v1" || m.Generation != "tokenhub-operator-v2" || len(m.Files) != len(facts) {
		return m, ErrInvalidArchive
	}
	seen := map[string]bool{}
	for _, item := range m.Files {
		v, ok := facts[item.Name]
		if !ok || seen[item.Name] || item.Size != v.Size || item.SHA256 != v.SHA256 {
			return m, ErrInvalidArchive
		}
		seen[item.Name] = true
	}
	if !seen["database.dump"] {
		return m, ErrInvalidArchive
	}
	return m, nil
}

// RestoreArchive validates the entire inventory before touching the empty
// destination bucket. It extracts the database dump without interpreting paths.
func RestoreArchive(ctx context.Context, path, dump string, store DurableStore) (Manifest, error) {
	m, e := InspectArchive(path)
	if e != nil {
		return m, e
	}
	if e = store.RequireEmpty(ctx); e != nil {
		return m, e
	}
	f, e := os.Open(path)
	if e != nil {
		return m, e
	}
	defer f.Close()
	r := tar.NewReader(f)
	types := map[string]string{}
	for _, v := range m.Files {
		types[v.Name] = v.ContentType
	}
	for {
		h, e := r.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return m, e
		}
		if h.Name == "manifest.json" {
			continue
		}
		if h.Name == "database.dump" {
			if e = PublishFile(dump, func(w io.Writer) error { _, e := io.Copy(w, r); return e }); e != nil {
				return m, e
			}
			continue
		}
		b, e := io.ReadAll(io.LimitReader(r, (64<<20)+1))
		if e != nil {
			return m, e
		}
		if len(b) > 64<<20 {
			return m, ErrInvalidArchive
		}
		if e = store.RestoreDurable(ctx, strings.TrimPrefix(h.Name, "objects/"), types[h.Name], b); e != nil {
			return m, e
		}
	}
	expected := map[string]FileFact{}
	for _, v := range m.Files {
		if v.Name != "database.dump" {
			expected[strings.TrimPrefix(v.Name, "objects/")] = v
		}
	}
	if e = store.VisitDurable(ctx, func(key, typ string, data []byte) error {
		v, ok := expected[key]
		hash := sha256.Sum256(data)
		if !ok || v.Size != int64(len(data)) || v.SHA256 != hex.EncodeToString(hash[:]) || v.ContentType != typ {
			return ErrInvalidArchive
		}
		delete(expected, key)
		return nil
	}); e != nil {
		return m, e
	}
	if len(expected) != 0 {
		return m, ErrInvalidArchive
	}
	return m, nil
}
