package executor

import (
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

func TestExecutor03AmountsPacked128FindsPackedPositionsAndSetsFlag(t *testing.T) {
	for _, tc := range []struct {
		name     string
		encodeFn func(resolved.DecimalString) (string, error)
	}{
		{
			name:     "positive int128",
			encodeFn: encodeUint128Decimal,
		},
		{
			name:     "negative int128",
			encodeFn: encodeNegativeInt128Decimal,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutor03Packed128RouteAndParams(t, tc.encodeFn)

			callData, err := NewExecutor03Builder(testEncodingContext()).buildDexCallData(
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
				t.Fatalf("buildDexCallData() error = %v", err)
			}

			assertExecutor03FromAmountPos(t, callData, 4)
			assertExecutor03ToAmountPos(t, callData, 36)
			assertExecutor03Flag(t, callData, flag(int(insertFromAmountDontCheckBalanceAfterSwap)|0x8000))
		})
	}
}

func TestExecutor03BuildBytecodeAcceptsAmountsPacked128(t *testing.T) {
	priceRoute, exchangeParams := testExecutor03Packed128RouteAndParams(t, encodeUint128Decimal)
	input := testBytecodeBuildInput(priceRoute, exchangeParams)
	input.ExecutorType = resolved.Executor03

	if _, err := NewExecutor03Builder(testEncodingContext()).BuildBytecode(input); err != nil {
		t.Fatalf("Executor03 BuildBytecode() error = %v", err)
	}
}

func TestExecutor03AmountsPacked128UsesInsertFromAmountPosOverride(t *testing.T) {
	priceRoute, exchangeParams := testExecutor03Packed128RouteAndParams(t, encodeUint128Decimal)
	insertFromAmountPos := 68
	exchangeParams[0].InsertFromAmountPos = &insertFromAmountPos

	callData, err := NewExecutor03Builder(testEncodingContext()).buildDexCallData(
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
		t.Fatalf("buildDexCallData() error = %v", err)
	}

	assertExecutor03FromAmountPos(t, callData, insertFromAmountPos)
	assertExecutor03ToAmountPos(t, callData, 36)
	assertExecutor03Flag(t, callData, flag(int(insertFromAmountDontCheckBalanceAfterSwap)|0x8000))
}

func TestFindAmount128PosInCalldataFallsBackPastEndWhenSlotPosWouldBeNegative(t *testing.T) {
	amount128, err := encodeUint128Decimal("123")
	if err != nil {
		t.Fatalf("encode amount error = %v", err)
	}

	callData := resolved.HexBytes("0x12345678" + strip0x(amount128))
	pos, err := findAmount128PosInCalldata(callData, "123")
	if err != nil {
		t.Fatalf("findAmount128PosInCalldata() error = %v", err)
	}
	if want := len(string(callData)) / 2; pos != want {
		t.Fatalf("fallback pos = %d, want %d", pos, want)
	}
}

func TestExecutor03AmountsPacked128SplitSwapSequencing(t *testing.T) {
	priceRoute, exchangeParams := testExecutor03SplitPacked128RouteAndParams(t)

	input := testBytecodeBuildInput(priceRoute, exchangeParams)
	input.ExecutorType = resolved.Executor03
	bytecode, err := NewExecutor03Builder(testEncodingContext()).BuildBytecode(input)
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}

	raw := strip0x(string(bytecode))
	firstDexIndex := strings.Index(raw, "aaaaaaaa")
	secondDexIndex := strings.Index(raw, "bbbbbbbb")
	if firstDexIndex < 0 || secondDexIndex < 0 {
		t.Fatalf("expected packed128 split dex calldata missing: %s", bytecode)
	}
	if firstDexIndex > secondDexIndex {
		t.Fatalf("unexpected split packed128 order: firstDex=%d secondDex=%d\nbytecode=%s", firstDexIndex, secondDexIndex, bytecode)
	}

	for i := range exchangeParams {
		callData, err := NewExecutor03Builder(testEncodingContext()).buildDexCallData(
			priceRoute,
			0,
			0,
			i,
			exchangeParams,
			i,
			insertFromAmountDontCheckBalanceAfterSwap,
			nil,
		)
		if err != nil {
			t.Fatalf("buildDexCallData(%d) error = %v", i, err)
		}

		assertExecutor03FromAmountPos(t, callData, 4)
		assertExecutor03ToAmountPos(t, callData, 36)
		assertExecutor03Flag(t, callData, flag(int(insertFromAmountDontCheckBalanceAfterSwap)|0x8000))
	}
}

func TestExecutor010203ValidationAcceptsAmountsPacked128(t *testing.T) {
	priceRoute, exchangeParams := testExecutor03Packed128RouteAndParams(t, encodeUint128Decimal)
	orderedLegs, err := getOrderedLegs(testBytecodeBuildInput(priceRoute, exchangeParams))
	if err != nil {
		t.Fatalf("getOrderedLegs() error = %v", err)
	}

	if err := NewExecutor03Builder(testEncodingContext()).validateExecutor03Input(priceRoute, orderedLegs, nil); err != nil {
		t.Fatalf("Executor03 valid amountsPacked128 rejected: %v", err)
	}
	if err := NewExecutor01Builder(testEncodingContext()).validateExecutor01Input(priceRoute, exchangeParams, nil); err != nil {
		t.Fatalf("Executor01 valid amountsPacked128 rejected: %v", err)
	}
	if err := NewExecutor02Builder(testEncodingContext()).validateExecutor02Input(priceRoute, exchangeParams); err != nil {
		t.Fatalf("Executor02 valid amountsPacked128 rejected: %v", err)
	}
}

func TestFindAmount128PosInCalldataFallsBackPastEnd(t *testing.T) {
	callData := resolved.HexBytes("0x12345678")
	pos, err := findAmount128PosInCalldata(callData, "123")
	if err != nil {
		t.Fatalf("findAmount128PosInCalldata() error = %v", err)
	}
	if want := len(string(callData)) / 2; pos != want {
		t.Fatalf("fallback pos = %d, want %d", pos, want)
	}
}

func testExecutor03Packed128RouteAndParams(
	t *testing.T,
	encodeFn func(resolved.DecimalString) (string, error),
) (executorRoute, []resolved.DexExchangeBuildParam) {
	t.Helper()

	srcAmount := resolved.DecimalString("123")
	destAmount := resolved.DecimalString("456")

	srcAmount128, err := encodeFn(srcAmount)
	if err != nil {
		t.Fatalf("encode src amount error = %v", err)
	}
	destAmount128, err := encodeFn(destAmount)
	if err != nil {
		t.Fatalf("encode dest amount error = %v", err)
	}

	exchangeData := resolved.HexBytes("0x12345678" +
		strings.Repeat("00", 16) +
		strip0x(srcAmount128) +
		strings.Repeat("00", 16) +
		strip0x(destAmount128))

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

	amountsPacked128 := true
	return priceRoute, []resolved.DexExchangeBuildParam{
		{
			NeedWrapNative:      resolved.RawBool{Value: true, Valid: true, Present: true},
			ExchangeData:        exchangeData,
			TargetExchange:      testTargetExchange,
			DexFuncHasRecipient: true,
			AmountsPacked128:    &amountsPacked128,
		},
	}
}

func testExecutor03SplitPacked128RouteAndParams(
	t *testing.T,
) (executorRoute, []resolved.DexExchangeBuildParam) {
	t.Helper()

	firstSrcAmount := resolved.DecimalString("123")
	firstDestAmount := resolved.DecimalString("456")
	secondSrcAmount := resolved.DecimalString("789")
	secondDestAmount := resolved.DecimalString("987")

	firstExchangeData := buildPacked128ExchangeData(t, "aaaaaaaa", firstSrcAmount, firstDestAmount)
	secondExchangeData := buildPacked128ExchangeData(t, "bbbbbbbb", secondSrcAmount, secondDestAmount)

	priceRoute := executorRoute{
		SrcToken:   testSrcToken,
		DestToken:  testDestToken,
		DestAmount: firstDestAmount,
		BestRoute: []resolved.RoutePlanRoute{
			{
				Percent: 100,
				Swaps: []resolved.RoutePlanSwap{
					{
						SrcToken:   testSrcToken,
						DestToken:  testDestToken,
						SrcAmount:  firstSrcAmount,
						DestAmount: firstDestAmount,
						SwapExchanges: []resolved.RoutePlanSwapExchange{
							{
								Exchange:   "test-a",
								Percent:    60,
								SrcAmount:  firstSrcAmount,
								DestAmount: firstDestAmount,
							},
							{
								Exchange:   "test-b",
								Percent:    40,
								SrcAmount:  secondSrcAmount,
								DestAmount: secondDestAmount,
							},
						},
					},
				},
			},
		},
	}

	amountsPacked128 := true
	return priceRoute, []resolved.DexExchangeBuildParam{
		{
			NeedWrapNative:      resolved.RawBool{Value: true, Valid: true, Present: true},
			ExchangeData:        firstExchangeData,
			TargetExchange:      testTargetExchange,
			DexFuncHasRecipient: true,
			AmountsPacked128:    &amountsPacked128,
		},
		{
			NeedWrapNative:      resolved.RawBool{Value: true, Valid: true, Present: true},
			ExchangeData:        secondExchangeData,
			TargetExchange:      testTargetExchange,
			DexFuncHasRecipient: true,
			AmountsPacked128:    &amountsPacked128,
		},
	}
}

func buildPacked128ExchangeData(
	t *testing.T,
	selector string,
	srcAmount resolved.DecimalString,
	destAmount resolved.DecimalString,
) resolved.HexBytes {
	t.Helper()

	srcAmount128, err := encodeUint128Decimal(srcAmount)
	if err != nil {
		t.Fatalf("encode src amount error = %v", err)
	}
	destAmount128, err := encodeUint128Decimal(destAmount)
	if err != nil {
		t.Fatalf("encode dest amount error = %v", err)
	}

	return resolved.HexBytes("0x" + selector +
		strings.Repeat("00", 16) +
		strip0x(srcAmount128) +
		strings.Repeat("00", 16) +
		strip0x(destAmount128))
}
