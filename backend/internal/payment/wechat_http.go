package payment

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	wechatAPIBase = "https://api.mch.weixin.qq.com"
	wechatHTTP    = &http.Client{Timeout: 15 * time.Second}
)

func wechatDo(ctx context.Context, creds map[string]string, method, path string, payload []byte) ([]byte, int, error) {
	if payload == nil {
		payload = []byte{}
	}
	ts := fmt.Sprintf("%d", time.Now().UTC().Unix())
	nonce, err := wechatNonce()
	if err != nil {
		return nil, 0, err
	}
	msg := method + "\n" + path + "\n" + ts + "\n" + nonce + "\n" + string(payload) + "\n"
	sig, err := wechatSign(cred(creds, "merchant_api_private_key"), msg)
	if err != nil {
		return nil, 0, err
	}
	auth := fmt.Sprintf(
		`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",timestamp="%s",serial_no="%s",signature="%s"`,
		cred(creds, "mch_id"), nonce, ts, cred(creds, "cert_serial"), sig,
	)
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(wechatAPIBase, "/")+path, strings.NewReader(string(payload)))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/json")
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := wechatHTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderBody))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return raw, resp.StatusCode, nil
}

func wechatNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func wechatSign(pemRaw, message string) (string, error) {
	key, err := parseRSAPrivateKey(pemRaw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func wechatVerify(pemRaw, timestamp, nonce, signature string, body []byte) bool {
	pub, err := parseRSAPublicKey(pemRaw)
	if err != nil {
		return false
	}
	sigRaw, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	msg := timestamp + "\n" + nonce + "\n" + string(body) + "\n"
	sum := sha256.Sum256([]byte(msg))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sigRaw) == nil
}

func wechatDecryptResource(apiV3Key, ciphertext, nonce, associatedData string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil || len(raw) < 16 {
		return nil, ErrProviderFailed
	}
	block, err := aes.NewCipher([]byte(apiV3Key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, []byte(nonce), raw, []byte(associatedData))
	if err != nil {
		return nil, err
	}
	return plain, nil
}

func wechatEventStatus(outer, inner map[string]any) (status, orderID, tradeID, eventID string) {
	eventID = asString(outer["id"])
	orderID = asString(inner["out_trade_no"])
	tradeID = asString(inner["transaction_id"])
	if tradeID == "" {
		tradeID = asString(inner["refund_id"])
	}
	st := asString(inner["trade_state"])
	if st == "" {
		st = asString(inner["refund_status"])
	}
	typ := asString(outer["event_type"])
	switch {
	case st == "SUCCESS" || strings.Contains(typ, "TRANSACTION.SUCCESS"):
		status = StatusPaid
	case strings.Contains(st, "REFUND") || strings.Contains(typ, "REFUND"):
		status = StatusRefunded
	case st == "CLOSED" || st == "PAYERROR" || strings.Contains(typ, "FAIL"):
		status = StatusFailed
	}
	if eventID == "" {
		eventID = tradeID
	}
	return status, orderID, tradeID, eventID
}

func wechatNotifyResource(body []byte) (outer map[string]any, resource map[string]any) {
	outer = decodeJSONMap(body)
	if outer == nil {
		return nil, nil
	}
	resource, _ = outer["resource"].(map[string]any)
	return outer, resource
}

func encodeJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}
