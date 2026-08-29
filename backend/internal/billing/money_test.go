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
}
