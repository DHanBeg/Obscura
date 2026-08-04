package models

import "testing"

func TestTierToAccessLevel(t *testing.T) {
	cases := []struct {
		tier int
		want int
	}{
		{0, 1}, // bilinmeyen/hesapsız DID (bkz. airdrop.callerTier) — en kısıtlı erişim
		{1, 1},
		{2, 2},
		{3, 2},
		{4, 3},
		{5, 3},
		{6, 3}, // gelecekte tier genişlerse üst sınırda kalır
	}
	for _, c := range cases {
		got := TierToAccessLevel(c.tier)
		if got != c.want {
			t.Errorf("TierToAccessLevel(%d) = %d, want %d", c.tier, got, c.want)
		}
	}
}
