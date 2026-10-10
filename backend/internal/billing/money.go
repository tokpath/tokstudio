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

// 换算比用基点（BPS）：10000 = 1.0，也就是默认 1:1。
// 平台可按渠道配置，B/C 代理商不能改。合法范围 0.1x～10x。
const (
	DefaultIssueRatioBPS int64 = 10_000
	MinIssueRatioBPS     int64 = 1_000
	MaxIssueRatioBPS     int64 = 100_000
)

// ConvertQuota 把用户充值金额换成要发放的服务额度。
// grant = amount * bps / 10000，只用整数除法，避免浮点。
func ConvertQuota(amountMinor, bps int64) (int64, error) {
	if amountMinor <= 0 {
		return 0, ErrInvalidAmount
	}
	if err := ValidateIssueRatioBPS(bps); err != nil {
		return 0, err
	}
	if amountMinor > (1<<63-1)/bps {
		return 0, ErrInvalidAmount
	}
	grant := amountMinor * bps / DefaultIssueRatioBPS
	if grant <= 0 {
		return 0, ErrInvalidAmount
	}
	return grant, nil
}

func ValidateIssueRatioBPS(bps int64) error {
	if bps < MinIssueRatioBPS || bps > MaxIssueRatioBPS {
		return ErrInvalidIssueRatio
	}
	return nil
}

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
	VersionID            string
	Currency             string
	Raw                  json.RawMessage
	InputSell            int64
	OutputSell           int64
	ReasoningSell        int64
	InputCost            int64
	OutputCost           int64
	ReasoningCost        int64
	InputWholesale       int64
	OutputWholesale      int64
	VideoSecondSell      int64
	ImageCountSell       int64
	AudioSecondSell      int64
	VideoSecondCost      int64
	ImageCountCost       int64
	AudioSecondCost      int64
	VideoSecondWholesale int64
	ImageCountWholesale  int64
	AudioSecondWholesale int64
	inputSellRate        string
	outputSellRate       string
	reasoningSellRate    string
	inputCostRate        string
	outputCostRate       string
	reasoningCostRate    string
	inputWholesaleRate   string
	outputWholesaleRate  string
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
	if q.AudioSecondCost, err = ParseUSDToMinor(str("audio_second_cost")); err != nil {
		return q, err
	}
	if q.VideoSecondWholesale, err = ParseUSDToMinor(str("wholesale_video_second")); err != nil {
		return q, err
	}
	if q.ImageCountWholesale, err = ParseUSDToMinor(str("wholesale_image_count")); err != nil {
		return q, err
	}
	if q.AudioSecondWholesale, err = ParseUSDToMinor(str("wholesale_audio_second")); err != nil {
		return q, err
	}
	q.inputSellRate, q.outputSellRate = str("input", "customer_sell_input"), str("output", "customer_sell_output")
	q.reasoningSellRate = str("reasoning", "reasoning_output")
	if q.reasoningSellRate == "" {
		q.reasoningSellRate = q.outputSellRate
	}
	q.inputCostRate, q.outputCostRate = str("upstream_cost_input"), str("upstream_cost_output")
	q.reasoningCostRate = str("upstream_cost_reasoning")
	if q.reasoningCostRate == "" {
		q.reasoningCostRate = q.outputCostRate
	}
	q.inputWholesaleRate, q.outputWholesaleRate = str("wholesale_input"), str("wholesale_output")
	return q, nil
}

func (q Quote) CustomerMinor(prompt, completion int) int64 {
	return tokenAmountMinor(tokenPart{prompt, q.inputSellRate}, tokenPart{completion, q.outputSellRate})
}

// Media can also have token charges. Every explicitly priced token dimension
// must be reported; absence cannot be treated as a free, actual zero count.
func (q Quote) TokenUsageComplete(usage map[string]int) bool {
	var prices map[string]any
	if err := json.Unmarshal(q.Raw, &prices); err != nil {
		return false
	}
	for _, dimension := range []struct {
		fields []string
		usage  string
	}{
		{[]string{"input", "customer_sell_input"}, "prompt_tokens"},
		{[]string{"output", "customer_sell_output"}, "completion_tokens"},
		{[]string{"reasoning", "reasoning_output"}, "reasoning_tokens"},
	} {
		for _, field := range dimension.fields {
			value, found := prices[field]
			if !found {
				continue
			}
			rate, ok := new(big.Rat).SetString(fmt.Sprint(value))
			if !ok || rate.Sign() < 0 {
				return false
			}
			if rate.Sign() > 0 {
				if _, known := usage[dimension.usage]; !known {
					return false
				}
			}
			break
		}
	}
	return true
}

func (q Quote) CostMinor(prompt, completion int) int64 {
	return tokenAmountMinor(tokenPart{prompt, q.inputCostRate}, tokenPart{completion, q.outputCostRate})
}

type tokenPart struct {
	count int
	rate  string
}

// Round once after multiplying all token counts. A $0.40/M rate would round
// to zero if each token price were first converted to micro-USD.
func tokenAmountMinor(parts ...tokenPart) int64 {
	total := new(big.Rat)
	for _, part := range parts {
		if part.count <= 0 || part.rate == "" {
			continue
		}
		rate, ok := new(big.Rat).SetString(part.rate)
		if !ok {
			continue
		} // ParseQuote validated every rate.
		total.Add(total, rate.Mul(rate, new(big.Rat).SetInt64(int64(part.count))))
	}
	total.Mul(total, new(big.Rat).SetInt64(MinorPerUSD))
	quot, rem := new(big.Int).QuoRem(total.Num(), total.Denom(), new(big.Int))
	if new(big.Int).Lsh(rem, 1).Cmp(total.Denom()) >= 0 {
		quot.Add(quot, big.NewInt(1))
	}
	if !quot.IsInt64() {
		return 0
	}
	return quot.Int64()
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
	amt := tokenAmountMinor(tokenPart{usage["prompt_tokens"], q.inputSellRate}, tokenPart{billableVisibleCompletion(usage), q.outputSellRate}, tokenPart{usage["reasoning_tokens"], q.reasoningSellRate})
	media := int64(usage["video_seconds"])*q.VideoSecondSell +
		int64(usage["image_count"])*q.ImageCountSell +
		int64(usage["audio_seconds"])*q.AudioSecondSell
	amt += media * resolutionFactor(resolution) / 10
	return amt
}

// New native facts mark reasoning already included in completion. Historical
// unmarked facts retain their original separate-dimension interpretation.
func billableVisibleCompletion(usage map[string]int) int {
	n := usage["completion_tokens"]
	if usage["completion_includes_reasoning"] == 1 {
		n -= usage["reasoning_tokens"]
		if n < 0 {
			n = 0
		}
	}
	return n
}

// WholesaleCharge 用价格快照里的批发价，而不是客户价打七折。
func (q Quote) WholesaleCharge(usage map[string]int, resolution string) int64 {
	if usage == nil {
		usage = map[string]int{}
	}
	amt := tokenAmountMinor(tokenPart{usage["prompt_tokens"], q.inputWholesaleRate}, tokenPart{billableVisibleCompletion(usage), q.outputWholesaleRate}, tokenPart{usage["reasoning_tokens"], q.outputWholesaleRate})
	media := int64(usage["video_seconds"])*q.VideoSecondWholesale +
		int64(usage["image_count"])*q.ImageCountWholesale +
		int64(usage["audio_seconds"])*q.AudioSecondWholesale
	amt += media * resolutionFactor(resolution) / 10
	return amt
}

func (q Quote) MediaCost(usage map[string]int, resolution string) int64 {
	if usage == nil {
		usage = map[string]int{}
	}
	amt := tokenAmountMinor(tokenPart{usage["prompt_tokens"], q.inputCostRate}, tokenPart{billableVisibleCompletion(usage), q.outputCostRate}, tokenPart{usage["reasoning_tokens"], q.reasoningCostRate})
	media := int64(usage["video_seconds"])*q.VideoSecondCost +
		int64(usage["image_count"])*q.ImageCountCost +
		int64(usage["audio_seconds"])*q.AudioSecondCost
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
	// Reasoning may be reported separately. Include it in the estimate even
	// without an explicit reasoning parameter; this is not an absolute ceiling.
	base := q.Charge(map[string]int{"prompt_tokens": promptHint, "completion_tokens": maxTokens, "reasoning_tokens": maxTokens}, "")
	buf := base / 5
	if buf < 100 {
		buf = 100
	}
	return base + buf
}

// Retained for internal callers; it estimates both output dimensions, without
// promising that an upstream will never exceed the reservation.
func EstimateBoundedTextReserveMinor(q Quote, maxInput, maxOutput int) int64 {
	base := q.Charge(map[string]int{"prompt_tokens": maxInput, "completion_tokens": maxOutput, "reasoning_tokens": maxOutput}, "")
	buffer := base / 5
	if buffer < 100 {
		buffer = 100
	}
	return base + buffer
}
