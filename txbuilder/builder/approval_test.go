package builder

import (
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const (
	testApprovalSrcToken       resolved.Address = "0x1111111111111111111111111111111111111111"
	testApprovalDestToken      resolved.Address = "0x2222222222222222222222222222222222222222"
	testApprovalTargetExchange resolved.Address = "0x3333333333333333333333333333333333333333"
	testApprovalSpender        resolved.Address = "0x4444444444444444444444444444444444444444"
	testApprovalWETH           resolved.Address = "0x4200000000000000000000000000000000000006"
)

func TestBuildDexExchangeApprovalRequestsSkipsWhenSkipApprovalTrue(t *testing.T) {
	priceRoute, routePlan, resolvedLegs := testApprovalRouteAndLeg()
	skipApproval := true
	resolvedLegs[0].ExchangeParam.SkipApproval = &skipApproval
	resolvedLegs[0].ExchangeParam.Spender = ptr(testApprovalSpender)

	requests, err := buildDexExchangeApprovalRequests(
		testApprovalEncodingContext(),
		priceRoute,
		routePlan,
		resolvedLegs,
	)
	if err != nil {
		t.Fatalf("buildDexExchangeApprovalRequests() error = %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("approval request count = %d, want 0", len(requests))
	}

	out, err := applyDexExchangeApprovalDecisions(resolvedLegs, requests, nil)
	if err != nil {
		t.Fatalf("applyDexExchangeApprovalDecisions() error = %v", err)
	}
	if out[0].ExchangeParam.ApproveData != nil {
		t.Fatalf("ApproveData = %+v, want nil", out[0].ExchangeParam.ApproveData)
	}
	if out[0].ExchangeParam.Spender != nil {
		t.Fatalf("Spender = %s, want nil", *out[0].ExchangeParam.Spender)
	}
}

func TestApplyDexExchangeApprovalDecisionsUsesSpenderAndClearsIt(t *testing.T) {
	priceRoute, routePlan, resolvedLegs := testApprovalRouteAndLeg()
	resolvedLegs[0].ExchangeParam.Spender = ptr(testApprovalSpender)

	requests, err := buildDexExchangeApprovalRequests(
		testApprovalEncodingContext(),
		priceRoute,
		routePlan,
		resolvedLegs,
	)
	if err != nil {
		t.Fatalf("buildDexExchangeApprovalRequests() error = %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("approval request count = %d, want 1", len(requests))
	}
	if requests[0].request.Target != testApprovalSpender {
		t.Fatalf("approval target = %s, want %s", requests[0].request.Target, testApprovalSpender)
	}

	out, err := applyDexExchangeApprovalDecisions(resolvedLegs, requests, []bool{false})
	if err != nil {
		t.Fatalf("applyDexExchangeApprovalDecisions() error = %v", err)
	}
	if out[0].ExchangeParam.Spender != nil {
		t.Fatalf("Spender = %s, want nil", *out[0].ExchangeParam.Spender)
	}
	if out[0].ExchangeParam.ApproveData == nil {
		t.Fatalf("ApproveData = nil, want populated")
	}
	if out[0].ExchangeParam.ApproveData.Target != testApprovalSpender {
		t.Fatalf(
			"ApproveData.Target = %s, want %s",
			out[0].ExchangeParam.ApproveData.Target,
			testApprovalSpender,
		)
	}
	if out[0].ExchangeParam.ApproveData.Token != testApprovalSrcToken {
		t.Fatalf(
			"ApproveData.Token = %s, want %s",
			out[0].ExchangeParam.ApproveData.Token,
			testApprovalSrcToken,
		)
	}
}

func TestApplyDexExchangeApprovalDecisionsClearsSpenderWhenAlreadyApproved(t *testing.T) {
	priceRoute, routePlan, resolvedLegs := testApprovalRouteAndLeg()
	resolvedLegs[0].ExchangeParam.Spender = ptr(testApprovalSpender)

	requests, err := buildDexExchangeApprovalRequests(
		testApprovalEncodingContext(),
		priceRoute,
		routePlan,
		resolvedLegs,
	)
	if err != nil {
		t.Fatalf("buildDexExchangeApprovalRequests() error = %v", err)
	}

	out, err := applyDexExchangeApprovalDecisions(resolvedLegs, requests, []bool{true})
	if err != nil {
		t.Fatalf("applyDexExchangeApprovalDecisions() error = %v", err)
	}
	if out[0].ExchangeParam.Spender != nil {
		t.Fatalf("Spender = %s, want nil", *out[0].ExchangeParam.Spender)
	}
	if out[0].ExchangeParam.ApproveData != nil {
		t.Fatalf("ApproveData = %+v, want nil", out[0].ExchangeParam.ApproveData)
	}
}

func testApprovalRouteAndLeg() (PriceRoute, resolved.RoutePlan, []resolved.ResolvedLeg) {
	srcAmount := resolved.DecimalString("123")
	destAmount := resolved.DecimalString("456")

	priceRoute := PriceRoute{
		Network:        1,
		ContractMethod: resolved.ContractMethodSwapExactAmountIn,
		Side:           resolved.SideSell,
		SrcToken:       testApprovalSrcToken,
		DestToken:      testApprovalDestToken,
		SrcAmount:      srcAmount,
		DestAmount:     destAmount,
		BestRoute: []PriceRouteRoute{
			{
				Percent: 100,
				Swaps: []PriceRouteSwap{
					{
						SrcToken:   testApprovalSrcToken,
						DestToken:  testApprovalDestToken,
						SrcAmount:  ptr(srcAmount),
						DestAmount: ptr(destAmount),
						SwapExchanges: []PriceRouteSwapExchange{
							{
								Exchange:   "test",
								Percent:    100,
								SrcAmount:  srcAmount,
								DestAmount: destAmount,
							},
						},
					},
				},
			},
		},
	}

	routePlan := resolved.RoutePlan{
		Routes: []resolved.RoutePlanRoute{
			{
				Percent: 100,
				Swaps: []resolved.RoutePlanSwap{
					{
						SrcToken:   testApprovalSrcToken,
						DestToken:  testApprovalDestToken,
						SrcAmount:  srcAmount,
						DestAmount: destAmount,
						SwapExchanges: []resolved.RoutePlanSwapExchange{
							{
								Exchange:   "test",
								Percent:    100,
								SrcAmount:  srcAmount,
								DestAmount: destAmount,
							},
						},
					},
				},
			},
		},
	}

	resolvedLegs := []resolved.ResolvedLeg{
		{
			RouteIndex:        0,
			SwapIndex:         0,
			SwapExchangeIndex: 0,
			ExchangeParam: resolved.DexExchangeBuildParam{
				NeedWrapNative:      resolved.RawBool{Value: true, Valid: true, Present: true},
				ExchangeData:        "0x12345678",
				TargetExchange:      testApprovalTargetExchange,
				DexFuncHasRecipient: true,
			},
		},
	}

	return priceRoute, routePlan, resolvedLegs
}

func testApprovalEncodingContext() resolved.EncodingContext {
	return resolved.EncodingContext{
		WrappedNativeTokenAddress: testApprovalWETH,
	}
}

func ptr[T any](value T) *T {
	return &value
}
