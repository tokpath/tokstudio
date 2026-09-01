package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func SignWebhook(secret, eventID, orderID, status string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(eventID + "|" + orderID + "|" + status))
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyWebhook(secret, eventID, orderID, status, sig string) bool {
	want := SignWebhook(secret, eventID, orderID, status)
	return hmac.Equal([]byte(want), []byte(strings.TrimSpace(sig)))
}
