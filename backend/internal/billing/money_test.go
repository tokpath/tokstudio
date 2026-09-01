package billing

import "testing"

func TestConvertQuota(t *testing.T) {
	got, err := ConvertQuota(10*MinorPerUSD, DefaultIssueRatioBPS)
	if err != nil || got != 10*MinorPerUSD {
		t.Fatalf("default 1:1: %d %v", got, err)
	}
	got, err = ConvertQuota(10*MinorPerUSD, 12_000)
	if err != nil || got != 12*MinorPerUSD {
		t.Fatalf("1.2x of 10 USD: %d %v", got, err)
	}
	got, err = ConvertQuota(10*MinorPerUSD, 5_000)
	if err != nil || got != 5*MinorPerUSD {
		t.Fatalf("0.5x of 10 USD: %d %v", got, err)
	}
	if _, err := ConvertQuota(10*MinorPerUSD, 0); err != ErrInvalidIssueRatio {
		t.Fatalf("zero bps should be invalid, got %v", err)
	}
	if _, err := ConvertQuota(10*MinorPerUSD, 999); err != ErrInvalidIssueRatio {
		t.Fatalf("below min bps should be invalid, got %v", err)
	}
	if _, err := ConvertQuota(10*MinorPerUSD, 100_001); err != ErrInvalidIssueRatio {
		t.Fatalf("above max bps should be invalid, got %v", err)
	}
	if _, err := ConvertQuota(0, DefaultIssueRatioBPS); err != ErrInvalidAmount {
		t.Fatalf("zero amount should be invalid, got %v", err)
	}
}

func TestParseUSDToMinor(t *testing.T) {
	got, err := ParseUSDToMinor("0.000001")
	if err != nil || got != 1 {
		t.Fatalf("input token price: %d %v", got, err)
	}
	got, err = ParseUSDToMinor("10")
	if err != nil || got != 10*MinorPerUSD {
		t.Fatalf("10 USD: %d %v", got, err)
	}
	q, err := ParseQuote("price_echo", []byte(`{"input":"0.000001","output":"0.000002"}`))
	if err != nil {
		t.Fatal(err)
	}
	if q.CustomerMinor(8, 4) != 16 {
		t.Fatalf("echo usage should be 16 micro, got %d", q.CustomerMinor(8, 4))
	}
	if q.Charge(map[string]int{"prompt_tokens": 8, "completion_tokens": 4, "reasoning_tokens": 3}, "") != 22 {
		t.Fatalf("reasoning tokens should bill at output price: %d", q.Charge(map[string]int{"prompt_tokens": 8, "completion_tokens": 4, "reasoning_tokens": 3}, ""))
	}
	custom, err := ParseQuote("price_wholesale", []byte(`{"input":"0.000001","output":"0.000002","wholesale_input":"0.000003","wholesale_output":"0.000004"}`))
	if err != nil {
		t.Fatal(err)
	}
	if custom.WholesaleCharge(map[string]int{"prompt_tokens": 8, "completion_tokens": 4}, "") != 40 {
		t.Fatalf("wholesale snapshot should beat customer*0.7: %d", custom.WholesaleCharge(map[string]int{"prompt_tokens": 8, "completion_tokens": 4}, ""))
	}
	if custom.Charge(map[string]int{"prompt_tokens": 8, "completion_tokens": 4}, "")*7/10 == custom.WholesaleCharge(map[string]int{"prompt_tokens": 8, "completion_tokens": 4}, "") {
		t.Fatal("wholesale snapshot must not collapse to a 70% haircut")
	}
	if EstimateReserveMinor(q, 8, 4) < 16 {
		t.Fatal("reserve must cover actual usage")
	}
	media, err := ParseQuote("price_seedance", []byte(`{"currency":"USD","video_second":"0.01","image_count":"0.02","audio_second":"0.002"}`))
	if err != nil {
		t.Fatal(err)
	}
	// 5 秒 720p：0.01 USD/秒 * 5 * 1.0 倍率 = 50000 micro-USD
	if media.Charge(map[string]int{"video_seconds": 5}, "720p") != 50_000 {
		t.Fatalf("720p video charge: %d", media.Charge(map[string]int{"video_seconds": 5}, "720p"))
	}
	if media.Charge(map[string]int{"video_seconds": 5}, "1080p") != 75_000 {
		t.Fatalf("1080p uses 1.5x: %d", media.Charge(map[string]int{"video_seconds": 5}, "1080p"))
	}
	if EstimateMediaReserveMinor(media, 5, 0, "720p", false) < 50_000 {
		t.Fatal("media reserve must cover actual usage")
	}
}
