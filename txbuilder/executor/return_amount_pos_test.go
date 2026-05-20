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

func TestExecutor010203InsertFromAmountPosOverridePacked(t *testing.T) {
	for _, insertFromAmountPos := range []int{0, 68, maxInsertFromAmountPos} {
		t.Run(fmt.Sprintf("pos_%d", insertFromAmountPos), func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			exchangeParams[0].ReturnAmountPos = nil
			exchangeParams[0].InsertFromAmountPos = &insertFromAmountPos

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
				t.Fatalf("Executor01 buildDexCallData() error = %v", err)
			}
			assertExecutor0102FromAmountPos(t, callData, insertFromAmountPos)

			callData, err = NewExecutor02Builder(testEncodingContext()).buildDexCallData(
				priceRoute,
				0,
				0,
				0,
				exchangeParams,
				0,
				insertFromAmountDontCheckBalanceAfterSwap,
			)
			if err != nil {
				t.Fatalf("Executor02 buildDexCallData() error = %v", err)
			}
			assertExecutor0102FromAmountPos(t, callData, insertFromAmountPos)

			callData, err = NewExecutor03Builder(testEncodingContext()).buildDexCallData(
				priceRoute,
				0,
				0,
				0,
				exchangeParams,
				0,
				insertFromAmountDontCheckBalanceAfterSwap,
				nil,
			)
			if err != nil {
				t.Fatalf("Executor03 buildDexCallData() error = %v", err)
			}
			assertExecutor03FromAmountPos(t, callData, insertFromAmountPos)
			assertExecutor03ToAmountPos(t, callData, 36)
		})
	}
}

func TestExecutor010203InsertFromAmountPosIgnoredWhenFlagDoesNotInsert(t *testing.T) {
	insertFromAmountPos := 68
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].InsertFromAmountPos = &insertFromAmountPos

	callData, err := NewExecutor01Builder(testEncodingContext()).buildDexCallData(
		priceRoute,
		exchangeParams,
		0,
		0,
		0,
		0,
		dontInsertFromAmountDontCheckBalanceAfterSwap,
	)
	if err != nil {
		t.Fatalf("Executor01 buildDexCallData() error = %v", err)
	}
	assertExecutor0102FromAmountPos(t, callData, 0)

	callData, err = NewExecutor02Builder(testEncodingContext()).buildDexCallData(
		priceRoute,
		0,
		0,
		0,
		exchangeParams,
		0,
		dontInsertFromAmountDontCheckBalanceAfterSwap,
	)
	if err != nil {
		t.Fatalf("Executor02 buildDexCallData() error = %v", err)
	}
	assertExecutor0102FromAmountPos(t, callData, 0)

	callData, err = NewExecutor03Builder(testEncodingContext()).buildDexCallData(
		priceRoute,
		0,
		0,
		0,
		exchangeParams,
		0,
		dontInsertFromAmountDontCheckBalanceAfterSwap,
		nil,
	)
	if err != nil {
		t.Fatalf("Executor03 buildDexCallData() error = %v", err)
	}
	assertExecutor03FromAmountPos(t, callData, 0)
	assertExecutor03ToAmountPos(t, callData, 0)
}

func TestExecutor010203InsertFromAmountPosValidation(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	exchangeParams[0].ReturnAmountPos = nil
	maxValid := maxInsertFromAmountPos
	exchangeParams[0].InsertFromAmountPos = &maxValid
	orderedLegs, err := getOrderedLegs(testBytecodeBuildInput(priceRoute, exchangeParams))
	if err != nil {
		t.Fatalf("getOrderedLegs() error = %v", err)
	}

	if err := NewExecutor01Builder(testEncodingContext()).validatePhase2eScope(priceRoute, exchangeParams, nil); err != nil {
		t.Fatalf("Executor01 valid insertFromAmountPos rejected: %v", err)
	}
	if err := NewExecutor02Builder(testEncodingContext()).validatePhase2cScope(priceRoute, exchangeParams); err != nil {
		t.Fatalf("Executor02 valid insertFromAmountPos rejected: %v", err)
	}
	if err := NewExecutor03Builder(testEncodingContext()).validatePhase2dScope(priceRoute, orderedLegs, nil); err != nil {
		t.Fatalf("Executor03 valid insertFromAmountPos rejected: %v", err)
	}

	invalid := maxInsertFromAmountPos + 1
	exchangeParams[0].InsertFromAmountPos = &invalid
	orderedLegs, err = getOrderedLegs(testBytecodeBuildInput(priceRoute, exchangeParams))
	if err != nil {
		t.Fatalf("getOrderedLegs() error = %v", err)
	}

	if err := NewExecutor01Builder(testEncodingContext()).validatePhase2eScope(priceRoute, exchangeParams, nil); err == nil {
		t.Fatalf("Executor01 accepted out-of-range insertFromAmountPos")
	}
	if err := NewExecutor02Builder(testEncodingContext()).validatePhase2cScope(priceRoute, exchangeParams); err == nil {
		t.Fatalf("Executor02 accepted out-of-range insertFromAmountPos")
	}
	if err := NewExecutor03Builder(testEncodingContext()).validatePhase2dScope(priceRoute, orderedLegs, nil); err == nil {
		t.Fatalf("Executor03 accepted out-of-range insertFromAmountPos")
	}
}

func TestExecutor010203BuildBytecodeAcceptsInsertFromAmountPos(t *testing.T) {
	insertFromAmountPos := 68
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].InsertFromAmountPos = &insertFromAmountPos
	input := testBytecodeBuildInput(priceRoute, exchangeParams)

	if _, err := NewExecutor01Builder(testEncodingContext()).BuildBytecode(input); err != nil {
		t.Fatalf("Executor01 BuildBytecode() error = %v", err)
	}
	if _, err := NewExecutor02Builder(testEncodingContext()).BuildBytecode(input); err != nil {
		t.Fatalf("Executor02 BuildBytecode() error = %v", err)
	}

	input.ExecutorType = resolved.Executor03
	input.Context = testEncodingContext()
	input.RoutePlan.Routes[0].Swaps[0].DestAmount = "456"
	if _, err := NewExecutor03Builder(testEncodingContext()).BuildBytecode(input); err != nil {
		t.Fatalf("Executor03 BuildBytecode() error = %v", err)
	}
}

func testExecutorRouteAndParams(returnAmountPos int) (executorRoute, []resolved.DexExchangeBuildParam) {
	srcAmount := resolved.DecimalString("123")
	destAmount := resolved.DecimalString("456")

	priceRoute := executorRoute{
		SrcToken:   testSrcToken,
		DestToken:  testDestToken,
		DestAmount: destAmount,
		BestRoute: []resolved.RoutePlanRoute{
			{
				Percent: 100,
				Swaps: []resolved.RoutePlanSwap{
					{
						SrcToken:   testSrcToken,
						DestToken:  testDestToken,
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

	encodedAmount, err := encodeUint256Decimal(srcAmount)
	if err != nil {
		panic(err)
	}
	encodedDestAmount, err := encodeUint256Decimal(destAmount)
	if err != nil {
		panic(err)
	}

	return priceRoute, []resolved.DexExchangeBuildParam{
		{
			NeedWrapNative:      resolved.RawBool{Value: true, Valid: true, Present: true},
			ExchangeData:        resolved.HexBytes("0x12345678" + strip0x(encodedAmount) + strip0x(encodedDestAmount)),
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

func assertExecutor0102FromAmountPos(t *testing.T, callData resolved.HexBytes, want int) {
	t.Helper()

	const fromAmountPosByteOffset = 24
	assertPackedUint16(t, callData, fromAmountPosByteOffset, want, "fromAmountPos")
}

func assertExecutor03ToAmountPos(t *testing.T, callData resolved.HexBytes, want int) {
	t.Helper()

	const toAmountPosByteOffset = 22
	assertPackedUint16(t, callData, toAmountPosByteOffset, want, "toAmountPos")
}

func assertExecutor03FromAmountPos(t *testing.T, callData resolved.HexBytes, want int) {
	t.Helper()

	const fromAmountPosByteOffset = 24
	assertPackedUint16(t, callData, fromAmountPosByteOffset, want, "fromAmountPos")
}

func assertPackedUint16(t *testing.T, callData resolved.HexBytes, byteOffset int, want int, field string) {
	t.Helper()

	raw := strip0x(string(callData))
	start := byteOffset * 2
	end := start + 4
	if len(raw) < end {
		t.Fatalf("callData too short: %s", callData)
	}

	got := raw[start:end]
	wantHex := fmt.Sprintf("%04x", want)
	if got != wantHex {
		t.Fatalf("%s = %s, want %s\ncallData = %s", field, got, wantHex, callData)
	}
}
