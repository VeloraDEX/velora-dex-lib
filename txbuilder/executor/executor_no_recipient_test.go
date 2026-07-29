package executor

import (
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const testAugustusV6 resolved.Address = "0x6666666666666666666666666666666666666666"

func testEncodingContextWithAugustus() resolved.EncodingContext {
	return resolved.EncodingContext{
		WrappedNativeTokenAddress: testWETH,
		AugustusV6Address:         testAugustusV6,
	}
}

func TestExecutor010203ValidationAcceptsNoRecipient(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].DexFuncHasRecipient = false

	if err := NewExecutor01Builder(testEncodingContext()).validateExecutor01Input(priceRoute, exchangeParams, nil); err != nil {
		t.Fatalf("Executor01 rejected dexFuncHasRecipient=false: %v", err)
	}
	if err := NewExecutor02Builder(testEncodingContext()).validateExecutor02Input(priceRoute, exchangeParams); err != nil {
		t.Fatalf("Executor02 rejected dexFuncHasRecipient=false: %v", err)
	}

	orderedLegs, err := getOrderedLegs(testBytecodeBuildInput(priceRoute, exchangeParams))
	if err != nil {
		t.Fatalf("getOrderedLegs() error = %v", err)
	}
	if err := NewExecutor03Builder(testEncodingContext()).validateExecutor03Input(priceRoute, orderedLegs, nil); err != nil {
		t.Fatalf("Executor03 rejected dexFuncHasRecipient=false: %v", err)
	}
}

func TestExecutor0102NoRecipientSimpleSwapTransferTrailer(t *testing.T) {
	for _, tc := range []struct {
		name           string
		buildFlags     func(executorRoute, []resolved.DexExchangeBuildParam) ([]flag, error)
		build          func(resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error)
		transferAmount resolved.DecimalString
	}{
		{
			name: "Executor01",
			buildFlags: func(priceRoute executorRoute, exchangeParams []resolved.DexExchangeBuildParam) ([]flag, error) {
				flags, err := NewExecutor01Builder(testEncodingContextWithAugustus()).buildFlags(priceRoute, exchangeParams, nil)
				return flags.dexes, err
			},
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor01Builder(testEncodingContextWithAugustus()).BuildBytecode(input)
			},
			// Executor01 forwards the route-level destAmount.
			transferAmount: "456",
		},
		{
			name: "Executor02",
			buildFlags: func(priceRoute executorRoute, exchangeParams []resolved.DexExchangeBuildParam) ([]flag, error) {
				flags, err := NewExecutor02Builder(testEncodingContextWithAugustus()).buildFlags(priceRoute, exchangeParams, nil)
				return flags.dexes, err
			},
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor02Builder(testEncodingContextWithAugustus()).BuildBytecode(input)
			},
			// Executor02 forwards the swap-exchange destAmount.
			transferAmount: "456",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			exchangeParams[0].ReturnAmountPos = nil
			exchangeParams[0].DexFuncHasRecipient = false

			dexFlags, err := tc.buildFlags(priceRoute, exchangeParams)
			if err != nil {
				t.Fatalf("buildFlags() error = %v", err)
			}
			if dexFlags[0] != insertFromAmountCheckSrcTokenBalanceAfterSwap {
				t.Fatalf("dex flag = %d, want %d", dexFlags[0], insertFromAmountCheckSrcTokenBalanceAfterSwap)
			}

			callData, err := tc.build(testBytecodeBuildInput(priceRoute, exchangeParams))
			if err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}

			transferCalldata, err := buildERC20TransferCalldata(testAugustusV6, tc.transferAmount)
			if err != nil {
				t.Fatalf("buildERC20TransferCalldata() error = %v", err)
			}
			wrappedTransferCalldata, err := buildTransferCallData(transferCalldata, testDestToken)
			if err != nil {
				t.Fatalf("buildTransferCallData() error = %v", err)
			}
			assertHexContainsOnce(t, callData, wrappedTransferCalldata, "transfer-to-Augustus trailer")

			raw := strip0x(string(callData))
			transferIndex := strings.Index(raw, testTransferSelector)
			dexIndex := strings.Index(raw, "12345678")
			if dexIndex < 0 || transferIndex < 0 {
				t.Fatalf("expected calldata parts missing: %s", callData)
			}
			if transferIndex < dexIndex {
				t.Fatalf("transfer trailer appears before dex calldata\ncallData = %s", callData)
			}
		})
	}
}

func TestExecutor0102NoRecipientEthDestFinalSpecialFlag(t *testing.T) {
	for _, tc := range []struct {
		name        string
		buildFlags  func(executorRoute, []resolved.DexExchangeBuildParam) ([]flag, error)
		build       func(resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error)
		mutateParam func(*resolved.DexExchangeBuildParam)
	}{
		{
			// Executor01 appends the trailer for any no-recipient ETH
			// destination (its scope requires needWrapNative=true).
			name: "Executor01",
			buildFlags: func(priceRoute executorRoute, exchangeParams []resolved.DexExchangeBuildParam) ([]flag, error) {
				flags, err := NewExecutor01Builder(testEncodingContextWithAugustus()).buildFlags(priceRoute, exchangeParams, nil)
				return flags.dexes, err
			},
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor01Builder(testEncodingContextWithAugustus()).BuildBytecode(input)
			},
		},
		{
			// Executor02 suppresses the per-exchange trailer when the route
			// needs a root unwrap, so exercise the non-wrapping dex path.
			name: "Executor02",
			buildFlags: func(priceRoute executorRoute, exchangeParams []resolved.DexExchangeBuildParam) ([]flag, error) {
				flags, err := NewExecutor02Builder(testEncodingContextWithAugustus()).buildFlags(priceRoute, exchangeParams, nil)
				return flags.dexes, err
			},
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor02Builder(testEncodingContextWithAugustus()).BuildBytecode(input)
			},
			mutateParam: func(param *resolved.DexExchangeBuildParam) {
				param.NeedWrapNative = resolved.RawBool{Value: false, Valid: true, Present: true}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			priceRoute.DestToken = resolved.NativeTokenAddress
			priceRoute.BestRoute[0].Swaps[0].DestToken = resolved.NativeTokenAddress
			exchangeParams[0].ReturnAmountPos = nil
			exchangeParams[0].DexFuncHasRecipient = false
			if tc.mutateParam != nil {
				tc.mutateParam(&exchangeParams[0])
			}

			// ETH dest without a withdraw plan keeps the check-ETH-balance arm.
			dexFlags, err := tc.buildFlags(priceRoute, exchangeParams)
			if err != nil {
				t.Fatalf("buildFlags() error = %v", err)
			}
			if dexFlags[0] != insertFromAmountCheckEthBalanceAfterSwap {
				t.Fatalf("dex flag = %d, want %d", dexFlags[0], insertFromAmountCheckEthBalanceAfterSwap)
			}

			callData, err := tc.build(testBytecodeBuildInput(priceRoute, exchangeParams))
			if err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}

			finalSpecialFlagCalldata, err := buildFinalSpecialFlagCalldata(testEncodingContextWithAugustus())
			if err != nil {
				t.Fatalf("buildFinalSpecialFlagCalldata() error = %v", err)
			}
			assertHexContainsOnce(t, callData, finalSpecialFlagCalldata, "final native-send trailer")

			raw := strip0x(string(callData))
			if strings.Contains(raw, testTransferSelector) {
				t.Fatalf("unexpected ERC20 transfer for ETH destination: %s", callData)
			}
		})
	}
}

func TestExecutor01NoRecipientMultiSwapForcesBalanceCheck(t *testing.T) {
	priceRoute, exchangeParams := testExecutor02MultiSwapRouteAndParams(t)
	exchangeParams[1].DexFuncHasRecipient = false

	builder := NewExecutor01Builder(testEncodingContextWithAugustus())
	flags, err := builder.buildFlags(priceRoute, exchangeParams, nil)
	if err != nil {
		t.Fatalf("buildFlags() error = %v", err)
	}
	// Non-last swaps always check balance; the last swap only because
	// dexFuncHasRecipient=false forces it.
	if flags.dexes[0] != insertFromAmountCheckSrcTokenBalanceAfterSwap {
		t.Fatalf("first dex flag = %d, want %d", flags.dexes[0], insertFromAmountCheckSrcTokenBalanceAfterSwap)
	}
	if flags.dexes[1] != insertFromAmountCheckSrcTokenBalanceAfterSwap {
		t.Fatalf("last dex flag = %d, want %d", flags.dexes[1], insertFromAmountCheckSrcTokenBalanceAfterSwap)
	}

	callData, err := builder.BuildBytecode(testBytecodeBuildInput(priceRoute, exchangeParams))
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}

	transferCalldata, err := buildERC20TransferCalldata(testAugustusV6, "456")
	if err != nil {
		t.Fatalf("buildERC20TransferCalldata() error = %v", err)
	}
	wrappedTransferCalldata, err := buildTransferCallData(transferCalldata, testDestToken)
	if err != nil {
		t.Fatalf("buildTransferCallData() error = %v", err)
	}
	assertHexContainsOnce(t, callData, wrappedTransferCalldata, "multi-swap transfer-to-Augustus trailer")
}

func TestExecutor02NoRecipientNonTerminalMultiSwap(t *testing.T) {
	priceRoute, exchangeParams := testExecutor02MultiSwapRouteAndParams(t)
	exchangeParams[0].DexFuncHasRecipient = false

	builder := NewExecutor02Builder(testEncodingContextWithAugustus())
	flags, err := builder.buildFlags(priceRoute, exchangeParams, nil)
	if err != nil {
		t.Fatalf("buildFlags() error = %v", err)
	}
	// dexFuncHasRecipient=false forces the balance check on the non-terminal
	// swap; the terminal recipient-capable swap keeps the default flag.
	if flags.dexes[0] != insertFromAmountCheckSrcTokenBalanceAfterSwap {
		t.Fatalf("first dex flag = %d, want %d", flags.dexes[0], insertFromAmountCheckSrcTokenBalanceAfterSwap)
	}
	if flags.dexes[1] != insertFromAmountDontCheckBalanceAfterSwap {
		t.Fatalf("second dex flag = %d, want %d", flags.dexes[1], insertFromAmountDontCheckBalanceAfterSwap)
	}

	callData, err := builder.BuildBytecode(testBytecodeBuildInput(priceRoute, exchangeParams))
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}

	// Intermediate output is threaded via the balance check, never transferred.
	raw := strip0x(string(callData))
	if strings.Contains(raw, testTransferSelector) {
		t.Fatalf("unexpected ERC20 transfer for non-terminal no-recipient swap: %s", callData)
	}
}

func TestExecutor03NoRecipientFlagsAndBuild(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].DexFuncHasRecipient = false

	builder := NewExecutor03Builder(testEncodingContextWithAugustus())
	flags, err := builder.buildFlags(priceRoute, exchangeParams, nil)
	if err != nil {
		t.Fatalf("buildFlags() error = %v", err)
	}
	if flags.dexes[0] != insertFromAmountCheckSrcTokenBalanceAfterSwap {
		t.Fatalf("dex flag = %d, want %d", flags.dexes[0], insertFromAmountCheckSrcTokenBalanceAfterSwap)
	}

	input := testBytecodeBuildInput(priceRoute, exchangeParams)
	input.ExecutorType = resolved.Executor03
	callData, err := builder.BuildBytecode(input)
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}

	// TS parity: Executor03 threads no-recipient output via the balance check
	// only — no transfer trailer is emitted.
	raw := strip0x(string(callData))
	if strings.Contains(raw, testTransferSelector) {
		t.Fatalf("unexpected ERC20 transfer trailer for Executor03: %s", callData)
	}
}
