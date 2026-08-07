package bridge

import (
	"math/big"
	"testing"
)

func TestEthWeiToDotPlanck_KnownVectors(t *testing.T) {
	tests := []struct {
		name        string
		weiAmount   string
		dotDecimals uint32
		wantPlanck  string
		wantDust    string
	}{
		{
			name:        "1.0 OBS (18 dec) -> Paseo 10 dec = 1.0 PAS exactly",
			weiAmount:   "1000000000000000000", // 1e18
			dotDecimals: 10,
			wantPlanck:  "10000000000", // 1e10
			wantDust:    "0",
		},
		{
			name:        "0.1 OBS -> 0.1 PAS exactly (this session's real transfer amount)",
			weiAmount:   "100000000000000000", // 0.1e18
			dotDecimals: 10,
			wantPlanck:  "1000000000", // 0.1e10
			wantDust:    "0",
		},
		{
			name:        "1.5 OBS -> 1.5 PAS exactly",
			weiAmount:   "1500000000000000000",
			dotDecimals: 10,
			wantPlanck:  "15000000000",
			wantDust:    "0",
		},
		{
			name:        "amount with sub-planck dust gets truncated, dust reported",
			weiAmount:   "1000000000000000123", // 1 OBS + 123 wei of dust
			dotDecimals: 10,
			wantPlanck:  "10000000000",
			wantDust:    "123",
		},
		{
			name:        "dotDecimals == ethDecimals: no scaling",
			weiAmount:   "42",
			dotDecimals: 18,
			wantPlanck:  "42",
			wantDust:    "0",
		},
		{
			name:        "dotDecimals > ethDecimals: multiply, never truncates",
			weiAmount:   "7",
			dotDecimals: 20,
			wantPlanck:  "700",
			wantDust:    "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wei, ok := new(big.Int).SetString(tt.weiAmount, 10)
			if !ok {
				t.Fatalf("bad test fixture weiAmount %q", tt.weiAmount)
			}
			planck, dust, err := EthWeiToDotPlanck(wei, tt.dotDecimals)
			if err != nil {
				t.Fatalf("EthWeiToDotPlanck: %v", err)
			}
			if planck.String() != tt.wantPlanck {
				t.Errorf("planck = %s, want %s", planck.String(), tt.wantPlanck)
			}
			if dust.String() != tt.wantDust {
				t.Errorf("dust = %s, want %s", dust.String(), tt.wantDust)
			}
		})
	}
}

func TestEthWeiToDotPlanck_AllDustIsError(t *testing.T) {
	// 5 wei at 18 decimals, converted to 10 decimals, divides by 10^8 -> 0.
	// A zero-amount transfer must be rejected, not silently sent.
	wei := big.NewInt(5)
	_, _, err := EthWeiToDotPlanck(wei, 10)
	if err == nil {
		t.Fatal("expected error for amount that rounds to zero planck, got nil")
	}
}

func TestEthWeiToDotPlanck_NegativeAmountRejected(t *testing.T) {
	wei := big.NewInt(-1)
	_, _, err := EthWeiToDotPlanck(wei, 10)
	if err == nil {
		t.Fatal("expected error for negative amount, got nil")
	}
}

func TestEthWeiToDotPlanck_NilAmountRejected(t *testing.T) {
	_, _, err := EthWeiToDotPlanck(nil, 10)
	if err == nil {
		t.Fatal("expected error for nil amount, got nil")
	}
}
