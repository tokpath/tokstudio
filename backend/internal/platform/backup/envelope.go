// Package backup encrypts complete backup archives, including authentication
// of chunk order and the final boundary. No decrypted file is published until
// every authentication tag and the final EOF have been verified.
package backup

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
)

const chunkSize = 4 << 20
const headerSize = 5 + 32 + 8

var ErrInvalidArchive = errors.New("backup archive authentication failed")

func aead(password string, header []byte) (cipher.AEAD, error) {
	if len(password) < 32 {
		return nil, errors.New("TOKENHUB_BACKUP_KEY must contain at least 32 random characters")
	}
	key, e := pbkdf2.Key(sha256.New, password, header[5:37], 600000, 32)
	if e != nil {
		return nil, e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(block)
}
func chunkMetadata(header []byte, index uint32, size uint32) ([]byte, []byte) {
	nonce := make([]byte, 12)
	copy(nonce, header[37:])
	binary.BigEndian.PutUint32(nonce[8:], index)
	ad := make([]byte, headerSize+8)
	copy(ad, header)
	binary.BigEndian.PutUint32(ad[headerSize:], index)
	binary.BigEndian.PutUint32(ad[headerSize+4:], size)
	return nonce, ad
}
func Seal(r io.Reader, w io.Writer, password string) error {
	header := make([]byte, headerSize)
	copy(header, []byte("THBK1"))
	if _, e := rand.Read(header[5:]); e != nil {
		return e
	}
	a, e := aead(password, header)
	if e != nil {
		return e
	}
	if _, e = w.Write(header); e != nil {
		return e
	}
	buf := make([]byte, chunkSize)
	for index := uint32(0); ; index++ {
		if index == math.MaxUint32 {
			return errors.New("backup archive too large")
		}
		n, readErr := io.ReadFull(r, buf)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return readErr
		}
		nonce, ad := chunkMetadata(header, index, uint32(n))
		sealed := a.Seal(nil, nonce, buf[:n], ad)
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(n))
		if _, e = w.Write(size[:]); e != nil {
			return e
		}
		if _, e = w.Write(sealed); e != nil {
			return e
		}
		if n == 0 {
			return nil
		}
	}
}
func Open(r io.Reader, w io.Writer, password string) error {
	header := make([]byte, headerSize)
	if _, e := io.ReadFull(r, header); e != nil || !bytes.Equal(header[:5], []byte("THBK1")) {
		return ErrInvalidArchive
	}
	a, e := aead(password, header)
	if e != nil {
		return e
	}
	for index := uint32(0); ; index++ {
		if index == math.MaxUint32 {
			return ErrInvalidArchive
		}
		var size [4]byte
		if _, e = io.ReadFull(r, size[:]); e != nil {
			return ErrInvalidArchive
		}
		n := binary.BigEndian.Uint32(size[:])
		if n > chunkSize {
			return ErrInvalidArchive
		}
		sealed := make([]byte, int(n)+a.Overhead())
		if _, e = io.ReadFull(r, sealed); e != nil {
			return ErrInvalidArchive
		}
		nonce, ad := chunkMetadata(header, index, n)
		plain, e := a.Open(nil, nonce, sealed, ad)
		if e != nil {
			return ErrInvalidArchive
		}
		if n == 0 {
			var trailing [1]byte
			if _, e = io.ReadFull(r, trailing[:]); e != io.EOF {
				return ErrInvalidArchive
			}
			return nil
		}
		if _, e = w.Write(plain); e != nil {
			return e
		}
	}
}

func TransformFile(input, output, password string, decrypt bool) error {
	if input == "" || output == "" {
		return errors.New("input and output files are required")
	}
	source, e := os.Open(input)
	if e != nil {
		return e
	}
	defer source.Close()
	return PublishFile(output, func(dest io.Writer) error {
		if decrypt {
			return Open(source, dest, password)
		}
		return Seal(source, dest, password)
	})
}

// PublishFile writes in the destination directory with mode 0600 and atomically
// links the authenticated result without overwriting any existing destination.
func PublishFile(output string, write func(io.Writer) error) error {
	f, e := os.CreateTemp(filepath.Dir(output), ".tokenhub-backup-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if e = write(f); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Link(f.Name(), output)
}
