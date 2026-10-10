package page

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

var ErrCursor = errors.New("invalid page cursor")

func Encode(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "\n" + id))
}
func Decode(raw string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", ErrCursor
	}
	parts := strings.Split(string(b), "\n")
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", ErrCursor
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", ErrCursor
	}
	return at, parts[1], nil
}
