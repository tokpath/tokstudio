package pagecursor

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid personal records cursor")

type Position struct {
	At        time.Time
	ID, Scope string
}

func scope(userID, kind string) string {
	sum := sha256.Sum256([]byte(userID + "\n" + kind))
	return hex.EncodeToString(sum[:])
}
func Decode(raw, userID, kind string) (*Position, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 1024 {
		return nil, ErrInvalid
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	var p Position
	if err != nil || json.Unmarshal(data, &p) != nil || p.At.IsZero() || p.ID == "" || len(p.ID) > 128 || p.Scope != scope(userID, kind) {
		return nil, ErrInvalid
	}
	return &p, nil
}
func Encode(at time.Time, id, userID, kind string) string {
	raw, _ := json.Marshal(Position{at, id, scope(userID, kind)})
	return base64.RawURLEncoding.EncodeToString(raw)
}
