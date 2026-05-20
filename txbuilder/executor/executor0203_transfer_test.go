package executor

import (
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const testTransferSelector = "a9059cbb"

func TestExecutor0203TransferBeforeSwapSuppressesApproval(t *testing.T) {
	recipient := resolved.Address("0x4444444444444444444444444444444444444444")

	for _, tc := range []struct {
		name         string
		build        func(resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error)
		wrapTransfer func(resolved.HexBytes, resolved.Address) (resolved.HexBytes, error)
		amount       resolved.DecimalString
	}{
		{
			name: "Executor02",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor02Builder(testEncodingContext()).BuildBytecode(input)
			},
			wrapTransfer: buildTransferCallData,
			amount:       "123",
		},
		{
			name: "Executor03",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				input.ExecutorType = resolved.Executor03
				return NewExecutor03Builder(testEncodingContext()).BuildBytecode(input)
			},
			wrapTransfer: buildExecutor03TransferCallData,
			amount:       "123",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			exchangeParams[0].ReturnAmountPos = nil
			exchangeParams[0].TransferSrcTokenBeforeSwap = &recipient
			exchangeParams[0].ApproveData = &resolved.ApproveData{
				Target: testTargetExchange,
				Token:  testSrcToken,
			}

			callData, err := tc.build(testBytecodeBuildInput(priceRoute, exchangeParams))
			if err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}
			transferCalldata, err := buildERC20TransferCalldata(recipient, "123")
			if err != nil {
				t.Fatalf("buildERC20TransferCalldata() error = %v", err)
			}
			wrappedTransferCalldata, err := tc.wrapTransfer(transferCalldata, testSrcToken)
			if err != nil {
				t.Fatalf("wrap transfer calldata error = %v", err)
			}
			assertHexContainsOnce(t, callData, wrappedTransferCalldata, "wrapped transfer calldata")

			raw := strip0x(string(callData))
			if strings.Contains(raw, testApprovalSelector) {
				t.Fatalf("approval calldata present with transferSrcTokenBeforeSwap: %s", callData)
			}
			transferIndex := strings.Index(raw, testTransferSelector)
			dexIndex := strings.Index(raw, "12345678")
			if transferIndex < 0 {
				t.Fatalf("transfer calldata missing: %s", callData)
			}
			if dexIndex < 0 {
				t.Fatalf("dex calldata missing: %s", callData)
			}
			if transferIndex > dexIndex {
				t.Fatalf("transfer appears after dex calldata\ncallData = %s", callData)
			}
		})
	}
}

func TestExecutor0203TransferBeforeSwapSplitSwapSequencing(t *testing.T) {
	recipient := resolved.Address("0x4444444444444444444444444444444444444444")

	for _, tc := range []struct {
		name         string
		build        func(resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error)
		wrapTransfer func(resolved.HexBytes, resolved.Address) (resolved.HexBytes, error)
		amount       resolved.DecimalString
	}{
		{
			name: "Executor02",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor02Builder(testEncodingContext()).BuildBytecode(input)
			},
			wrapTransfer: buildTransferCallData,
			amount:       "321",
		},
		{
			name: "Executor03",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				input.ExecutorType = resolved.Executor03
				return NewExecutor03Builder(testEncodingContext()).BuildBytecode(input)
			},
			wrapTransfer: buildExecutor03TransferCallData,
			amount:       "123",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testSplitSwapRouteAndParams(t)
			exchangeParams[1].TransferSrcTokenBeforeSwap = &recipient

			callData, err := tc.build(testBytecodeBuildInput(priceRoute, exchangeParams))
			if err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}

			transferCalldata, err := buildERC20TransferCalldata(recipient, tc.amount)
			if err != nil {
				t.Fatalf("buildERC20TransferCalldata() error = %v", err)
			}
			wrappedTransferCalldata, err := tc.wrapTransfer(transferCalldata, testSrcToken)
			if err != nil {
				t.Fatalf("wrap transfer calldata error = %v", err)
			}
			assertHexContainsOnce(t, callData, wrappedTransferCalldata, "wrapped split-swap transfer calldata")

			raw := strip0x(string(callData))
			firstDexIndex := strings.Index(raw, "aaaaaaaa")
			transferIndex := strings.Index(raw, testTransferSelector)
			secondDexIndex := strings.Index(raw, "bbbbbbbb")
			if firstDexIndex < 0 || transferIndex < 0 || secondDexIndex < 0 {
				t.Fatalf("expected split-swap parts missing: %s", callData)
			}
			if !(firstDexIndex < transferIndex && transferIndex < secondDexIndex) {
				t.Fatalf(
					"unexpected split-swap order: firstDex=%d transfer=%d secondDex=%d\ncallData=%s",
					firstDexIndex,
					transferIndex,
					secondDexIndex,
					callData,
				)
			}
		})
	}
}

func TestExecutor02TransferBeforeSwapMultiSwapSequencing(t *testing.T) {
	recipient := resolved.Address("0x4444444444444444444444444444444444444444")
	priceRoute, exchangeParams := testExecutor02MultiSwapRouteAndParams(t)
	exchangeParams[1].TransferSrcTokenBeforeSwap = &recipient

	callData, err := NewExecutor02Builder(testEncodingContext()).BuildBytecode(
		testBytecodeBuildInput(priceRoute, exchangeParams),
	)
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}
	transferCalldata, err := buildERC20TransferCalldata(recipient, "234")
	if err != nil {
		t.Fatalf("buildERC20TransferCalldata() error = %v", err)
	}
	wrappedTransferCalldata, err := buildTransferCallData(transferCalldata, resolved.Address("0x5555555555555555555555555555555555555555"))
	if err != nil {
		t.Fatalf("buildTransferCallData() error = %v", err)
	}
	assertHexContainsOnce(t, callData, wrappedTransferCalldata, "wrapped multiswap transfer calldata")

	raw := strip0x(string(callData))
	firstDexIndex := strings.Index(raw, "aaaaaaaa")
	transferIndex := strings.Index(raw, testTransferSelector)
	secondDexIndex := strings.Index(raw, "bbbbbbbb")
	if firstDexIndex < 0 || transferIndex < 0 || secondDexIndex < 0 {
		t.Fatalf("expected multi-swap parts missing: %s", callData)
	}
	if !(firstDexIndex < transferIndex && transferIndex < secondDexIndex) {
		t.Fatalf(
			"unexpected multi-swap order: firstDex=%d transfer=%d secondDex=%d\ncallData=%s",
			firstDexIndex,
			transferIndex,
			secondDexIndex,
			callData,
		)
	}
}

func TestExecutor03TransferCallDataUsesExecutor03Metadata(t *testing.T) {
	transferCalldata, err := buildERC20TransferCalldata(
		resolved.Address("0x4444444444444444444444444444444444444444"),
		"123",
	)
	if err != nil {
		t.Fatalf("buildERC20TransferCalldata() error = %v", err)
	}

	callData, err := buildExecutor03TransferCallData(transferCalldata, testSrcToken)
	if err != nil {
		t.Fatalf("buildExecutor03TransferCallData() error = %v", err)
	}

	transferLength, err := hexDataLength(string(transferCalldata))
	if err != nil {
		t.Fatalf("hexDataLength() error = %v", err)
	}
	assertExecutor03ToAmountPos(t, callData, transferLength)
	assertExecutor03FromAmountPos(t, callData, erc20TransferAmountPos)
}

func TestExecutor03BuildBytecodeTransferUsesExecutor03Metadata(t *testing.T) {
	recipient := resolved.Address("0x4444444444444444444444444444444444444444")
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].TransferSrcTokenBeforeSwap = &recipient

	input := testBytecodeBuildInput(priceRoute, exchangeParams)
	input.ExecutorType = resolved.Executor03
	callData, err := NewExecutor03Builder(testEncodingContext()).BuildBytecode(input)
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}

	transferCalldata, err := buildERC20TransferCalldata(recipient, "123")
	if err != nil {
		t.Fatalf("buildERC20TransferCalldata() error = %v", err)
	}
	wrappedTransferCalldata, err := buildExecutor03TransferCallData(transferCalldata, testSrcToken)
	if err != nil {
		t.Fatalf("buildExecutor03TransferCallData() error = %v", err)
	}
	assertHexContainsOnce(t, callData, wrappedTransferCalldata, "Executor03 wrapped transfer calldata")
}

func TestExecutor0203ValidationAcceptsTransferBeforeSwap(t *testing.T) {
	recipient := resolved.Address("0x4444444444444444444444444444444444444444")
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].TransferSrcTokenBeforeSwap = &recipient

	if err := NewExecutor02Builder(testEncodingContext()).validatePhase2cScope(priceRoute, exchangeParams); err != nil {
		t.Fatalf("Executor02 valid transferSrcTokenBeforeSwap rejected: %v", err)
	}

	orderedLegs, err := getOrderedLegs(testBytecodeBuildInput(priceRoute, exchangeParams))
	if err != nil {
		t.Fatalf("getOrderedLegs() error = %v", err)
	}
	if err := NewExecutor03Builder(testEncodingContext()).validatePhase2dScope(priceRoute, orderedLegs, nil); err != nil {
		t.Fatalf("Executor03 valid transferSrcTokenBeforeSwap rejected: %v", err)
	}
}

func testExecutor02MultiSwapRouteAndParams(t *testing.T) (executorRoute, []resolved.DexExchangeBuildParam) {
	t.Helper()

	srcAmount := resolved.DecimalString("123")
	midAmount := resolved.DecimalString("234")
	destAmount := resolved.DecimalString("456")
	midToken := resolved.Address("0x5555555555555555555555555555555555555555")

	firstExchangeData := testExchangeDataWithSelector(t, "aaaaaaaa", srcAmount, midAmount)
	secondExchangeData := testExchangeDataWithSelector(t, "bbbbbbbb", midAmount, destAmount)

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
						DestToken:  midToken,
						SrcAmount:  srcAmount,
						DestAmount: midAmount,
						SwapExchanges: []resolved.RoutePlanSwapExchange{
							{
								Exchange:   "first",
								Percent:    100,
								SrcAmount:  srcAmount,
								DestAmount: midAmount,
							},
						},
					},
					{
						SrcToken:   midToken,
						DestToken:  testDestToken,
						SrcAmount:  midAmount,
						DestAmount: destAmount,
						SwapExchanges: []resolved.RoutePlanSwapExchange{
							{
								Exchange:   "second",
								Percent:    100,
								SrcAmount:  midAmount,
								DestAmount: destAmount,
							},
						},
					},
				},
			},
		},
	}

	return priceRoute, []resolved.DexExchangeBuildParam{
		{
			NeedWrapNative:      resolved.RawBool{Value: true, Valid: true, Present: true},
			ExchangeData:        firstExchangeData,
			TargetExchange:      testTargetExchange,
			DexFuncHasRecipient: true,
		},
		{
			NeedWrapNative:      resolved.RawBool{Value: true, Valid: true, Present: true},
			ExchangeData:        secondExchangeData,
			TargetExchange:      testTargetExchange,
			DexFuncHasRecipient: true,
		},
	}
}

func testSplitSwapRouteAndParams(t *testing.T) (executorRoute, []resolved.DexExchangeBuildParam) {
	t.Helper()

	srcAmount := resolved.DecimalString("123")
	firstDestAmount := resolved.DecimalString("111")
	secondSrcAmount := resolved.DecimalString("321")
	secondDestAmount := resolved.DecimalString("222")
	destAmount := resolved.DecimalString("333")

	firstExchangeData := testExchangeDataWithSelector(t, "aaaaaaaa", srcAmount, firstDestAmount)
	secondExchangeData := testExchangeDataWithSelector(t, "bbbbbbbb", secondSrcAmount, secondDestAmount)

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
								Exchange:   "first",
								Percent:    50,
								SrcAmount:  srcAmount,
								DestAmount: firstDestAmount,
							},
							{
								Exchange:   "second",
								Percent:    50,
								SrcAmount:  secondSrcAmount,
								DestAmount: secondDestAmount,
							},
						},
					},
				},
			},
		},
	}

	return priceRoute, []resolved.DexExchangeBuildParam{
		{
			NeedWrapNative:      resolved.RawBool{Value: true, Valid: true, Present: true},
			ExchangeData:        firstExchangeData,
			TargetExchange:      testTargetExchange,
			DexFuncHasRecipient: true,
		},
		{
			NeedWrapNative:      resolved.RawBool{Value: true, Valid: true, Present: true},
			ExchangeData:        secondExchangeData,
			TargetExchange:      testTargetExchange,
			DexFuncHasRecipient: true,
		},
	}
}

func testExchangeDataWithSelector(
	t *testing.T,
	selector string,
	srcAmount resolved.DecimalString,
	destAmount resolved.DecimalString,
) resolved.HexBytes {
	t.Helper()

	encodedSrcAmount, err := encodeUint256Decimal(srcAmount)
	if err != nil {
		t.Fatalf("encodeUint256Decimal(srcAmount) error = %v", err)
	}
	encodedDestAmount, err := encodeUint256Decimal(destAmount)
	if err != nil {
		t.Fatalf("encodeUint256Decimal(destAmount) error = %v", err)
	}
	return resolved.HexBytes("0x" + selector + strip0x(encodedSrcAmount) + strip0x(encodedDestAmount))
}

func assertHexContainsOnce(
	t *testing.T,
	callData resolved.HexBytes,
	want resolved.HexBytes,
	label string,
) {
	t.Helper()

	count := strings.Count(strip0x(string(callData)), strip0x(string(want)))
	if count != 1 {
		t.Fatalf("%s count = %d, want 1\nwant = %s\ncallData = %s", label, count, want, callData)
	}
}
