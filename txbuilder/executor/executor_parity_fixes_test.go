package executor

import (
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

// Executor01/02 must retry the negative int256 encoding when the positive
// uint256 encoding is absent from the exchange data (Curve-family int256
// args), matching ExecutorBytecodeBuilder.findAmountPosWithFallback.
func TestExecutor0102NegativeAmountPosFallback(t *testing.T) {
	srcAmount := resolved.DecimalString("123")
	negativeEncoded, err := encodeNegativeInt256Decimal(srcAmount)
	if err != nil {
		t.Fatalf("encodeNegativeInt256Decimal() error = %v", err)
	}
	encodedDestAmount, err := encodeUint256Decimal("456")
	if err != nil {
		t.Fatalf("encodeUint256Decimal() error = %v", err)
	}

	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].ExchangeData = resolved.HexBytes(
		"0x12345678" + strip0x(negativeEncoded) + strip0x(encodedDestAmount),
	)

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
	assertExecutor0102FromAmountPos(t, callData, 4)

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
	assertExecutor0102FromAmountPos(t, callData, 4)
}

// Same fallback, but on a non-zero swapExchangeIndex: the searched amount must
// come from that exchange's own srcAmount.
func TestExecutor02NegativeAmountPosFallbackSplitSwap(t *testing.T) {
	priceRoute, exchangeParams := testSplitSwapRouteAndParams(t)
	secondSrcAmount := priceRoute.BestRoute[0].Swaps[0].SwapExchanges[1].SrcAmount

	negativeEncoded, err := encodeNegativeInt256Decimal(secondSrcAmount)
	if err != nil {
		t.Fatalf("encodeNegativeInt256Decimal() error = %v", err)
	}
	encodedDestAmount, err := encodeUint256Decimal("222")
	if err != nil {
		t.Fatalf("encodeUint256Decimal() error = %v", err)
	}
	exchangeParams[1].ExchangeData = resolved.HexBytes(
		"0xbbbbbbbb" + strip0x(negativeEncoded) + strip0x(encodedDestAmount),
	)

	callData, err := NewExecutor02Builder(testEncodingContext()).buildDexCallData(
		priceRoute,
		0,
		0,
		1,
		exchangeParams,
		1,
		insertFromAmountDontCheckBalanceAfterSwap,
	)
	if err != nil {
		t.Fatalf("Executor02 buildDexCallData() error = %v", err)
	}
	assertExecutor0102FromAmountPos(t, callData, 4)
}

// Legacy sorts exchanges with the single-argument comparator
// `e => e.needWrapNative ? 1 : -1`; under V8 that reverses non-wrap exchanges
// at the front and keeps wrap exchanges in input order at the back.
func TestExecutor03OrderExchangesMatchesLegacySort(t *testing.T) {
	mkLeg := func(index int, needWrap bool) orderedExecutorLeg {
		return orderedExecutorLeg{
			RoutePlanExchange: resolved.RoutePlanExchange{
				SwapExchangeIndex: index,
			},
			ResolvedLeg: resolved.ResolvedLeg{
				SwapExchangeIndex: index,
				ExchangeParam: resolved.DexExchangeBuildParam{
					NeedWrapNative: resolved.RawBool{Value: needWrap, Valid: true, Present: true},
				},
			},
		}
	}

	for _, tc := range []struct {
		name  string
		wraps []bool
		want  []int
	}{
		// All verified with node against the legacy comparator.
		{name: "mixed", wraps: []bool{true, false, true, false}, want: []int{3, 1, 0, 2}},
		{name: "all_non_wrap_reversed", wraps: []bool{false, false, false}, want: []int{2, 1, 0}},
		{name: "all_wrap_keeps_order", wraps: []bool{true, true, true}, want: []int{0, 1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legs := make([]orderedExecutorLeg, 0, len(tc.wraps))
			for index, needWrap := range tc.wraps {
				legs = append(legs, mkLeg(index, needWrap))
			}

			ordered := NewExecutor03Builder(testEncodingContext()).orderExchanges(legs)

			got := make([]int, 0, len(ordered))
			for _, entry := range ordered {
				got = append(got, entry.swapExchangeIndex)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("orderExchanges() order = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// Executor01 must wrap the native output on a needUnwrapNative WETH-dest hop
// (Executor01BytecodeBuilder.ts:308-315) instead of rejecting it.
func TestExecutor01WETHDestWrapAfterSwap(t *testing.T) {
	needUnwrapNative := true
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	priceRoute.DestToken = testWETH
	priceRoute.BestRoute[0].Swaps[0].DestToken = testWETH
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].NeedUnwrapNative = &needUnwrapNative

	builder := NewExecutor01Builder(testEncodingContext())
	flags, err := builder.buildFlags(priceRoute, exchangeParams, nil)
	if err != nil {
		t.Fatalf("buildFlags() error = %v", err)
	}
	if flags.dexes[0] != insertFromAmountCheckEthBalanceAfterSwap {
		t.Fatalf("dex flag = %d, want %d", flags.dexes[0], insertFromAmountCheckEthBalanceAfterSwap)
	}

	callData, err := builder.BuildBytecode(testBytecodeBuildInput(priceRoute, exchangeParams))
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}

	depositRawCalldata, err := buildERC20DepositCalldata()
	if err != nil {
		t.Fatalf("buildERC20DepositCalldata() error = %v", err)
	}
	depositCallData, err := buildWrapEthCallData(
		testWETH,
		depositRawCalldata,
		sendEthEqualToFromAmountDontCheckBalanceAfterSwap,
		0,
	)
	if err != nil {
		t.Fatalf("buildWrapEthCallData() error = %v", err)
	}
	assertHexContainsOnce(t, callData, depositCallData, "WETH-dest deposit wrap")

	raw := strip0x(string(callData))
	dexIndex := strings.Index(raw, "12345678")
	depositIndex := strings.Index(raw, testDepositSelector)
	if dexIndex < 0 || depositIndex < 0 {
		t.Fatalf("expected calldata parts missing: %s", callData)
	}
	if depositIndex < dexIndex {
		t.Fatalf("deposit wrap appears before dex calldata\ncallData = %s", callData)
	}
}

// Executor02 must emit the [len(16)][percent*100(16)]-prefixed root deposit on
// single-swap routes (Executor02BytecodeBuilder.ts:1890-1911) instead of
// rejecting them.
func TestExecutor02RootDepositOnSingleSwapRoute(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	priceRoute.SrcToken = resolved.NativeTokenAddress
	priceRoute.BestRoute[0].Swaps[0].SrcToken = resolved.NativeTokenAddress
	exchangeParams[0].ReturnAmountPos = nil

	wethPlan := &resolved.WethPlan{
		Deposit: &resolved.WethSubPlan{Calldata: resolved.HexBytes("0x" + testDepositSelector)},
	}
	input := testBytecodeBuildInput(priceRoute, exchangeParams)
	input.WethPlan = wethPlan

	callData, err := NewExecutor02Builder(testEncodingContext()).BuildBytecode(input)
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}

	depositCallData, err := buildWrapEthCallData(
		testWETH,
		wethPlan.Deposit.Calldata,
		sendEthEqualToFromAmountDontCheckBalanceAfterSwap,
		0,
	)
	if err != nil {
		t.Fatalf("buildWrapEthCallData() error = %v", err)
	}
	depositLength, err := hexDataLength(string(depositCallData))
	if err != nil {
		t.Fatalf("hexDataLength() error = %v", err)
	}
	lengthField, err := leftPadUint(depositLength, 16)
	if err != nil {
		t.Fatalf("leftPadUint() error = %v", err)
	}
	percentField, err := leftPadUint(100*100, 16)
	if err != nil {
		t.Fatalf("leftPadUint() error = %v", err)
	}
	prefixedDeposit, err := concatHex(lengthField, percentField, string(depositCallData))
	if err != nil {
		t.Fatalf("concatHex() error = %v", err)
	}
	assertHexContainsOnce(t, callData, resolved.HexBytes(prefixedDeposit), "prefixed root deposit")

	raw := strip0x(string(callData))
	depositIndex := strings.Index(raw, testDepositSelector)
	dexIndex := strings.Index(raw, "12345678")
	if depositIndex < 0 || dexIndex < 0 {
		t.Fatalf("expected calldata parts missing: %s", callData)
	}
	if depositIndex > dexIndex {
		t.Fatalf("root deposit appears after dex calldata\ncallData = %s", callData)
	}
}
