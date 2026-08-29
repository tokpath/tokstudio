package billing

import "testing"

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
