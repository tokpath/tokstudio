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
	ReasoningSell   int64
	InputCost       int64
	OutputCost      int64
	ReasoningCost   int64
	InputWholesale  int64
	OutputWholesale int64
	VideoSecondSell int64
	ImageCountSell  int64
	AudioSecondSell int64
	VideoSecondCost int64
	ImageCountCost  int64
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
	if q.ReasoningSell, err = ParseUSDToMinor(str("reasoning", "reasoning_output")); err != nil {
		return q, err
	}
	if q.ReasoningCost, err = ParseUSDToMinor(str("upstream_cost_reasoning")); err != nil {
		return q, err
	}
	if q.ReasoningSell == 0 {
		q.ReasoningSell = q.OutputSell
	}
	if q.ReasoningCost == 0 {
		q.ReasoningCost = q.OutputCost
	}
	if q.VideoSecondSell, err = ParseUSDToMinor(str("video_second")); err != nil {
		return q, err
	}
	if q.ImageCountSell, err = ParseUSDToMinor(str("image_count")); err != nil {
		return q, err
	}
	if q.AudioSecondSell, err = ParseUSDToMinor(str("audio_second")); err != nil {
		return q, err
	}
	if q.VideoSecondCost, err = ParseUSDToMinor(str("video_second_cost")); err != nil {
		return q, err
	}
	if q.ImageCountCost, err = ParseUSDToMinor(str("image_count_cost")); err != nil {
		return q, err
	}
	if q.VideoSecondCost == 0 {
		q.VideoSecondCost = q.VideoSecondSell * 4 / 10
	}
	if q.ImageCountCost == 0 {
		q.ImageCountCost = q.ImageCountSell * 4 / 10
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

func resolutionFactor(resolution string) int64 {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "1080p", "1080", "1920x1080":
		return 15
	case "4k", "2160p":
		return 20
	default:
		return 10
	}
}

// Charge 按 token + 媒体单位计算客户金额。1080p/4K 用整数倍率，避免浮点。
func (q Quote) Charge(usage map[string]int, resolution string) int64 {
	if usage == nil {
		usage = map[string]int{}
	}
	amt := q.CustomerMinor(usage["prompt_tokens"], usage["completion_tokens"]) + int64(usage["reasoning_tokens"])*q.ReasoningSell
	media := int64(usage["video_seconds"])*q.VideoSecondSell +
		int64(usage["image_count"])*q.ImageCountSell +
		int64(usage["audio_seconds"])*q.AudioSecondSell
	amt += media * resolutionFactor(resolution) / 10
	return amt
}

func (q Quote) MediaCost(usage map[string]int, resolution string) int64 {
	if usage == nil {
		usage = map[string]int{}
	}
	amt := q.CostMinor(usage["prompt_tokens"], usage["completion_tokens"]) + int64(usage["reasoning_tokens"])*q.ReasoningCost
	media := int64(usage["video_seconds"])*q.VideoSecondCost +
		int64(usage["image_count"])*q.ImageCountCost
	amt += media * resolutionFactor(resolution) / 10
	return amt
}

func EstimateMediaReserveMinor(q Quote, seconds, images int, resolution string, audio bool) int64 {
	if seconds <= 0 && images <= 0 {
		seconds = 5
	}
	usage := map[string]int{"video_seconds": seconds, "image_count": images}
	if audio {
		usage["audio_seconds"] = seconds
	}
	base := q.Charge(usage, resolution)
	buf := base / 5
	if buf < 100 {
		buf = 100
	}
	return base + buf
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
