package billing

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// MinorPerUSD 是 1 美元对应的最小货币单位（micro-USD）。
// 账务加减只在这个整数上进行，避免 float64 误差。
const MinorPerUSD int64 = 1_000_000

// ParseUSDToMinor 把十进制美元字符串转成 micro-USD，四舍五入到最近的整数。
func ParseUSDToMinor(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, fmt.Errorf("invalid decimal %q", s)
	}
	r.Mul(r, new(big.Rat).SetInt64(MinorPerUSD))
	num := new(big.Int).Set(r.Num())
	den := r.Denom()
	quot, rem := new(big.Int).QuoRem(new(big.Int).Abs(num), den, new(big.Int))
	twice := new(big.Int).Lsh(rem, 1)
	if twice.Cmp(den) >= 0 {
		quot.Add(quot, big.NewInt(1))
	}
	if r.Sign() < 0 {
		quot.Neg(quot)
	}
	if !quot.IsInt64() {
		return 0, fmt.Errorf("amount overflow")
	}
	return quot.Int64(), nil
}

// MinorToUSDString 把 micro-USD 格式化成最多 6 位小数的美元字符串。
func MinorToUSDString(minor int64) string {
	r := new(big.Rat).SetFrac64(minor, MinorPerUSD)
	return strings.TrimRight(strings.TrimRight(r.FloatString(6), "0"), ".")
}

// Quote 是结算用的价格快照。网关从 catalog 拿到 JSON 后交给账务，账务不再读 catalog 表。
type Quote struct {
	VersionID       string
	Currency        string
	Raw             json.RawMessage
	InputSell       int64
	OutputSell      int64
	InputCost       int64
	OutputCost      int64
	InputWholesale  int64
	OutputWholesale int64
}

func ParseQuote(versionID string, raw []byte) (Quote, error) {
	q := Quote{VersionID: versionID, Currency: "USD", Raw: raw}
	var m map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	str := func(keys ...string) string {
		for _, key := range keys {
			if v, ok := m[key]; ok {
				return fmt.Sprint(v)
			}
		}
		return ""
	}
	if c := str("currency"); c != "" {
		q.Currency = c
	}
	var err error
	if q.InputSell, err = ParseUSDToMinor(str("input", "customer_sell_input")); err != nil {
		return q, err
	}
	if q.OutputSell, err = ParseUSDToMinor(str("output", "customer_sell_output")); err != nil {
		return q, err
	}
	if q.InputCost, err = ParseUSDToMinor(str("upstream_cost_input")); err != nil {
		return q, err
	}
	if q.OutputCost, err = ParseUSDToMinor(str("upstream_cost_output")); err != nil {
		return q, err
	}
	if q.InputWholesale, err = ParseUSDToMinor(str("wholesale_input")); err != nil {
		return q, err
	}
	if q.OutputWholesale, err = ParseUSDToMinor(str("wholesale_output")); err != nil {
		return q, err
	}
	if q.InputCost == 0 {
		q.InputCost = q.InputSell * 4 / 10
	}
	if q.OutputCost == 0 {
		q.OutputCost = q.OutputSell * 4 / 10
	}
	if q.InputWholesale == 0 {
		q.InputWholesale = q.InputSell * 7 / 10
	}
	if q.OutputWholesale == 0 {
		q.OutputWholesale = q.OutputSell * 7 / 10
	}
	return q, nil
}

func (q Quote) CustomerMinor(prompt, completion int) int64 {
	return int64(prompt)*q.InputSell + int64(completion)*q.OutputSell
}

func (q Quote) CostMinor(prompt, completion int) int64 {
	return int64(prompt)*q.InputCost + int64(completion)*q.OutputCost
}

func (q Quote) WholesaleMinor(prompt, completion int) int64 {
	return int64(prompt)*q.InputWholesale + int64(completion)*q.OutputWholesale
}

// EstimateReserveMinor 按提示长度和 max_tokens 估算预授权。
// 多留 20% 缓冲，且不少于 100 micro-USD，避免余额刚好卡在四舍五入边界。
func EstimateReserveMinor(q Quote, promptHint, maxTokens int) int64 {
	if promptHint <= 0 {
		promptHint = 64
	}
	if maxTokens <= 0 {
		maxTokens = 256
	}
	base := q.CustomerMinor(promptHint, maxTokens)
	buf := base / 5
	if buf < 100 {
		buf = 100
	}
	return base + buf
}
