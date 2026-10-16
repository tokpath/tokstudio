package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthenticatedArchiveRejectsCorruptionTruncationReorderingAndWrongKey(t *testing.T) {
	key := "isolated-test-key-with-at-least-thirty-two-characters"
	plain := bytes.Repeat([]byte("financial-history-and-private-evidence"), 250000)
	var encrypted bytes.Buffer
	if e := Seal(bytes.NewReader(plain), &encrypted, key); e != nil {
		t.Fatal(e)
	}
	var restored bytes.Buffer
	if e := Open(bytes.NewReader(encrypted.Bytes()), &restored, key); e != nil || !bytes.Equal(plain, restored.Bytes()) {
		t.Fatal("roundtrip", e)
	}
	b := encrypted.Bytes()
	corrupt := append([]byte(nil), b...)
	corrupt[headerSize+80] ^= 1
	reordered := append([]byte(nil), b...)
	first := headerSize
	second := headerSize + 4 + chunkSize + 16
	copy(reordered[first:first+4+chunkSize+16], b[second:second+4+chunkSize+16])
	copy(reordered[second:second+4+chunkSize+16], b[first:first+4+chunkSize+16])
	for name, data := range map[string][]byte{"changed": corrupt, "truncated": b[:len(b)-20], "appended": append(append([]byte(nil), b...), 1), "reordered": reordered} {
		t.Run(name, func(t *testing.T) {
			var got bytes.Buffer
			if e := Open(bytes.NewReader(data), &got, key); e == nil {
				t.Fatal("invalid archive accepted")
			}
		})
	}
	if e := Open(bytes.NewReader(b), &bytes.Buffer{}, key+"wrong"); e == nil {
		t.Fatal("wrong key accepted")
	}
}
func TestDecryptPublishesOnlyFullyVerifiedArchiveAndNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "archive")
	sealed := filepath.Join(dir, "sealed")
	out := filepath.Join(dir, "restored")
	key := "isolated-file-key-with-at-least-thirty-two-characters"
	if e := os.WriteFile(raw, []byte("confirmed payment"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := TransformFile(raw, sealed, key, false); e != nil {
		t.Fatal(e)
	}
	if e := TransformFile(sealed, out, key+"wrong", true); e == nil {
		t.Fatal("wrong key")
	}
	if _, e := os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("unauthenticated output published")
	}
	if e := TransformFile(sealed, out, key, true); e != nil {
		t.Fatal(e)
	}
	if e := TransformFile(sealed, out, key, true); e == nil {
		t.Fatal("existing restoration overwritten")
	}
	info, e := os.Stat(out)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unsafe output permissions", e)
	}
}
