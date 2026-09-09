package identity

import "testing"

func TestValidateChannelParent(t *testing.T) {
	if err := ValidateChannelParent(ChannelTypeA, ChannelTypeB); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChannelParent(ChannelTypeA, ChannelTypeC); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChannelParent(ChannelTypeC, ChannelTypeB); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChannelParent(ChannelTypeC, ChannelTypeC); err != ErrChannelImmutable {
		t.Fatalf("C cannot create C: %v", err)
	}
	if err := ValidateChannelParent(ChannelTypeB, ChannelTypeB); err != ErrChannelImmutable {
		t.Fatalf("B cannot create: %v", err)
	}
}

func TestPoolChannelID(t *testing.T) {
	if got := PoolChannelID(ChannelTypeB, "b1", "c1", ChannelTypeC); got != "b1" {
		t.Fatalf("B under C keeps own pool: %s", got)
	}
	if got := PoolChannelID(ChannelTypeB, "b1", OfficialChannelID, ChannelTypeA); got != "b1" {
		t.Fatalf("B under A keeps own pool: %s", got)
	}
}

func TestMarketChannelID(t *testing.T) {
	if got := MarketChannelID(ChannelTypeB, "b1", "c1", ChannelTypeC); got != "c1" {
		t.Fatalf("B under C uses C market: %s", got)
	}
	if got := MarketChannelID(ChannelTypeC, "c1", OfficialChannelID, ChannelTypeA); got != "c1" {
		t.Fatalf("C is own market: %s", got)
	}
	if got := MarketChannelID(ChannelTypeB, "b1", OfficialChannelID, ChannelTypeA); got != "" {
		t.Fatalf("B under A uses platform: %s", got)
	}
}
