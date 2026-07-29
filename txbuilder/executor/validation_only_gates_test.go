package executor

import (
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

func TestExecutor01SendEthButSupportsInsertFromAmount(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	priceRoute.SrcToken = resolved.NativeTokenAddress
	priceRoute.BestRoute[0].Swaps[0].SrcToken = resolved.NativeTokenAddress
	sendEthButInsert := true
	insertFromAmountPos := 68
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].SendEthButSupportsInsertFromAmount = &sendEthButInsert
	exchangeParams[0].InsertFromAmountPos = &insertFromAmountPos

	builder := NewExecutor01Builder(testEncodingContext())
	flags, err := builder.buildFlags(priceRoute, exchangeParams, nil)
	if err != nil {
		t.Fatalf("buildFlags() error = %v", err)
	}
	if flags.dexes[0] != sendEthEqualToFromAmountPlusInsertFromAmountDontCheckBalanceAfterSwap {
		t.Fatalf(
			"dex flag = %d, want %d",
			flags.dexes[0],
			sendEthEqualToFromAmountPlusInsertFromAmountDontCheckBalanceAfterSwap,
		)
	}

	callData, err := builder.buildDexCallData(
		priceRoute,
		exchangeParams,
		0,
		0,
		0,
		0,
		flags.dexes[0],
	)
	if err != nil {
		t.Fatalf("buildDexCallData() error = %v", err)
	}
	assertExecutor0102FromAmountPos(t, callData, insertFromAmountPos)
	assertExecutor0102Flag(t, callData, sendEthEqualToFromAmountPlusInsertFromAmountDontCheckBalanceAfterSwap)

	if _, err := builder.BuildBytecode(testBytecodeBuildInput(priceRoute, exchangeParams)); err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}
}

func TestExecutor01SwappedAmountNotPresentInExchangeData(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	swappedAmountMissing := true
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].SwappedAmountNotPresentInExchangeData = &swappedAmountMissing

	builder := NewExecutor01Builder(testEncodingContext())
	flags, err := builder.buildFlags(priceRoute, exchangeParams, nil)
	if err != nil {
		t.Fatalf("buildFlags() error = %v", err)
	}
	if flags.dexes[0] != dontInsertFromAmountDontCheckBalanceAfterSwap {
		t.Fatalf("dex flag = %d, want %d", flags.dexes[0], dontInsertFromAmountDontCheckBalanceAfterSwap)
	}

	callData, err := builder.buildDexCallData(
		priceRoute,
		exchangeParams,
		0,
		0,
		0,
		0,
		flags.dexes[0],
	)
	if err != nil {
		t.Fatalf("buildDexCallData() error = %v", err)
	}
	assertExecutor0102FromAmountPos(t, callData, 0)
	assertExecutor0102Flag(t, callData, dontInsertFromAmountDontCheckBalanceAfterSwap)

	if _, err := builder.BuildBytecode(testBytecodeBuildInput(priceRoute, exchangeParams)); err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}
}

func TestExecutor01SpecialDexSupportsInsertFromAmount(t *testing.T) {
	for _, tc := range []struct {
		name        string
		specialFlag specialDex
		supports    bool
		wantFlag    flag
		wantFromPos int
	}{
		{
			name:        "supports_insert",
			specialFlag: specialDexSwapOnAugustusRFQ,
			supports:    true,
			wantFlag:    insertFromAmountDontCheckBalanceAfterSwap,
			wantFromPos: 68,
		},
		{
			name:        "prevents_insert",
			specialFlag: specialDexSwapOnHashflow,
			supports:    false,
			wantFlag:    dontInsertFromAmountDontCheckBalanceAfterSwap,
			wantFromPos: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			insertFromAmountPos := 68
			specialFlag := int(tc.specialFlag)
			exchangeParams[0].ReturnAmountPos = nil
			exchangeParams[0].SpecialDexFlag = &specialFlag
			exchangeParams[0].SpecialDexSupportsInsertFromAmount = &tc.supports
			exchangeParams[0].InsertFromAmountPos = &insertFromAmountPos

			builder := NewExecutor01Builder(testEncodingContext())
			flags, err := builder.buildFlags(priceRoute, exchangeParams, nil)
			if err != nil {
				t.Fatalf("buildFlags() error = %v", err)
			}
			if flags.dexes[0] != tc.wantFlag {
				t.Fatalf("dex flag = %d, want %d", flags.dexes[0], tc.wantFlag)
			}

			callData, err := builder.buildDexCallData(
				priceRoute,
				exchangeParams,
				0,
				0,
				0,
				0,
				flags.dexes[0],
			)
			if err != nil {
				t.Fatalf("buildDexCallData() error = %v", err)
			}
			assertExecutor0102FromAmountPos(t, callData, tc.wantFromPos)
			assertExecutor0102SpecialFlag(t, callData, tc.specialFlag)
			assertExecutor0102Flag(t, callData, tc.wantFlag)

			if _, err := builder.BuildBytecode(testBytecodeBuildInput(priceRoute, exchangeParams)); err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}
		})
	}
}

func TestExecutor010203SpecialDexFlagValidationAndPacking(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	exchangeParams[0].ReturnAmountPos = nil

	executor0102SpecialFlag := int(specialDexSwapOnAugustusRFQ)
	exchangeParams[0].SpecialDexFlag = &executor0102SpecialFlag
	if err := NewExecutor01Builder(testEncodingContext()).validateExecutor01Input(priceRoute, exchangeParams, nil); err != nil {
		t.Fatalf("Executor01 valid specialDexFlag rejected: %v", err)
	}
	if err := NewExecutor02Builder(testEncodingContext()).validateExecutor02Input(priceRoute, exchangeParams); err != nil {
		t.Fatalf("Executor02 valid specialDexFlag rejected: %v", err)
	}

	callData, err := NewExecutor02Builder(testEncodingContext()).buildDexCallData(
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
	assertExecutor0102SpecialFlag(t, callData, specialDexSwapOnAugustusRFQ)
	assertExecutor0102Flag(t, callData, dontInsertFromAmountDontCheckBalanceAfterSwap)

	executor03SpecialFlag := int(specialDexBuyOnSolidlyV3)
	exchangeParams[0].SpecialDexFlag = &executor03SpecialFlag
	orderedLegs, err := getOrderedLegs(testBytecodeBuildInput(priceRoute, exchangeParams))
	if err != nil {
		t.Fatalf("getOrderedLegs() error = %v", err)
	}
	if err := NewExecutor03Builder(testEncodingContext()).validateExecutor03Input(priceRoute, orderedLegs, nil); err != nil {
		t.Fatalf("Executor03 valid specialDexFlag rejected: %v", err)
	}
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
	assertExecutor03SpecialFlag(t, callData, specialDexBuyOnSolidlyV3)
	assertExecutor03Flag(t, callData, dontInsertFromAmountDontCheckBalanceAfterSwap)

	unsupported := int(specialDexExecuteVerticalBranching)
	exchangeParams[0].SpecialDexFlag = &unsupported
	if err := NewExecutor01Builder(testEncodingContext()).validateExecutor01Input(priceRoute, exchangeParams, nil); err == nil {
		t.Fatalf("Executor01 accepted unsupported specialDexFlag")
	}
	if err := NewExecutor02Builder(testEncodingContext()).validateExecutor02Input(priceRoute, exchangeParams); err == nil {
		t.Fatalf("Executor02 accepted unsupported specialDexFlag")
	}
	orderedLegs, err = getOrderedLegs(testBytecodeBuildInput(priceRoute, exchangeParams))
	if err != nil {
		t.Fatalf("getOrderedLegs() error = %v", err)
	}
	if err := NewExecutor03Builder(testEncodingContext()).validateExecutor03Input(priceRoute, orderedLegs, nil); err == nil {
		t.Fatalf("Executor03 accepted unsupported specialDexFlag")
	}
}

func TestExecutor0203ValidationOnlyFlagFields(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mutateRoute func(*executorRoute)
		mutateParam func(*resolved.DexExchangeBuildParam)
		wantFlag    flag
		wantFromPos int
		wantSpecial specialDex
	}{
		{
			name: "send_eth_supports_insert",
			mutateRoute: func(priceRoute *executorRoute) {
				priceRoute.SrcToken = resolved.NativeTokenAddress
				priceRoute.BestRoute[0].Swaps[0].SrcToken = resolved.NativeTokenAddress
			},
			mutateParam: func(param *resolved.DexExchangeBuildParam) {
				sendEthButInsert := true
				insertFromAmountPos := 68
				param.NeedWrapNative = resolved.RawBool{Value: false, Valid: true, Present: true}
				param.SendEthButSupportsInsertFromAmount = &sendEthButInsert
				param.InsertFromAmountPos = &insertFromAmountPos
			},
			wantFlag:    sendEthEqualToFromAmountPlusInsertFromAmountDontCheckBalanceAfterSwap,
			wantFromPos: 68,
			wantSpecial: specialDexDefault,
		},
		{
			name: "swapped_amount_missing",
			mutateParam: func(param *resolved.DexExchangeBuildParam) {
				swappedAmountMissing := true
				param.SwappedAmountNotPresentInExchangeData = &swappedAmountMissing
			},
			wantFlag:    dontInsertFromAmountDontCheckBalanceAfterSwap,
			wantFromPos: 0,
			wantSpecial: specialDexDefault,
		},
		{
			name: "special_dex_supports_insert",
			mutateParam: func(param *resolved.DexExchangeBuildParam) {
				specialFlag := int(specialDexSwapOnAugustusRFQ)
				supportsInsert := true
				insertFromAmountPos := 68
				param.SpecialDexFlag = &specialFlag
				param.SpecialDexSupportsInsertFromAmount = &supportsInsert
				param.InsertFromAmountPos = &insertFromAmountPos
			},
			wantFlag:    insertFromAmountDontCheckBalanceAfterSwap,
			wantFromPos: 68,
			wantSpecial: specialDexSwapOnAugustusRFQ,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			priceRoute, exchangeParams := testExecutorRouteAndParams(0)
			exchangeParams[0].ReturnAmountPos = nil
			if tc.mutateRoute != nil {
				tc.mutateRoute(&priceRoute)
			}
			tc.mutateParam(&exchangeParams[0])

			executor02Builder := NewExecutor02Builder(testEncodingContext())
			executor02Flags, err := executor02Builder.buildFlags(priceRoute, exchangeParams, nil)
			if err != nil {
				t.Fatalf("Executor02 buildFlags() error = %v", err)
			}
			if executor02Flags.dexes[0] != tc.wantFlag {
				t.Fatalf("Executor02 dex flag = %d, want %d", executor02Flags.dexes[0], tc.wantFlag)
			}
			callData, err := executor02Builder.buildDexCallData(
				priceRoute,
				0,
				0,
				0,
				exchangeParams,
				0,
				executor02Flags.dexes[0],
			)
			if err != nil {
				t.Fatalf("Executor02 buildDexCallData() error = %v", err)
			}
			assertExecutor0102FromAmountPos(t, callData, tc.wantFromPos)
			assertExecutor0102SpecialFlag(t, callData, tc.wantSpecial)
			assertExecutor0102Flag(t, callData, tc.wantFlag)
			if _, err := executor02Builder.BuildBytecode(testBytecodeBuildInput(priceRoute, exchangeParams)); err != nil {
				t.Fatalf("Executor02 BuildBytecode() error = %v", err)
			}

			executor03Builder := NewExecutor03Builder(testEncodingContext())
			executor03Flags, err := executor03Builder.buildFlags(priceRoute, exchangeParams, nil)
			if err != nil {
				t.Fatalf("Executor03 buildFlags() error = %v", err)
			}
			if executor03Flags.dexes[0] != tc.wantFlag {
				t.Fatalf("Executor03 dex flag = %d, want %d", executor03Flags.dexes[0], tc.wantFlag)
			}
			callData, err = executor03Builder.buildDexCallData(
				priceRoute,
				0,
				0,
				0,
				exchangeParams,
				0,
				executor03Flags.dexes[0],
				nil,
			)
			if err != nil {
				t.Fatalf("Executor03 buildDexCallData() error = %v", err)
			}
			assertExecutor03FromAmountPos(t, callData, tc.wantFromPos)
			assertExecutor03SpecialFlag(t, callData, tc.wantSpecial)
			assertExecutor03Flag(t, callData, tc.wantFlag)

			input := testBytecodeBuildInput(priceRoute, exchangeParams)
			input.ExecutorType = resolved.Executor03
			input.Context = testEncodingContext()
			if _, err := executor03Builder.BuildBytecode(input); err != nil {
				t.Fatalf("Executor03 BuildBytecode() error = %v", err)
			}
		})
	}
}

func TestExecutor02MultiSwapSpecialDexForcesBalanceCheck(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	secondSwap := priceRoute.BestRoute[0].Swaps[0]
	secondSwap.SrcToken = testDestToken
	secondSwap.DestToken = resolved.Address("0x5555555555555555555555555555555555555555")
	secondSwap.SrcAmount = "456"
	secondSwap.DestAmount = "789"
	secondSwap.SwapExchanges[0].SrcAmount = "456"
	secondSwap.SwapExchanges[0].DestAmount = "789"
	priceRoute.BestRoute[0].Swaps = append(priceRoute.BestRoute[0].Swaps, secondSwap)
	priceRoute.DestToken = secondSwap.DestToken
	priceRoute.DestAmount = secondSwap.DestAmount

	firstSpecialFlag := int(specialDexSwapOnAugustusRFQ)
	firstSupportsInsert := true
	firstInsertFromAmountPos := 68
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].SpecialDexFlag = &firstSpecialFlag
	exchangeParams[0].SpecialDexSupportsInsertFromAmount = &firstSupportsInsert
	exchangeParams[0].InsertFromAmountPos = &firstInsertFromAmountPos

	secondExchangeParam := exchangeParams[0]
	secondExchangeParam.SpecialDexFlag = nil
	secondExchangeParam.SpecialDexSupportsInsertFromAmount = nil
	secondExchangeParam.InsertFromAmountPos = nil
	secondExchangeParam.ExchangeData = testExchangeDataForAmounts("456", "789")
	exchangeParams = append(exchangeParams, secondExchangeParam)

	builder := NewExecutor02Builder(testEncodingContext())
	flags, err := builder.buildFlags(priceRoute, exchangeParams, nil)
	if err != nil {
		t.Fatalf("buildFlags() error = %v", err)
	}
	if flags.dexes[0] != insertFromAmountCheckSrcTokenBalanceAfterSwap {
		t.Fatalf("first dex flag = %d, want %d", flags.dexes[0], insertFromAmountCheckSrcTokenBalanceAfterSwap)
	}
	if flags.dexes[1] != insertFromAmountDontCheckBalanceAfterSwap {
		t.Fatalf("second dex flag = %d, want %d", flags.dexes[1], insertFromAmountDontCheckBalanceAfterSwap)
	}

	callData, err := builder.buildDexCallData(
		priceRoute,
		0,
		0,
		0,
		exchangeParams,
		0,
		flags.dexes[0],
	)
	if err != nil {
		t.Fatalf("buildDexCallData() error = %v", err)
	}
	assertExecutor0102FromAmountPos(t, callData, firstInsertFromAmountPos)
	assertExecutor0102SpecialFlag(t, callData, specialDexSwapOnAugustusRFQ)
	assertExecutor0102Flag(t, callData, insertFromAmountCheckSrcTokenBalanceAfterSwap)

	if _, err := builder.BuildBytecode(testBytecodeBuildInput(priceRoute, exchangeParams)); err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}
}

func assertExecutor0102SpecialFlag(t *testing.T, callData resolved.HexBytes, want specialDex) {
	t.Helper()

	const specialFlagByteOffset = 29
	assertPackedByte(t, callData, specialFlagByteOffset, int(want), "specialDexFlag")
}

func assertExecutor03SpecialFlag(t *testing.T, callData resolved.HexBytes, want specialDex) {
	t.Helper()

	const specialFlagByteOffset = 28
	assertPackedUint16(t, callData, specialFlagByteOffset, int(want), "specialDexFlag")
}

func assertExecutor0102Flag(t *testing.T, callData resolved.HexBytes, want flag) {
	t.Helper()

	const flagByteOffset = 30
	assertPackedUint16(t, callData, flagByteOffset, int(want), "flag")
}

func assertExecutor03Flag(t *testing.T, callData resolved.HexBytes, want flag) {
	t.Helper()

	const flagByteOffset = 30
	assertPackedUint16(t, callData, flagByteOffset, int(want), "flag")
}

func assertPackedByte(t *testing.T, callData resolved.HexBytes, byteOffset int, want int, field string) {
	t.Helper()

	raw := strip0x(string(callData))
	start := byteOffset * 2
	end := start + 2
	if len(raw) < end {
		t.Fatalf("callData too short: %s", callData)
	}

	got := raw[start:end]
	wantHex, err := leftPadUint(want, 1)
	if err != nil {
		t.Fatalf("leftPadUint() error = %v", err)
	}
	wantHex = strip0x(wantHex)
	if got != wantHex {
		t.Fatalf("%s = %s, want %s\ncallData = %s", field, got, wantHex, callData)
	}
}

func testExchangeDataForAmounts(srcAmount resolved.DecimalString, destAmount resolved.DecimalString) resolved.HexBytes {
	encodedSrcAmount, err := encodeUint256Decimal(srcAmount)
	if err != nil {
		panic(err)
	}
	encodedDestAmount, err := encodeUint256Decimal(destAmount)
	if err != nil {
		panic(err)
	}
	return resolved.HexBytes("0x12345678" + strip0x(encodedSrcAmount) + strip0x(encodedDestAmount))
}
