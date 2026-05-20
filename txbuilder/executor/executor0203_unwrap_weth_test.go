package executor

import (
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const testWithdrawSelector = "2e1a7d4d"

var testCustomWETH = resolved.Address("0x1111111111111111111111111111111111111111")

func TestExecutor0203WETHSourceUnwrapBeforeDexCall(t *testing.T) {
	for _, tc := range []struct {
		name       string
		build      func(resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error)
		wrapUnwrap func(resolved.HexBytes, resolved.Address) (resolved.HexBytes, error)
	}{
		{
			name: "Executor02",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor02Builder(testEncodingContext()).BuildBytecode(input)
			},
			wrapUnwrap: func(withdrawCalldata resolved.HexBytes, wethAddress resolved.Address) (resolved.HexBytes, error) {
				return buildUnwrapEthCallData(wethAddress, withdrawCalldata)
			},
		},
		{
			name: "Executor03",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				input.ExecutorType = resolved.Executor03
				return NewExecutor03Builder(testEncodingContext()).BuildBytecode(input)
			},
			wrapUnwrap: func(withdrawCalldata resolved.HexBytes, wethAddress resolved.Address) (resolved.HexBytes, error) {
				return buildExecutor03UnwrapEthCallData(wethAddress, withdrawCalldata)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			priceRoute.SrcToken = testWETH
			priceRoute.BestRoute[0].Swaps[0].SrcToken = testWETH
			exchangeParams[0].ReturnAmountPos = nil
			needUnwrapNative := true
			exchangeParams[0].NeedUnwrapNative = &needUnwrapNative

			input := testBytecodeBuildInput(priceRoute, exchangeParams)
			bytecode, err := tc.build(input)
			if err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}

			withdrawRawCalldata, err := buildERC20WithdrawCalldata("123")
			if err != nil {
				t.Fatalf("buildERC20WithdrawCalldata() error = %v", err)
			}
			wrappedWithdrawCalldata, err := tc.wrapUnwrap(withdrawRawCalldata, testWETH)
			if err != nil {
				t.Fatalf("wrap withdraw calldata error = %v", err)
			}
			assertHexContainsOnce(t, bytecode, wrappedWithdrawCalldata, "wrapped WETH source withdraw calldata")
			assertSelectorOrder(t, bytecode, testWithdrawSelector, "12345678")
		})
	}
}

func TestExecutor0203WETHDestinationWrapAfterDexCall(t *testing.T) {
	for _, tc := range []struct {
		name    string
		build   func(resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error)
		wrapEth func(resolved.HexBytes, resolved.Address) (resolved.HexBytes, error)
	}{
		{
			name: "Executor02",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor02Builder(testEncodingContext()).BuildBytecode(input)
			},
			wrapEth: func(depositCalldata resolved.HexBytes, wethAddress resolved.Address) (resolved.HexBytes, error) {
				return buildWrapEthCallData(
					wethAddress,
					depositCalldata,
					sendEthEqualToFromAmountDontCheckBalanceAfterSwap,
					0,
				)
			},
		},
		{
			name: "Executor03",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				input.ExecutorType = resolved.Executor03
				return NewExecutor03Builder(testEncodingContext()).BuildBytecode(input)
			},
			wrapEth: func(depositCalldata resolved.HexBytes, wethAddress resolved.Address) (resolved.HexBytes, error) {
				return buildExecutor03WrapEthCallData(
					wethAddress,
					depositCalldata,
					sendEthEqualToFromAmountDontCheckBalanceAfterSwap,
					0,
				)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			priceRoute.DestToken = testWETH
			priceRoute.BestRoute[0].Swaps[0].DestToken = testWETH
			exchangeParams[0].ReturnAmountPos = nil
			needUnwrapNative := true
			exchangeParams[0].NeedUnwrapNative = &needUnwrapNative

			input := testBytecodeBuildInput(priceRoute, exchangeParams)
			bytecode, err := tc.build(input)
			if err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}

			depositRawCalldata, err := buildERC20DepositCalldata()
			if err != nil {
				t.Fatalf("buildERC20DepositCalldata() error = %v", err)
			}
			wrappedDepositCalldata, err := tc.wrapEth(depositRawCalldata, testWETH)
			if err != nil {
				t.Fatalf("wrap deposit calldata error = %v", err)
			}
			assertHexContainsOnce(t, bytecode, wrappedDepositCalldata, "wrapped WETH destination deposit calldata")
			assertSelectorOrder(t, bytecode, "12345678", testDepositSelector)
		})
	}
}

func TestExecutor02MixedNeedWrapNativeLastSwapUnwrap(t *testing.T) {
	priceRoute, exchangeParams := testSplitSwapRouteAndParams(t)
	priceRoute.DestToken = resolved.NativeTokenAddress
	priceRoute.BestRoute[0].Swaps[0].DestToken = resolved.NativeTokenAddress
	exchangeParams[0].NeedWrapNative = resolved.RawBool{Value: true, Valid: true, Present: true}
	exchangeParams[1].NeedWrapNative = resolved.RawBool{Value: false, Valid: true, Present: true}

	input := testBytecodeBuildInput(priceRoute, exchangeParams)
	input.WethPlan = testWethPlan()
	bytecode, err := NewExecutor02Builder(testEncodingContext()).BuildBytecode(input)
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}

	if !strings.Contains(strip0x(string(bytecode)), testWithdrawSelector) {
		t.Fatalf("WETH withdraw calldata missing for mixed needWrapNative route: %s", bytecode)
	}
	assertSelectorOrder(t, bytecode, "aaaaaaaa", testWithdrawSelector)
}

func TestExecutor0203CustomWETHAddress(t *testing.T) {
	for _, tc := range []struct {
		name     string
		build    func(resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error)
		validate func(executorRoute, []resolved.DexExchangeBuildParam) error
	}{
		{
			name: "Executor02",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor02Builder(testEncodingContext()).BuildBytecode(input)
			},
			validate: func(priceRoute executorRoute, exchangeParams []resolved.DexExchangeBuildParam) error {
				return NewExecutor02Builder(testEncodingContext()).validatePhase2cScope(priceRoute, exchangeParams)
			},
		},
		{
			name: "Executor03",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				input.ExecutorType = resolved.Executor03
				return NewExecutor03Builder(testEncodingContext()).BuildBytecode(input)
			},
			validate: func(priceRoute executorRoute, exchangeParams []resolved.DexExchangeBuildParam) error {
				input := testBytecodeBuildInput(priceRoute, exchangeParams)
				input.ExecutorType = resolved.Executor03
				orderedLegs, err := getOrderedLegs(input)
				if err != nil {
					return err
				}
				return NewExecutor03Builder(testEncodingContext()).validatePhase2dScope(priceRoute, orderedLegs, nil)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			exchangeParams[0].ReturnAmountPos = nil
			exchangeParams[0].WethAddress = &testCustomWETH

			if err := tc.validate(priceRoute, exchangeParams); err != nil {
				t.Fatalf("custom WETH validation rejected: %v", err)
			}
			if _, err := tc.build(testBytecodeBuildInput(priceRoute, exchangeParams)); err != nil {
				t.Fatalf("BuildBytecode() with custom WETH error = %v", err)
			}
		})
	}
}

func TestExecutor02CustomWETHUsesCustomAddressForNativeDestUnwrap(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	priceRoute.DestToken = resolved.NativeTokenAddress
	priceRoute.BestRoute[0].Swaps[0].DestToken = resolved.NativeTokenAddress
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].WethAddress = &testCustomWETH

	input := testBytecodeBuildInput(priceRoute, exchangeParams)
	input.WethPlan = testWethPlan()
	bytecode, err := NewExecutor02Builder(testEncodingContext()).BuildBytecode(input)
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}

	raw := strip0x(string(bytecode))
	if !strings.Contains(raw, strip0x(string(testCustomWETH))) {
		t.Fatalf("custom WETH address missing from bytecode: %s", bytecode)
	}
	if !strings.Contains(raw, testWithdrawSelector) {
		t.Fatalf("custom WETH withdraw calldata missing: %s", bytecode)
	}
}

func TestExecutor03RejectsCustomWETHWrapperShapeWithoutTSParity(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	priceRoute.SrcToken = testWETH
	priceRoute.BestRoute[0].Swaps[0].SrcToken = testWETH
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].WethAddress = &testCustomWETH
	needUnwrapNative := true
	exchangeParams[0].NeedUnwrapNative = &needUnwrapNative

	input := testBytecodeBuildInput(priceRoute, exchangeParams)
	input.ExecutorType = resolved.Executor03
	if _, err := NewExecutor03Builder(testEncodingContext()).BuildBytecode(input); err == nil {
		t.Fatalf("Executor03 accepted custom WETH with WETH-source unwrap")
	}
}

func testWethPlan() *resolved.WethPlan {
	return &resolved.WethPlan{
		Deposit:  &resolved.WethSubPlan{Calldata: "0x" + testDepositSelector},
		Withdraw: &resolved.WethSubPlan{Calldata: "0x" + testWithdrawSelector},
	}
}

func assertSelectorOrder(t *testing.T, callData resolved.HexBytes, before string, after string) {
	t.Helper()

	raw := strip0x(string(callData))
	beforeIndex := strings.Index(raw, before)
	afterIndex := strings.Index(raw, after)
	if beforeIndex < 0 || afterIndex < 0 {
		t.Fatalf("expected selectors missing: before=%s after=%s\ncallData=%s", before, after, callData)
	}
	if beforeIndex > afterIndex {
		t.Fatalf(
			"unexpected selector order: before=%s@%d after=%s@%d\ncallData=%s",
			before,
			beforeIndex,
			after,
			afterIndex,
			callData,
		)
	}
}
