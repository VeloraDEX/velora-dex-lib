package builder

import (
	"context"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

type recordingDexRegistry struct {
	encoder *recordingDexEncoder
}

func (r recordingDexRegistry) GetDexEncoder(_ context.Context, _ int, _ string) (DexEncoder, error) {
	return r.encoder, nil
}

type recordingDexEncoder struct {
	got DexParamInput
	all []DexParamInput
}

func (r *recordingDexEncoder) NeedWrapNative(context.Context, NeedWrapNativeInput) (bool, error) {
	return true, nil
}

func (r *recordingDexEncoder) GetDexParam(_ context.Context, input DexParamInput) (DexExchangeParam, error) {
	r.got = input
	r.all = append(r.all, input)
	return DexExchangeParam{
		NeedWrapNative:      true,
		ExchangeData:        "0x12345678",
		TargetExchange:      testApprovalTargetExchange,
		DexFuncHasRecipient: true,
	}, nil
}

func TestBuildResolvedLegs_ForwardsGetDexParamOptions(t *testing.T) {
	priceRoute, routePlan, _ := testApprovalRouteAndLeg()
	nowTimestampMs := uint64(1_700_000_123_456)
	options := &GetDexParamOptions{NowTimestampMs: &nowTimestampMs}
	recorder := &recordingDexEncoder{}

	_, err := buildResolvedLegs(
		context.Background(),
		BuildRequest{
			PriceRoute:         priceRoute,
			MinMaxAmount:       "1",
			GetDexParamOptions: options,
		},
		Deps{
			DexRegistry: recordingDexRegistry{encoder: recorder},
		},
		resolved.EncodingContext{
			AugustusV6Address:         resolved.NullAddress,
			WrappedNativeTokenAddress: testApprovalWETH,
		},
		routePlan,
		resolved.Executor01,
		testApprovalTargetExchange,
	)
	if err != nil {
		t.Fatalf("buildResolvedLegs() error = %v", err)
	}
	// A per-leg clone reaches the encoder, not the caller's object — but the
	// route-level part of it has to survive that clone.
	if recorder.got.Options == options {
		t.Fatal("DexParamInput.Options is the caller's object; the per-leg clone is missing")
	}
	if recorder.got.Options.NowTimestampMs == nil || *recorder.got.Options.NowTimestampMs != nowTimestampMs {
		t.Fatalf("NowTimestampMs not preserved: %+v", recorder.got.Options)
	}
	if options.PreProcess != nil {
		t.Fatal("the caller's options must not be mutated")
	}
}

// twoSwapRouteAndPlan is a chained A->B->C sell route with per-swap decimals, so
// every leg has a distinct preprocess context to be confused with another's.
func twoSwapRouteAndPlan() (PriceRoute, resolved.RoutePlan) {
	const (
		tokenA = testApprovalSrcToken
		tokenB = testApprovalTargetExchange
		tokenC = testApprovalDestToken
	)
	first := PriceRouteSwap{
		SrcToken: tokenA, DestToken: tokenB,
		SrcAmount: ptr(resolved.DecimalString("1000")), DestAmount: ptr(resolved.DecimalString("2000")),
		SrcDecimals: ptr(6), DestDecimals: ptr(8),
		SwapExchanges: []PriceRouteSwapExchange{
			{Exchange: "first", Percent: 100, SrcAmount: "1000", DestAmount: "2000"},
		},
	}
	second := PriceRouteSwap{
		SrcToken: tokenB, DestToken: tokenC,
		SrcAmount: ptr(resolved.DecimalString("2000")), DestAmount: ptr(resolved.DecimalString("500")),
		SrcDecimals: ptr(8), DestDecimals: ptr(18),
		SwapExchanges: []PriceRouteSwapExchange{
			{Exchange: "second", Percent: 100, SrcAmount: "2000", DestAmount: "500"},
		},
	}

	priceRoute := PriceRoute{
		Network:        1,
		ContractMethod: resolved.ContractMethodSwapExactAmountIn,
		Side:           resolved.SideSell,
		SrcToken:       tokenA,
		DestToken:      tokenC,
		SrcAmount:      "1000",
		DestAmount:     "500",
		Partner:        ptr("wallet"),
		BestRoute:      []PriceRouteRoute{{Percent: 100, Swaps: []PriceRouteSwap{first, second}}},
	}

	routePlan := resolved.RoutePlan{Routes: []resolved.RoutePlanRoute{{
		Percent: 100,
		Swaps: []resolved.RoutePlanSwap{
			{
				SrcToken: tokenA, DestToken: tokenB, SrcAmount: "1000", DestAmount: "2000",
				SwapExchanges: []resolved.RoutePlanSwapExchange{
					{Exchange: "first", Percent: 100, SrcAmount: "1000", DestAmount: "2000"},
				},
			},
			{
				SrcToken: tokenB, DestToken: tokenC, SrcAmount: "2000", DestAmount: "500",
				SwapExchanges: []resolved.RoutePlanSwapExchange{
					{Exchange: "second", Percent: 100, SrcAmount: "2000", DestAmount: "500"},
				},
			},
		},
	}}}

	return priceRoute, routePlan
}

func TestBuildResolvedLegs_PreProcessIsPerLeg(t *testing.T) {
	priceRoute, routePlan := twoSwapRouteAndPlan()
	nowTimestampMs := uint64(1_700_000_123_456)
	options := &GetDexParamOptions{NowTimestampMs: &nowTimestampMs}
	recorder := &recordingDexEncoder{}

	_, err := buildResolvedLegs(
		context.Background(),
		BuildRequest{
			PriceRoute:         priceRoute,
			MinMaxAmount:       "495",
			UserAddress:        "0xAAAA000000000000000000000000000000000001",
			TxOrigin:           "0xBBBB000000000000000000000000000000000002",
			Special:            true,
			GetDexParamOptions: options,
		},
		Deps{DexRegistry: recordingDexRegistry{encoder: recorder}},
		resolved.EncodingContext{
			AugustusV6Address:         resolved.NullAddress,
			WrappedNativeTokenAddress: testApprovalWETH,
		},
		routePlan,
		resolved.Executor01,
		testApprovalTargetExchange,
	)
	if err != nil {
		t.Fatalf("buildResolvedLegs() error = %v", err)
	}
	if len(recorder.all) != 2 {
		t.Fatalf("recorded %d legs, want 2", len(recorder.all))
	}

	if options.PreProcess != nil {
		t.Fatal("the shared route-level options must not be mutated")
	}

	first, second := recorder.all[0].Options.PreProcess, recorder.all[1].Options.PreProcess
	if first == nil || second == nil {
		t.Fatalf("both legs need a preprocess context: %+v / %+v", first, second)
	}
	if first == second {
		t.Fatal("both legs share one preprocess context; the per-leg clone is missing")
	}

	// Swap-level tokens with their own decimals, not the route's endpoints.
	if first.SrcToken.Decimals != 6 || first.DestToken.Decimals != 8 {
		t.Fatalf("first leg decimals = %d/%d, want 6/8", first.SrcToken.Decimals, first.DestToken.Decimals)
	}
	if second.SrcToken.Decimals != 8 || second.DestToken.Decimals != 18 {
		t.Fatalf("second leg decimals = %d/%d, want 8/18", second.SrcToken.Decimals, second.DestToken.Decimals)
	}
	// The leg's real amounts, not what getDexParam receives: on SELL the
	// encoder's own destAmount is "1".
	if second.SrcAmount != "2000" || second.DestAmount != "500" {
		t.Fatalf("second leg amounts = %s/%s, want 2000/500", second.SrcAmount, second.DestAmount)
	}
	if recorder.all[1].DestAmount != "1" {
		t.Fatalf("precondition: encoder destAmount = %s, want the SELL placeholder 1", recorder.all[1].DestAmount)
	}
	// Recipient is per-leg too: a non-final leg pays into the executor.
	if first.Recipient == second.Recipient {
		t.Fatalf("both legs report recipient %s; the per-leg value was not read", first.Recipient)
	}

	for i, preProcess := range []*GetDexParamPreProcess{first, second} {
		if preProcess.SlippageFactor != "0.99" {
			t.Fatalf("leg %d slippageFactor = %q, want 0.99", i, preProcess.SlippageFactor)
		}
		if preProcess.TxOrigin != "0xbbbb000000000000000000000000000000000002" {
			t.Fatalf("leg %d txOrigin = %q", i, preProcess.TxOrigin)
		}
		if preProcess.Partner != "wallet" || !preProcess.Special {
			t.Fatalf("leg %d partner/special = %q/%t", i, preProcess.Partner, preProcess.Special)
		}
		if preProcess.Version != versionV6 {
			t.Fatalf("leg %d version = %q, want %q", i, preProcess.Version, versionV6)
		}
		if recorder.all[i].Options.NowTimestampMs == nil ||
			*recorder.all[i].Options.NowTimestampMs != nowTimestampMs {
			t.Fatalf("leg %d lost the route-level nowTimestampMs", i)
		}
	}
}
