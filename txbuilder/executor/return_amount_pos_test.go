package executor

import (
	"fmt"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const (
	testSrcToken       resolved.Address = "0x1111111111111111111111111111111111111111"
	testDestToken      resolved.Address = "0x2222222222222222222222222222222222222222"
	testTargetExchange resolved.Address = "0x3333333333333333333333333333333333333333"
	testWETH           resolved.Address = "0x4200000000000000000000000000000000000006"
)

func TestExecutor0102ReturnAmountPosOverridePacked(t *testing.T) {
	for _, returnAmountPos := range []int{0, 7} {
		t.Run("Executor01", func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(returnAmountPos)
			callData, err := NewExecutor01Builder(testEncodingContext()).buildDexCallData(
				priceRoute,
				exchangeParams,
				0,
				0,
				0,
				0,
				insertFromAmountDontCheckBalanceAfterSwap,
			)
			if err != nil {
				t.Fatalf("buildDexCallData() error = %v", err)
			}

			assertExecutor0102ReturnAmountByte(t, callData, byte(returnAmountPos))
		})

		t.Run("Executor02", func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(returnAmountPos)
			callData, err := NewExecutor02Builder(testEncodingContext()).buildDexCallData(
				priceRoute,
				0,
				0,
				0,
				exchangeParams,
				0,
				insertFromAmountDontCheckBalanceAfterSwap,
			)
			if err != nil {
				t.Fatalf("buildDexCallData() error = %v", err)
			}

			assertExecutor0102ReturnAmountByte(t, callData, byte(returnAmountPos))
		})
	}
}

func TestExecutor02ReturnAmountPosFallsBackForRootUnwrap(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	priceRoute.DestToken = resolved.NativeTokenAddress
	priceRoute.BestRoute[0].Swaps[0].DestToken = resolved.NativeTokenAddress
	exchangeParams[0].NeedWrapNative = resolved.RawBool{Value: true, Valid: true, Present: true}

	callData, err := NewExecutor02Builder(testEncodingContext()).buildDexCallData(
		priceRoute,
		0,
		0,
		0,
		exchangeParams,
		0,
		insertFromAmountDontCheckBalanceAfterSwap,
	)
	if err != nil {
		t.Fatalf("buildDexCallData() error = %v", err)
	}

	assertExecutor0102ReturnAmountByte(t, callData, defaultReturnAmountPos)
}

func TestExecutor02ReturnAmountPosFallsBackForUnwrapAfterLastSwapInRoute(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	priceRoute.BestRoute[0].Swaps[0].DestToken = resolved.NativeTokenAddress
	priceRoute.BestRoute[0].Swaps[0].SwapExchanges = append(
		priceRoute.BestRoute[0].Swaps[0].SwapExchanges,
		resolved.RoutePlanSwapExchange{
			Exchange:   "other",
			Percent:    50,
			SrcAmount:  "123",
			DestAmount: "456",
		},
	)
	priceRoute.BestRoute[0].Swaps[0].SwapExchanges[0].Percent = 50

	exchangeParams[0].NeedWrapNative = resolved.RawBool{Value: true, Valid: true, Present: true}
	exchangeParams = append(exchangeParams, exchangeParams[0])
	exchangeParams[1].NeedWrapNative = resolved.RawBool{Value: false, Valid: true, Present: true}
	otherReturnAmountPos := 7
	exchangeParams[1].ReturnAmountPos = &otherReturnAmountPos

	callData, err := NewExecutor02Builder(testEncodingContext()).buildDexCallData(
		priceRoute,
		0,
		0,
		0,
		exchangeParams,
		0,
		insertFromAmountDontCheckBalanceAfterSwap,
	)
	if err != nil {
		t.Fatalf("buildDexCallData() error = %v", err)
	}

	assertExecutor0102ReturnAmountByte(t, callData, defaultReturnAmountPos)

	callData, err = NewExecutor02Builder(testEncodingContext()).buildDexCallData(
		priceRoute,
		0,
		0,
		1,
		exchangeParams,
		1,
		insertFromAmountDontCheckBalanceAfterSwap,
	)
	if err != nil {
		t.Fatalf("buildDexCallData() error = %v", err)
	}

	assertExecutor0102ReturnAmountByte(t, callData, byte(otherReturnAmountPos))
}

func TestExecutor0102ReturnAmountPosValidation(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(255)

	if err := NewExecutor01Builder(testEncodingContext()).validatePhase2eScope(priceRoute, exchangeParams, nil); err != nil {
		t.Fatalf("Executor01 valid returnAmountPos rejected: %v", err)
	}
	if err := NewExecutor02Builder(testEncodingContext()).validatePhase2cScope(priceRoute, exchangeParams); err != nil {
		t.Fatalf("Executor02 valid returnAmountPos rejected: %v", err)
	}

	invalid := 256
	exchangeParams[0].ReturnAmountPos = &invalid

	if err := NewExecutor01Builder(testEncodingContext()).validatePhase2eScope(priceRoute, exchangeParams, nil); err == nil {
		t.Fatalf("Executor01 accepted out-of-range returnAmountPos")
	}
	if err := NewExecutor02Builder(testEncodingContext()).validatePhase2cScope(priceRoute, exchangeParams); err == nil {
		t.Fatalf("Executor02 accepted out-of-range returnAmountPos")
	}
}

func TestExecutor0102BuildBytecodeAcceptsReturnAmountPos(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	input := testBytecodeBuildInput(priceRoute, exchangeParams)

	if _, err := NewExecutor01Builder(testEncodingContext()).BuildBytecode(input); err != nil {
		t.Fatalf("Executor01 BuildBytecode() error = %v", err)
	}
	if _, err := NewExecutor02Builder(testEncodingContext()).BuildBytecode(input); err != nil {
		t.Fatalf("Executor02 BuildBytecode() error = %v", err)
	}
}

func testExecutorRouteAndParams(returnAmountPos int) (executorRoute, []resolved.DexExchangeBuildParam) {
	srcAmount := resolved.DecimalString("123")

	priceRoute := executorRoute{
		SrcToken:   testSrcToken,
		DestToken:  testDestToken,
		DestAmount: "456",
		BestRoute: []resolved.RoutePlanRoute{
			{
				Percent: 100,
				Swaps: []resolved.RoutePlanSwap{
					{
						SrcToken:   testSrcToken,
						DestToken:  testDestToken,
						SrcAmount:  srcAmount,
						DestAmount: "456",
						SwapExchanges: []resolved.RoutePlanSwapExchange{
							{
								Exchange:   "test",
								Percent:    100,
								SrcAmount:  srcAmount,
								DestAmount: "456",
							},
						},
					},
				},
			},
		},
	}

	encodedAmount, err := encodeUint256Decimal(srcAmount)
	if err != nil {
		panic(err)
	}

	return priceRoute, []resolved.DexExchangeBuildParam{
		{
			NeedWrapNative:      resolved.RawBool{Value: true, Valid: true, Present: true},
			ExchangeData:        resolved.HexBytes("0x12345678" + strip0x(encodedAmount)),
			TargetExchange:      testTargetExchange,
			DexFuncHasRecipient: true,
			ReturnAmountPos:     &returnAmountPos,
		},
	}
}

func testBytecodeBuildInput(
	priceRoute executorRoute,
	exchangeParams []resolved.DexExchangeBuildParam,
) resolved.ExecutorBytecodeBuildInput {
	resolvedLegs := make([]resolved.ResolvedLeg, 0, len(exchangeParams))
	for routeIndex, route := range priceRoute.BestRoute {
		for swapIndex, swap := range route.Swaps {
			for swapExchangeIndex := range swap.SwapExchanges {
				exchangeParamIndex := exchangeParamIndexForPosition(
					priceRoute,
					routeIndex,
					swapIndex,
					swapExchangeIndex,
				)
				resolvedLegs = append(resolvedLegs, resolved.ResolvedLeg{
					RouteIndex:        routeIndex,
					SwapIndex:         swapIndex,
					SwapExchangeIndex: swapExchangeIndex,
					ExchangeParam:     exchangeParams[exchangeParamIndex],
				})
			}
		}
	}

	return resolved.ExecutorBytecodeBuildInput{
		RoutePlan: resolved.RoutePlan{
			Routes: priceRoute.BestRoute,
		},
		ResolvedLegs: resolvedLegs,
		SrcToken:     priceRoute.SrcToken,
		DestToken:    priceRoute.DestToken,
		DestAmount:   priceRoute.DestAmount,
	}
}

func testEncodingContext() resolved.EncodingContext {
	return resolved.EncodingContext{
		WrappedNativeTokenAddress: testWETH,
	}
}

func assertExecutor0102ReturnAmountByte(t *testing.T, callData resolved.HexBytes, want byte) {
	t.Helper()

	raw := strip0x(string(callData))
	const returnAmountPosByteOffset = 28
	start := returnAmountPosByteOffset * 2
	end := start + 2
	if len(raw) < end {
		t.Fatalf("callData too short: %s", callData)
	}

	got := raw[start:end]
	wantHex := fmt.Sprintf("%02x", want)
	if got != wantHex {
		t.Fatalf("returnAmountPos byte = %s, want %s\ncallData = %s", got, wantHex, callData)
	}
}
