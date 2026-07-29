package executor

import (
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const (
	testDisabledMaxApprovalToken resolved.Address = "0xdac17f958d2ee523a2206206994597c13d831ec7"
	testDepositSelector                           = "d0e30db0"
)

func TestExecutor0203ApprovalCalldataHonorsSkipApproval(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error)
	}{
		{
			name: "Executor02",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor02Builder(testEncodingContext()).BuildBytecode(input)
			},
		},
		{
			name: "Executor03",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				input.ExecutorType = resolved.Executor03
				return NewExecutor03Builder(testEncodingContext()).BuildBytecode(input)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			exchangeParams[0].ReturnAmountPos = nil
			exchangeParams[0].ApproveData = &resolved.ApproveData{
				Target: testTargetExchange,
				Token:  testSrcToken,
			}

			callData, err := tc.build(testBytecodeBuildInput(priceRoute, exchangeParams))
			if err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}
			if !strings.Contains(strip0x(string(callData)), testApprovalSelector) {
				t.Fatalf("approval calldata missing when ApproveData is present: %s", callData)
			}

			skipApproval := true
			exchangeParams[0].SkipApproval = &skipApproval
			callData, err = tc.build(testBytecodeBuildInput(priceRoute, exchangeParams))
			if err != nil {
				t.Fatalf("BuildBytecode() with SkipApproval error = %v", err)
			}
			if strings.Contains(strip0x(string(callData)), testApprovalSelector) {
				t.Fatalf("approval calldata present with SkipApproval=true: %s", callData)
			}
		})
	}
}

func TestExecutor0203Permit2ApprovalCalldata(t *testing.T) {
	permit2Calldata, err := buildPermit2ApproveCalldata(
		testSrcToken,
		testTargetExchange,
		maxUint160,
		maxUint48,
	)
	if err != nil {
		t.Fatalf("buildPermit2ApproveCalldata() error = %v", err)
	}
	permit2Selector := strip0x(string(permit2Calldata))[:functionSelectorLength]

	for _, tc := range []struct {
		name  string
		build func(resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error)
	}{
		{
			name: "Executor02",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor02Builder(testEncodingContext()).BuildBytecode(input)
			},
		},
		{
			name: "Executor03",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				input.ExecutorType = resolved.Executor03
				return NewExecutor03Builder(testEncodingContext()).BuildBytecode(input)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			exchangeParams[0].ReturnAmountPos = nil
			permit2Approval := true
			exchangeParams[0].Permit2Approval = &permit2Approval
			exchangeParams[0].ApproveData = &resolved.ApproveData{
				Target: testTargetExchange,
				Token:  testSrcToken,
			}

			callData, err := tc.build(testBytecodeBuildInput(priceRoute, exchangeParams))
			if err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}
			raw := strip0x(string(callData))
			if !strings.Contains(raw, strip0x(permit2Address)) {
				t.Fatalf("Permit2 address missing from approval calldata: %s", callData)
			}
			if !strings.Contains(raw, permit2Selector) {
				t.Fatalf("Permit2 approval selector missing from calldata: %s", callData)
			}
		})
	}
}

func TestExecutor0203ApprovalPrependsDisabledMaxReset(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error)
	}{
		{
			name: "Executor02",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				return NewExecutor02Builder(testApprovalNetworkContext()).BuildBytecode(input)
			},
		},
		{
			name: "Executor03",
			build: func(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
				input.ExecutorType = resolved.Executor03
				return NewExecutor03Builder(testApprovalNetworkContext()).BuildBytecode(input)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			priceRoute.SrcToken = testDisabledMaxApprovalToken
			priceRoute.BestRoute[0].Swaps[0].SrcToken = testDisabledMaxApprovalToken
			exchangeParams[0].ReturnAmountPos = nil
			exchangeParams[0].ApproveData = &resolved.ApproveData{
				Target: testTargetExchange,
				Token:  testDisabledMaxApprovalToken,
			}

			callData, err := tc.build(testBytecodeBuildInput(priceRoute, exchangeParams))
			if err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}
			if count := strings.Count(strip0x(string(callData)), testApprovalSelector); count != 2 {
				t.Fatalf("approval selector count = %d, want 2 reset+max\ncallData = %s", count, callData)
			}
		})
	}
}

func TestExecutor0203WETHDepositApprovalCalldata(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(executorRoute, []resolved.DexExchangeBuildParam, *resolved.WethPlan) (resolved.HexBytes, error)
	}{
		{
			name: "Executor02",
			build: func(
				priceRoute executorRoute,
				exchangeParams []resolved.DexExchangeBuildParam,
				wethPlan *resolved.WethPlan,
			) (resolved.HexBytes, error) {
				priceRoute.BestRoute[0].Swaps[0].SwapExchanges[0].Percent = 50
				priceRoute.BestRoute[0].Swaps[0].SwapExchanges = append(
					priceRoute.BestRoute[0].Swaps[0].SwapExchanges,
					resolved.RoutePlanSwapExchange{
						Exchange:   "other",
						Percent:    50,
						SrcAmount:  "123",
						DestAmount: "456",
					},
				)
				otherExchangeParam := exchangeParams[0]
				otherExchangeParam.NeedWrapNative = resolved.RawBool{
					Value:   false,
					Valid:   true,
					Present: true,
				}
				otherExchangeParam.ApproveData = nil
				exchangeParams = append(exchangeParams, otherExchangeParam)

				builder := NewExecutor02Builder(testEncodingContext())
				flags, err := builder.buildFlags(priceRoute, exchangeParams, wethPlan)
				if err != nil {
					return "", err
				}
				return builder.buildSingleSwapExchangeCallData(
					priceRoute,
					0,
					0,
					0,
					exchangeParams,
					flags,
					map[string]bool{},
					true,
					false,
					map[int]bool{},
					wethPlan,
					false,
					false,
				)
			},
		},
		{
			name: "Executor03",
			build: func(
				priceRoute executorRoute,
				exchangeParams []resolved.DexExchangeBuildParam,
				wethPlan *resolved.WethPlan,
			) (resolved.HexBytes, error) {
				input := testBytecodeBuildInput(priceRoute, exchangeParams)
				input.ExecutorType = resolved.Executor03
				input.WethPlan = wethPlan
				return NewExecutor03Builder(testEncodingContext()).BuildBytecode(input)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			priceRoute.SrcToken = resolved.NativeTokenAddress
			priceRoute.BestRoute[0].Swaps[0].SrcToken = resolved.NativeTokenAddress
			exchangeParams[0].ReturnAmountPos = nil
			exchangeParams[0].ApproveData = &resolved.ApproveData{
				Target: testTargetExchange,
				Token:  testWETH,
			}
			wethPlan := &resolved.WethPlan{
				Deposit: &resolved.WethSubPlan{Calldata: "0x" + testDepositSelector},
			}

			callData, err := tc.build(priceRoute, exchangeParams, wethPlan)
			if err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}
			raw := strip0x(string(callData))
			approvalIndex := strings.Index(raw, testApprovalSelector)
			depositIndex := strings.Index(raw, testDepositSelector)
			if approvalIndex < 0 {
				t.Fatalf("approval calldata missing before WETH deposit: %s", callData)
			}
			if depositIndex < 0 {
				t.Fatalf("WETH deposit calldata missing: %s", callData)
			}
			if approvalIndex > depositIndex {
				t.Fatalf("approval appears after WETH deposit\ncallData = %s", callData)
			}
		})
	}
}

func TestExecutor0203ValidationAcceptsSkipApprovalAndNormalizedSpender(t *testing.T) {
	for _, tc := range []struct {
		name     string
		validate func(executorRoute, []resolved.DexExchangeBuildParam) error
	}{
		{
			name: "Executor02",
			validate: func(priceRoute executorRoute, exchangeParams []resolved.DexExchangeBuildParam) error {
				return NewExecutor02Builder(testEncodingContext()).validateExecutor02Input(priceRoute, exchangeParams)
			},
		},
		{
			name: "Executor03",
			validate: func(priceRoute executorRoute, exchangeParams []resolved.DexExchangeBuildParam) error {
				orderedLegs, err := getOrderedLegs(testBytecodeBuildInput(priceRoute, exchangeParams))
				if err != nil {
					return err
				}
				return NewExecutor03Builder(testEncodingContext()).validateExecutor03Input(priceRoute, orderedLegs, nil)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			exchangeParams[0].ReturnAmountPos = nil
			skipApproval := true
			spender := testTargetExchange
			exchangeParams[0].SkipApproval = &skipApproval
			exchangeParams[0].Spender = &spender

			if err := tc.validate(priceRoute, exchangeParams); err != nil {
				t.Fatalf("valid skipApproval+spender rejected: %v", err)
			}

			skipApproval = false
			exchangeParams[0].SkipApproval = &skipApproval
			if err := tc.validate(priceRoute, exchangeParams); err == nil {
				t.Fatalf("accepted spender without approveData or skipApproval")
			}

			exchangeParams[0].ApproveData = &resolved.ApproveData{
				Target: spender,
				Token:  testSrcToken,
			}
			if err := tc.validate(priceRoute, exchangeParams); err != nil {
				t.Fatalf("valid spender+approveData rejected: %v", err)
			}
		})
	}
}

func testApprovalNetworkContext() resolved.EncodingContext {
	context := testEncodingContext()
	context.Network = 1
	return context
}
