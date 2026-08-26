package builder

import (
	"context"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const (
	fbTestExecutor resolved.Address = "0x7777777777777777777777777777777777777777"
	fbTestAugustus resolved.Address = "0x6666666666666666666666666666666666666666"
	fbTestTarget   resolved.Address = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

type fallbackTestEncoder struct {
	param        DexExchangeParam
	needWrap     bool
	gotDexParams []DexParamInput
}

func (e *fallbackTestEncoder) NeedWrapNative(context.Context, NeedWrapNativeInput) (bool, error) {
	return e.needWrap, nil
}

func (e *fallbackTestEncoder) GetDexParam(_ context.Context, input DexParamInput) (DexExchangeParam, error) {
	e.gotDexParams = append(e.gotDexParams, input)
	return e.param, nil
}

type fallbackTestRegistry struct {
	encoders map[string]*fallbackTestEncoder
}

func (r fallbackTestRegistry) GetDexEncoder(_ context.Context, _ int, dexKey string) (DexEncoder, error) {
	return r.encoders[dexKey], nil
}

func fbTestEncoder(needWrap, hasRecipient bool, target resolved.Address) *fallbackTestEncoder {
	return &fallbackTestEncoder{
		needWrap: needWrap,
		param: DexExchangeParam{
			NeedWrapNative:      needWrap,
			ExchangeData:        "0x12345678",
			TargetExchange:      target,
			DexFuncHasRecipient: hasRecipient,
		},
	}
}

func fbTestBuild(
	t *testing.T,
	executorType resolved.ExecutorType,
	primaryEncoder, fallbackEncoder *fallbackTestEncoder,
	destToken resolved.Address,
) []resolvedLegWithWeth {
	t.Helper()

	priceRoute, routePlan, _ := testApprovalRouteAndLeg()
	priceRoute.DestToken = destToken
	priceRoute.BestRoute[0].Swaps[0].DestToken = destToken
	routePlan.Routes[0].Swaps[0].DestToken = destToken
	priceRoute.BestRoute[0].Swaps[0].SwapExchanges[0].Fallback = &PriceRouteSwapExchange{
		Exchange:   "fbdex",
		Percent:    100,
		SrcAmount:  "123",
		DestAmount: "400",
	}

	legs, err := buildResolvedLegs(
		context.Background(),
		BuildRequest{PriceRoute: priceRoute, MinMaxAmount: "1"},
		Deps{DexRegistry: fallbackTestRegistry{encoders: map[string]*fallbackTestEncoder{
			"test":  primaryEncoder,
			"fbdex": fallbackEncoder,
		}}},
		resolved.EncodingContext{
			AugustusV6Address:         fbTestAugustus,
			WrappedNativeTokenAddress: testApprovalWETH,
		},
		routePlan,
		executorType,
		fbTestExecutor,
	)
	if err != nil {
		t.Fatalf("buildResolvedLegs() error = %v", err)
	}
	if len(legs) != 1 {
		t.Fatalf("expected 1 leg, got %d", len(legs))
	}
	return legs
}

func TestBuildResolvedLegsBuildsFallbackParam(t *testing.T) {
	t.Run("recipient-capable primary: fallback delivers to Augustus, no dest-receiver mark", func(t *testing.T) {
		primaryEncoder := fbTestEncoder(false, true, testApprovalTargetExchange)
		fallbackEncoder := fbTestEncoder(false, true, fbTestTarget)
		legs := fbTestBuild(t, resolved.Executor01, primaryEncoder, fallbackEncoder, testApprovalDestToken)

		fallbackParam := legs[0].leg.ExchangeParam.FallbackParam
		if fallbackParam == nil {
			t.Fatal("fallback param was not built")
		}
		if fallbackParam.TargetExchange != fbTestTarget {
			t.Errorf("fallback target = %s, want %s", fallbackParam.TargetExchange, fbTestTarget)
		}
		if fallbackParam.ExecutorIsDestReceiver {
			t.Error("executorIsDestReceiver must never be set behind a recipient-capable primary")
		}
		if len(fallbackEncoder.gotDexParams) != 1 {
			t.Fatalf("fallback GetDexParam calls = %d, want 1", len(fallbackEncoder.gotDexParams))
		}
		if got := fallbackEncoder.gotDexParams[0].Recipient; got != fbTestAugustus {
			t.Errorf("fallback recipient = %s, want Augustus %s", got, fbTestAugustus)
		}
	})

	t.Run("false-recipient Executor01 primary: fallback redirected onto the executor", func(t *testing.T) {
		primaryEncoder := fbTestEncoder(false, false, testApprovalTargetExchange)
		fallbackEncoder := fbTestEncoder(false, true, fbTestTarget)
		legs := fbTestBuild(t, resolved.Executor01, primaryEncoder, fallbackEncoder, testApprovalDestToken)

		fallbackParam := legs[0].leg.ExchangeParam.FallbackParam
		if fallbackParam == nil {
			t.Fatal("fallback param was not built")
		}
		if !fallbackParam.ExecutorIsDestReceiver {
			t.Error("executorIsDestReceiver must be set behind a false-recipient Executor01 primary")
		}
		if got := fallbackEncoder.gotDexParams[0].Recipient; got != fbTestExecutor {
			t.Errorf("fallback recipient = %s, want executor %s", got, fbTestExecutor)
		}
		// The primary keeps its normal recipient rules.
		if got := primaryEncoder.gotDexParams[0].Recipient; got != fbTestAugustus {
			t.Errorf("primary recipient = %s, want Augustus %s", got, fbTestAugustus)
		}
	})

	t.Run("false-recipient Executor02 primary: no redirect (per-branch forwards)", func(t *testing.T) {
		primaryEncoder := fbTestEncoder(false, false, testApprovalTargetExchange)
		fallbackEncoder := fbTestEncoder(false, true, fbTestTarget)
		legs := fbTestBuild(t, resolved.Executor02, primaryEncoder, fallbackEncoder, testApprovalDestToken)

		fallbackParam := legs[0].leg.ExchangeParam.FallbackParam
		if fallbackParam.ExecutorIsDestReceiver {
			t.Error("executorIsDestReceiver is Executor01-only")
		}
		if got := fallbackEncoder.gotDexParams[0].Recipient; got != fbTestAugustus {
			t.Errorf("fallback recipient = %s, want Augustus %s", got, fbTestAugustus)
		}
	})

	t.Run("ETH-dest hop: fallback stays on the executor", func(t *testing.T) {
		primaryEncoder := fbTestEncoder(false, true, testApprovalTargetExchange)
		fallbackEncoder := fbTestEncoder(false, true, fbTestTarget)
		fbTestBuild(t, resolved.Executor01, primaryEncoder, fallbackEncoder, resolved.NativeTokenAddress)

		if got := primaryEncoder.gotDexParams[0].Recipient; got != fbTestAugustus {
			t.Errorf("primary recipient = %s, want Augustus %s", got, fbTestAugustus)
		}
		if got := fallbackEncoder.gotDexParams[0].Recipient; got != fbTestExecutor {
			t.Errorf("fallback recipient = %s, want executor %s", got, fbTestExecutor)
		}
	})

	t.Run("fallback wrap amounts count toward the shared WETH plan", func(t *testing.T) {
		primaryEncoder := fbTestEncoder(false, true, testApprovalTargetExchange)
		fallbackEncoder := fbTestEncoder(true, true, fbTestTarget)

		priceRoute, routePlan, _ := testApprovalRouteAndLeg()
		priceRoute.SrcToken = resolved.NativeTokenAddress
		priceRoute.BestRoute[0].Swaps[0].SrcToken = resolved.NativeTokenAddress
		routePlan.Routes[0].Swaps[0].SrcToken = resolved.NativeTokenAddress
		priceRoute.BestRoute[0].Swaps[0].SwapExchanges[0].Fallback = &PriceRouteSwapExchange{
			Exchange:   "fbdex",
			Percent:    100,
			SrcAmount:  "123",
			DestAmount: "400",
		}

		legs, err := buildResolvedLegs(
			context.Background(),
			BuildRequest{PriceRoute: priceRoute, MinMaxAmount: "1"},
			Deps{DexRegistry: fallbackTestRegistry{encoders: map[string]*fallbackTestEncoder{
				"test":  primaryEncoder,
				"fbdex": fallbackEncoder,
			}}},
			resolved.EncodingContext{
				AugustusV6Address:         fbTestAugustus,
				WrappedNativeTokenAddress: testApprovalWETH,
			},
			routePlan,
			resolved.Executor01,
			fbTestExecutor,
		)
		if err != nil {
			t.Fatalf("buildResolvedLegs() error = %v", err)
		}
		// Only the wrap-needing fallback deposits; the raw primary does not.
		if got := legs[0].wethDeposit.String(); got != "123" {
			t.Errorf("wethDeposit = %s, want 123 (fallback slice)", got)
		}
	})
}

func TestApprovalDecisionsRouteToFallback(t *testing.T) {
	priceRoute, routePlan, resolvedLegs := testApprovalRouteAndLeg()
	spender := testApprovalTargetExchange
	fallbackSpender := fbTestTarget
	resolvedLegs[0].ExchangeParam.Spender = &spender
	resolvedLegs[0].ExchangeParam.FallbackParam = &resolved.DexExchangeBuildParam{
		NeedWrapNative:      resolved.RawBool{Value: false, Valid: true, Present: true},
		ExchangeData:        "0x12345678",
		TargetExchange:      fbTestTarget,
		DexFuncHasRecipient: true,
		Spender:             &fallbackSpender,
	}

	context := resolved.EncodingContext{
		AugustusV6Address:         fbTestAugustus,
		WrappedNativeTokenAddress: testApprovalWETH,
	}
	requests, err := buildDexExchangeApprovalRequests(context, priceRoute, routePlan, resolvedLegs)
	if err != nil {
		t.Fatalf("buildDexExchangeApprovalRequests() error = %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected primary + fallback approval requests, got %d", len(requests))
	}
	if requests[1].isFallback != true || requests[1].request.Target != fbTestTarget {
		t.Fatalf("second request must be the fallback's: %+v", requests[1])
	}

	t.Run("fallback not approved gets its own approveData", func(t *testing.T) {
		out, err := applyDexExchangeApprovalDecisions(resolvedLegs, requests, []bool{true, false})
		if err != nil {
			t.Fatalf("applyDexExchangeApprovalDecisions() error = %v", err)
		}
		if out[0].ExchangeParam.ApproveData != nil {
			t.Error("already-approved primary must not carry approveData")
		}
		fallbackParam := out[0].ExchangeParam.FallbackParam
		if fallbackParam.ApproveData == nil || fallbackParam.ApproveData.Target != fbTestTarget {
			t.Errorf("fallback approveData = %+v, want target %s", fallbackParam.ApproveData, fbTestTarget)
		}
		if fallbackParam.Spender != nil {
			t.Error("fallback spender must be cleared after planning")
		}
	})

	t.Run("approved fallback carries no approveData", func(t *testing.T) {
		out, err := applyDexExchangeApprovalDecisions(resolvedLegs, requests, []bool{false, true})
		if err != nil {
			t.Fatalf("applyDexExchangeApprovalDecisions() error = %v", err)
		}
		if out[0].ExchangeParam.ApproveData == nil {
			t.Error("unapproved primary must carry approveData")
		}
		if out[0].ExchangeParam.FallbackParam.ApproveData != nil {
			t.Error("already-approved fallback must not carry approveData")
		}
	})
}

func TestRouteHasFallback(t *testing.T) {
	priceRoute, _, _ := testApprovalRouteAndLeg()
	if RouteHasFallback(priceRoute) {
		t.Error("route without fallbacks must report false")
	}
	priceRoute.BestRoute[0].Swaps[0].SwapExchanges[0].Fallback = &PriceRouteSwapExchange{Exchange: "fbdex"}
	if !RouteHasFallback(priceRoute) {
		t.Error("route with a fallback must report true")
	}
}
