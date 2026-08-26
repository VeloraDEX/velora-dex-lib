package builder

import (
	"math/big"
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

func TestSlippageFactorString(t *testing.T) {
	// Expected values are what `new BigNumber(a).div(b)` produces under
	// BigNumber.js defaults (DECIMAL_PLACES 20, ROUND_HALF_UP).
	cases := []struct {
		name        string
		minMax      string
		denominator string
		want        string
		wantErr     bool
	}{
		{name: "no slippage", minMax: "1", denominator: "1", want: "1"},
		{name: "typical sell", minMax: "995", denominator: "1000", want: "0.995"},
		{name: "typical buy", minMax: "1005", denominator: "1000", want: "1.005"},
		{name: "zero numerator", minMax: "0", denominator: "1000", want: "0"},
		{
			name: "repeating decimal truncates at 20 places",
			// 1/3 = 0.333... — the 21st digit is 3, so half-up rounds down.
			minMax: "1", denominator: "3", want: "0.33333333333333333333",
		},
		{
			name: "exact within 20 places",
			// 3/16 = 0.1875, no rounding involved.
			minMax: "3", denominator: "16", want: "0.1875",
		},
		{
			name: "exactly half a unit in the last kept place rounds up",
			// 1/2e20 is 5e-21: the discarded part is exactly half of 1e-20, the
			// one input where half-up and truncation disagree. BigNumber.js
			// prints it as 1e-20; the wire format has to stay plain decimal
			// because the receiving schema is ^\d+(\.\d+)?$.
			minMax: "1", denominator: "200000000000000000000",
			want: "0.00000000000000000001",
		},
		{
			name: "large real-world amounts",
			// 0.995 of a 1e26 destAmount.
			minMax: "99500000000000000000000000", denominator: "100000000000000000000000000",
			want: "0.995",
		},
		{
			name: "rounds up at the 21st digit",
			// 2/3 = 0.666... — the 21st digit is 6, so half-up rounds the 20th up.
			minMax: "2", denominator: "3", want: "0.66666666666666666667",
		},
		{name: "zero denominator is an error", minMax: "1", denominator: "0", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			minMax, _ := new(big.Int).SetString(tc.minMax, 10)
			denominator, _ := new(big.Int).SetString(tc.denominator, 10)

			got, err := slippageFactorString(minMax, denominator)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("slippageFactorString(%s, %s) = %q, want error", tc.minMax, tc.denominator, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("slippageFactorString(%s, %s) error = %v", tc.minMax, tc.denominator, err)
			}
			if got != tc.want {
				t.Fatalf("slippageFactorString(%s, %s) = %q, want %q", tc.minMax, tc.denominator, got, tc.want)
			}
		})
	}
}

func TestResolveSlippageFactor_DenominatorFollowsSide(t *testing.T) {
	sell := BuildRequest{
		MinMaxAmount: "495",
		PriceRoute: PriceRoute{
			Side:       resolved.SideSell,
			SrcAmount:  "1000",
			DestAmount: "500",
		},
	}
	got, err := resolveSlippageFactor(sell)
	if err != nil {
		t.Fatalf("SELL error = %v", err)
	}
	if got != "0.99" {
		t.Fatalf("SELL slippage factor = %q, want 0.99 (minMaxAmount / destAmount)", got)
	}

	buy := sell
	buy.PriceRoute.Side = resolved.SideBuy
	buy.MinMaxAmount = "1010"
	got, err = resolveSlippageFactor(buy)
	if err != nil {
		t.Fatalf("BUY error = %v", err)
	}
	if got != "1.01" {
		t.Fatalf("BUY slippage factor = %q, want 1.01 (minMaxAmount / srcAmount)", got)
	}
}

func preProcessTestRequest() BuildRequest {
	return BuildRequest{
		MinMaxAmount: "495",
		UserAddress:  "0xAAAA000000000000000000000000000000000001",
		TxOrigin:     "0xBBBB000000000000000000000000000000000002",
		Special:      true,
		PriceRoute: PriceRoute{
			Network:    1,
			Side:       resolved.SideSell,
			SrcAmount:  "1000",
			DestAmount: "500",
			Partner:    ptr("wallet"),
		},
	}
}

func preProcessTestSwap() PriceRouteSwap {
	return PriceRouteSwap{
		SrcToken:     testApprovalSrcToken,
		DestToken:    testApprovalDestToken,
		SrcDecimals:  ptr(6),
		DestDecimals: ptr(18),
	}
}

func TestBuildGetDexParamPreProcess(t *testing.T) {
	req := preProcessTestRequest()
	swapExchange := PriceRouteSwapExchange{Exchange: "GenericRFQ", SrcAmount: "1000", DestAmount: "500"}
	callParams := genericDexCallParams{recipient: testApprovalTargetExchange}

	got, err := buildGetDexParamPreProcess(req, preProcessTestSwap(), swapExchange, callParams, testApprovalWETH)
	if err != nil {
		t.Fatalf("buildGetDexParamPreProcess() error = %v", err)
	}
	if got == nil {
		t.Fatal("buildGetDexParamPreProcess() = nil, want a payload")
	}

	want := GetDexParamPreProcess{
		SrcToken:                 PreProcessToken{Address: testApprovalSrcToken, Decimals: 6},
		DestToken:                PreProcessToken{Address: testApprovalDestToken, Decimals: 18},
		SrcAmount:                "1000",
		DestAmount:               "500",
		SlippageFactor:           "0.99",
		TxOrigin:                 "0xbbbb000000000000000000000000000000000002",
		UserAddress:              "0xaaaa000000000000000000000000000000000001",
		ExecutionContractAddress: testApprovalWETH,
		Recipient:                testApprovalTargetExchange,
		Version:                  versionV6,
		Partner:                  "wallet",
		Special:                  true,
	}
	if *got != want {
		t.Fatalf("preProcess =\n%+v\nwant\n%+v", *got, want)
	}
}

func TestBuildGetDexParamPreProcess_TxOriginFallsBackToUserAddress(t *testing.T) {
	req := preProcessTestRequest()
	req.TxOrigin = ""

	got, err := buildGetDexParamPreProcess(
		req, preProcessTestSwap(), PriceRouteSwapExchange{SrcAmount: "1000", DestAmount: "500"},
		genericDexCallParams{}, testApprovalWETH,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TxOrigin != got.UserAddress {
		t.Fatalf("TxOrigin = %q, want the user address %q", got.TxOrigin, got.UserAddress)
	}
}

func TestBuildGetDexParamPreProcess_FailsWhenDecimalsMissing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mut     func(*PriceRouteSwap)
		wantErr string
	}{
		{
			name: "src decimals absent", wantErr: "srcDecimals",
			mut: func(s *PriceRouteSwap) { s.SrcDecimals = nil },
		},
		{
			name: "dest decimals absent", wantErr: "destDecimals",
			mut: func(s *PriceRouteSwap) { s.DestDecimals = nil },
		},
		{
			name: "negative decimals", wantErr: "must not be negative",
			mut: func(s *PriceRouteSwap) { s.SrcDecimals = ptr(-1) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			swap := preProcessTestSwap()
			tc.mut(&swap)

			got, err := buildGetDexParamPreProcess(
				preProcessTestRequest(), swap,
				PriceRouteSwapExchange{SrcAmount: "1000", DestAmount: "500"},
				genericDexCallParams{}, testApprovalWETH,
			)
			if err == nil {
				t.Fatalf("preProcess = %+v, want an error rather than a quote at the wrong size", *got)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to name %q", err, tc.wantErr)
			}
		})
	}
}

func TestWithPreProcess_DoesNotShareTheRouteLevelOptions(t *testing.T) {
	nowTimestampMs := uint64(1_700_000_123_456)
	shared := &GetDexParamOptions{NowTimestampMs: &nowTimestampMs}

	first := withPreProcess(shared, &GetDexParamPreProcess{SrcAmount: "1"})
	second := withPreProcess(shared, &GetDexParamPreProcess{SrcAmount: "2"})

	if shared.PreProcess != nil {
		t.Fatal("withPreProcess must not write into the shared route-level options")
	}
	if first.PreProcess.SrcAmount != "1" || second.PreProcess.SrcAmount != "2" {
		t.Fatalf("per-leg contexts collided: %q and %q", first.PreProcess.SrcAmount, second.PreProcess.SrcAmount)
	}
	if first.NowTimestampMs == nil || *first.NowTimestampMs != nowTimestampMs {
		t.Fatal("route-level nowTimestampMs must survive the clone")
	}
}

func TestWithPreProcess_WorksWithoutRouteLevelOptions(t *testing.T) {
	got := withPreProcess(nil, &GetDexParamPreProcess{SrcAmount: "1"})
	if got == nil || got.PreProcess == nil {
		t.Fatalf("withPreProcess(nil, payload) = %+v, want the payload carried on fresh options", got)
	}
	if got.NowTimestampMs != nil {
		t.Fatal("nowTimestampMs must stay absent when the caller did not set it")
	}
}
