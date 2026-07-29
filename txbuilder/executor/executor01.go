package executor

import (
	"fmt"
	"strings"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

type Executor01Builder struct {
	context resolved.EncodingContext
}

func NewExecutor01Builder(context resolved.EncodingContext) Executor01Builder {
	return Executor01Builder{context: context}
}

type executor01Flags struct {
	approves []flag
	dexes    []flag
	wrap     flag
}

func (b Executor01Builder) BuildBytecode(input resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
	priceRoute := buildExecutorRoute(input)
	exchangeParams, err := getExchangeParams(input)
	if err != nil {
		return "", err
	}
	if len(exchangeParams) == 0 {
		return "", fmt.Errorf("Executor01 requires at least one exchange param")
	}
	if err := b.validateExecutor01Input(priceRoute, exchangeParams, input.WethPlan); err != nil {
		return "", err
	}

	flags, err := b.buildFlags(priceRoute, exchangeParams, input.WethPlan)
	if err != nil {
		return "", err
	}

	swapsCalldata := resolved.HexBytes("0x")
	for index := range exchangeParams {
		swapCallData, err := b.buildSingleSwapCallData(
			priceRoute,
			exchangeParams,
			index,
			flags,
			input.WethPlan,
		)
		if err != nil {
			return "", err
		}
		swapsCalldata, err = concatHex(string(swapsCalldata), string(swapCallData))
		if err != nil {
			return "", err
		}
	}

	lastParam := exchangeParams[len(exchangeParams)-1]
	// No-recipient dexes leave the output on the executor; forward it to
	// Augustus with an ERC20 transfer trailer (TS parity).
	if !lastParam.DexFuncHasRecipient && !isETHAddress(priceRoute.DestToken) {
		transferCallData, err := buildERC20TransferCalldata(
			b.context.AugustusV6Address,
			priceRoute.DestAmount,
		)
		if err != nil {
			return "", err
		}
		wrappedTransferCallData, err := buildTransferCallData(
			transferCallData,
			priceRoute.DestToken,
		)
		if err != nil {
			return "", err
		}
		swapsCalldata, err = concatHex(string(swapsCalldata), string(wrappedTransferCallData))
		if err != nil {
			return "", err
		}
	}
	// Final native-send trailer: ETH-destination WETH withdraw, or a
	// no-recipient dex whose native output stays on the executor.
	if (input.WethPlan != nil && input.WethPlan.Withdraw != nil && isETHAddress(priceRoute.DestToken)) ||
		(!lastParam.DexFuncHasRecipient && isETHAddress(priceRoute.DestToken)) {
		finalSpecialFlagCalldata, err := buildFinalSpecialFlagCalldata(b.context)
		if err != nil {
			return "", err
		}
		swapsCalldata, err = concatHex(string(swapsCalldata), string(finalSpecialFlagCalldata))
		if err != nil {
			return "", err
		}
	}

	return buildExecutor01TopLevelBytecode(swapsCalldata)
}

func (b Executor01Builder) validateExecutor01Input(
	priceRoute executorRoute,
	exchangeParams []resolved.DexExchangeBuildParam,
	wethPlan *resolved.WethPlan,
) error {
	if len(priceRoute.BestRoute) != 1 {
		return fmt.Errorf("Executor01 does not support routes with multiple entries; mega-swap uses Executor02/Executor03")
	}

	exchangeParamIndex := 0
	for _, route := range priceRoute.BestRoute {
		for _, swap := range route.Swaps {
			if len(swap.SwapExchanges) != 1 {
				return fmt.Errorf("Executor01 supports one swapExchange per swap")
			}
			if exchangeParamIndex >= len(exchangeParams) {
				return fmt.Errorf("missing exchange param for route position")
			}

			exchangeParam := exchangeParams[exchangeParamIndex]
			if exchangeParam.Spender != nil &&
				exchangeParam.ApproveData == nil &&
				!boolValue(exchangeParam.SkipApproval) {
				return fmt.Errorf("Executor01 spender requires approveData or skipApproval after approval planning")
			}
			if err := validateReturnAmountPosOverride("Executor01", exchangeParam.ReturnAmountPos); err != nil {
				return err
			}
			if err := validateInsertFromAmountPosOverride("Executor01", exchangeParam.InsertFromAmountPos); err != nil {
				return err
			}
			if err := validateSpecialDexFlagOverride("Executor01", exchangeParam.SpecialDexFlag); err != nil {
				return err
			}

			exchangeParamIndex++
		}
	}
	if exchangeParamIndex != len(exchangeParams) {
		return fmt.Errorf(
			"exchange param count mismatch: consumed %d, got %d",
			exchangeParamIndex,
			len(exchangeParams),
		)
	}

	return nil
}

func (b Executor01Builder) buildFlags(
	priceRoute executorRoute,
	exchangeParams []resolved.DexExchangeBuildParam,
	wethPlan *resolved.WethPlan,
) (executor01Flags, error) {
	isMegaSwap := len(priceRoute.BestRoute) > 1
	if len(priceRoute.BestRoute) == 0 {
		return executor01Flags{}, fmt.Errorf("Executor01 route must contain at least one route")
	}
	isMultiSwap := !isMegaSwap && len(priceRoute.BestRoute[0].Swaps) > 1

	dexes := make([]flag, 0, len(exchangeParams))
	approves := make([]flag, 0, len(exchangeParams))

	exchangeParamIndex := 0
	for routeIndex, route := range priceRoute.BestRoute {
		for swapIndex, swap := range route.Swaps {
			for swapExchangeIndex := range swap.SwapExchanges {
				if exchangeParamIndex >= len(exchangeParams) {
					return executor01Flags{}, fmt.Errorf(
						"missing exchange param for route position %d:%d:%d",
						routeIndex,
						swapIndex,
						swapExchangeIndex,
					)
				}

				var dexFlag flag
				var approveFlag flag
				var err error
				if isMultiSwap || isMegaSwap {
					dexFlag, approveFlag, err = b.buildMultiMegaSwapFlags(
						priceRoute,
						exchangeParams,
						routeIndex,
						swapIndex,
						exchangeParamIndex,
						wethPlan,
					)
				} else {
					dexFlag, approveFlag, err = b.buildSimpleSwapFlags(
						priceRoute,
						exchangeParams,
						routeIndex,
						swapIndex,
						exchangeParamIndex,
						wethPlan,
					)
				}
				if err != nil {
					return executor01Flags{}, err
				}

				dexes = append(dexes, dexFlag)
				approves = append(approves, approveFlag)
				exchangeParamIndex++
			}
		}
	}

	if exchangeParamIndex != len(exchangeParams) {
		return executor01Flags{}, fmt.Errorf(
			"exchange param count mismatch: consumed %d, got %d",
			exchangeParamIndex,
			len(exchangeParams),
		)
	}

	wrapFlag := insertFromAmountCheckEthBalanceAfterSwap
	if isETHAddress(priceRoute.SrcToken) && wethPlan != nil && wethPlan.Deposit != nil {
		wrapFlag = sendEthEqualToFromAmountDontCheckBalanceAfterSwap
	}

	return executor01Flags{
		dexes:    dexes,
		approves: approves,
		wrap:     wrapFlag,
	}, nil
}

func (b Executor01Builder) buildSimpleSwapFlags(
	priceRoute executorRoute,
	exchangeParams []resolved.DexExchangeBuildParam,
	routeIndex int,
	swapIndex int,
	exchangeParamIndex int,
	wethPlan *resolved.WethPlan,
) (flag, flag, error) {
	swap := priceRoute.BestRoute[routeIndex].Swaps[swapIndex]
	exchangeParam := exchangeParams[exchangeParamIndex]
	isEthSrc := isETHAddress(swap.SrcToken)
	isEthDest := isETHAddress(swap.DestToken)
	isWETHSrc := boolValue(exchangeParam.NeedUnwrapNative) && isWETHAddress(swap.SrcToken, b.context)
	isWETHDest := boolValue(exchangeParam.NeedUnwrapNative) && isWETHAddress(swap.DestToken, b.context)
	needWrap := exchangeParam.NeedWrapNative.Value &&
		isEthSrc &&
		wethPlan != nil &&
		wethPlan.Deposit != nil
	needUnwrap := exchangeParam.NeedWrapNative.Value &&
		isEthDest &&
		wethPlan != nil &&
		wethPlan.Withdraw != nil
	isSpecialDex := exchangeParam.SpecialDexFlag != nil &&
		*exchangeParam.SpecialDexFlag != int(specialDexDefault)
	forcePreventInsertFromAmount :=
		boolValue(exchangeParam.SwappedAmountNotPresentInExchangeData) ||
			(isSpecialDex && !boolValue(exchangeParam.SpecialDexSupportsInsertFromAmount))

	dexFlag := insertFromAmountDontCheckBalanceAfterSwap
	if forcePreventInsertFromAmount {
		dexFlag = dontInsertFromAmountDontCheckBalanceAfterSwap
	}
	approveFlag := dontInsertFromAmountDontCheckBalanceAfterSwap

	if isEthSrc && !needWrap {
		if exchangeParam.DexFuncHasRecipient {
			if !boolValue(exchangeParam.SendEthButSupportsInsertFromAmount) {
				dexFlag = sendEthEqualToFromAmountDontCheckBalanceAfterSwap
			} else {
				dexFlag = sendEthEqualToFromAmountPlusInsertFromAmountDontCheckBalanceAfterSwap
			}
		} else if !boolValue(exchangeParam.SendEthButSupportsInsertFromAmount) {
			dexFlag = sendEthEqualToFromAmountCheckSrcTokenBalanceAfterSwap
		} else {
			dexFlag = sendEthEqualToFromAmountPlusInsertFromAmountCheckSrcTokenBalanceAfterSwap
		}
	} else if isEthDest && !needUnwrap {
		if forcePreventInsertFromAmount {
			dexFlag = dontInsertFromAmountCheckEthBalanceAfterSwap
		} else {
			dexFlag = insertFromAmountCheckEthBalanceAfterSwap
		}
	} else if !exchangeParam.DexFuncHasRecipient || (isEthDest && needUnwrap) {
		if forcePreventInsertFromAmount {
			dexFlag = dontInsertFromAmountCheckSrcTokenBalanceAfterSwap
		} else {
			dexFlag = insertFromAmountCheckSrcTokenBalanceAfterSwap
		}
	}

	if isWETHSrc {
		// Also insert the runtime fromAmount into the dex calldata when the dex
		// supports it: msg.value is the threaded amount, and split slicing can
		// drift a wei from the quoted amount — dexes that require
		// msg.value == amountIn (e.g. FluidDex) revert on the mismatch otherwise.
		if boolValue(exchangeParam.SendEthButSupportsInsertFromAmount) && !forcePreventInsertFromAmount {
			dexFlag = sendEthEqualToFromAmountPlusInsertFromAmountCheckSrcTokenBalanceAfterSwap
		} else {
			dexFlag = sendEthEqualToFromAmountCheckSrcTokenBalanceAfterSwap
		}
	} else if isWETHDest {
		if forcePreventInsertFromAmount {
			dexFlag = dontInsertFromAmountCheckEthBalanceAfterSwap
		} else {
			dexFlag = insertFromAmountCheckEthBalanceAfterSwap
		}
	}

	return dexFlag, approveFlag, nil
}

func (b Executor01Builder) buildMultiMegaSwapFlags(
	priceRoute executorRoute,
	exchangeParams []resolved.DexExchangeBuildParam,
	routeIndex int,
	swapIndex int,
	exchangeParamIndex int,
	wethPlan *resolved.WethPlan,
) (flag, flag, error) {
	swap := priceRoute.BestRoute[routeIndex].Swaps[swapIndex]
	exchangeParam := exchangeParams[exchangeParamIndex]
	isLastSwap := swapIndex == len(priceRoute.BestRoute[routeIndex].Swaps)-1
	isEthSrc := isETHAddress(swap.SrcToken)
	isEthDest := isETHAddress(swap.DestToken)
	isWETHSrc := boolValue(exchangeParam.NeedUnwrapNative) && isWETHAddress(swap.SrcToken, b.context)
	isWETHDest := boolValue(exchangeParam.NeedUnwrapNative) && isWETHAddress(swap.DestToken, b.context)
	isSpecialDex := exchangeParam.SpecialDexFlag != nil &&
		*exchangeParam.SpecialDexFlag != int(specialDexDefault)
	forcePreventInsertFromAmount :=
		boolValue(exchangeParam.SwappedAmountNotPresentInExchangeData) ||
			(isSpecialDex && !boolValue(exchangeParam.SpecialDexSupportsInsertFromAmount))
	needUnwrap := exchangeParam.NeedWrapNative.Value &&
		isEthDest &&
		wethPlan != nil &&
		wethPlan.Withdraw != nil
	forceBalanceOfCheck := true
	if isLastSwap {
		forceBalanceOfCheck = !exchangeParam.DexFuncHasRecipient || needUnwrap
	}
	needSendEth := isEthSrc && !exchangeParam.NeedWrapNative.Value
	needCheckEthBalance := isEthDest && !exchangeParam.NeedWrapNative.Value
	needCheckSrcTokenBalanceOf := needUnwrap && !isLastSwap

	dexFlag := insertFromAmountDontCheckBalanceAfterSwap
	approveFlag := dontInsertFromAmountDontCheckBalanceAfterSwap
	if needSendEth {
		preventInsertForSendEth :=
			forcePreventInsertFromAmount ||
				!boolValue(exchangeParam.SendEthButSupportsInsertFromAmount)
		if forceBalanceOfCheck {
			if preventInsertForSendEth {
				dexFlag = sendEthEqualToFromAmountCheckSrcTokenBalanceAfterSwap
			} else {
				dexFlag = sendEthEqualToFromAmountPlusInsertFromAmountCheckSrcTokenBalanceAfterSwap
			}
		} else if exchangeParam.DexFuncHasRecipient {
			if preventInsertForSendEth {
				dexFlag = sendEthEqualToFromAmountDontCheckBalanceAfterSwap
			} else {
				dexFlag = sendEthEqualToFromAmountPlusInsertFromAmountDontCheckBalanceAfterSwap
			}
		} else if preventInsertForSendEth {
			dexFlag = sendEthEqualToFromAmountCheckSrcTokenBalanceAfterSwap
		} else {
			dexFlag = sendEthEqualToFromAmountPlusInsertFromAmountCheckSrcTokenBalanceAfterSwap
		}
	} else if needCheckEthBalance {
		if needCheckSrcTokenBalanceOf || forceBalanceOfCheck {
			if forcePreventInsertFromAmount && exchangeParam.DexFuncHasRecipient {
				dexFlag = dontInsertFromAmountCheckEthBalanceAfterSwap
			} else {
				dexFlag = insertFromAmountCheckEthBalanceAfterSwap
			}
		} else if forcePreventInsertFromAmount && exchangeParam.DexFuncHasRecipient {
			dexFlag = dontInsertFromAmountDontCheckBalanceAfterSwap
		} else {
			dexFlag = insertFromAmountDontCheckBalanceAfterSwap
		}
	} else {
		if needCheckSrcTokenBalanceOf || forceBalanceOfCheck {
			if forcePreventInsertFromAmount {
				dexFlag = dontInsertFromAmountCheckSrcTokenBalanceAfterSwap
			} else {
				dexFlag = insertFromAmountCheckSrcTokenBalanceAfterSwap
			}
		} else if forcePreventInsertFromAmount {
			dexFlag = dontInsertFromAmountDontCheckBalanceAfterSwap
		} else {
			dexFlag = insertFromAmountDontCheckBalanceAfterSwap
		}
	}

	if isWETHSrc {
		// Also insert the runtime fromAmount into the dex calldata when the dex
		// supports it: msg.value is the threaded amount, and split slicing can
		// drift a wei from the quoted amount — dexes that require
		// msg.value == amountIn (e.g. FluidDex) revert on the mismatch otherwise.
		if boolValue(exchangeParam.SendEthButSupportsInsertFromAmount) && !forcePreventInsertFromAmount {
			dexFlag = sendEthEqualToFromAmountPlusInsertFromAmountCheckSrcTokenBalanceAfterSwap
		} else {
			dexFlag = sendEthEqualToFromAmountCheckSrcTokenBalanceAfterSwap
		}
	} else if isWETHDest {
		if forcePreventInsertFromAmount {
			dexFlag = dontInsertFromAmountCheckEthBalanceAfterSwap
		} else {
			dexFlag = insertFromAmountCheckEthBalanceAfterSwap
		}
	}

	return dexFlag, approveFlag, nil
}

func (b Executor01Builder) buildSingleSwapCallData(
	priceRoute executorRoute,
	exchangeParams []resolved.DexExchangeBuildParam,
	index int,
	flags executor01Flags,
	wethPlan *resolved.WethPlan,
) (resolved.HexBytes, error) {
	if len(priceRoute.BestRoute) != 1 {
		return "", fmt.Errorf("Executor01 does not support routes with multiple entries; mega-swap uses Executor02/Executor03")
	}
	if index >= len(priceRoute.BestRoute[0].Swaps) {
		return "", fmt.Errorf("missing swap for exchange param index %d", index)
	}

	swap := priceRoute.BestRoute[0].Swaps[index]
	curExchangeParam := exchangeParams[index]
	dexCallData, err := b.buildDexCallData(
		priceRoute,
		exchangeParams,
		0,
		index,
		0,
		index,
		flags.dexes[index],
	)
	if err != nil {
		return "", err
	}

	swapCallData := dexCallData
	isWETHSrcUnwrap :=
		boolValue(curExchangeParam.NeedUnwrapNative) &&
			isWETHAddress(swap.SrcToken, b.context)
	isWETHDestWrap :=
		boolValue(curExchangeParam.NeedUnwrapNative) &&
			isWETHAddress(swap.DestToken, b.context)
	if isWETHSrcUnwrap {
		withdrawRawCalldata, err := buildERC20WithdrawCalldata(swap.SwapExchanges[0].SrcAmount)
		if err != nil {
			return "", err
		}
		withdrawCallData, err := buildUnwrapEthCallData(
			getWETHAddress(curExchangeParam, b.context),
			withdrawRawCalldata,
		)
		if err != nil {
			return "", err
		}
		swapCallData, err = concatHex(string(withdrawCallData), string(dexCallData))
		if err != nil {
			return "", err
		}
	}

	// DEX returns native ETH on a WETH-dest hop; wrap it after the swap.
	if isWETHDestWrap {
		depositRawCalldata, err := buildERC20DepositCalldata()
		if err != nil {
			return "", err
		}
		depositCallData, err := buildWrapEthCallData(
			getWETHAddress(curExchangeParam, b.context),
			depositRawCalldata,
			sendEthEqualToFromAmountDontCheckBalanceAfterSwap,
			0,
		)
		if err != nil {
			return "", err
		}
		swapCallData, err = concatHex(string(swapCallData), string(depositCallData))
		if err != nil {
			return "", err
		}
	}

	if curExchangeParam.TransferSrcTokenBeforeSwap != nil {
		transferCallData, err := buildERC20TransferCalldata(
			*curExchangeParam.TransferSrcTokenBeforeSwap,
			swap.SwapExchanges[0].SrcAmount,
		)
		if err != nil {
			return "", err
		}
		tokenAddress := resolved.Address(lowerHex(string(swap.SrcToken)))
		if isETHAddress(swap.SrcToken) {
			tokenAddress = getWETHAddress(curExchangeParam, b.context)
		}
		wrappedTransferCallData, err := buildTransferCallData(transferCallData, tokenAddress)
		if err != nil {
			return "", err
		}
		swapCallData, err = concatHex(string(wrappedTransferCallData), string(swapCallData))
		if err != nil {
			return "", err
		}
	}

	if int(flags.dexes[index])%4 != 1 &&
		(!isETHAddress(swap.SrcToken) || (isETHAddress(swap.SrcToken) && index != 0)) &&
		curExchangeParam.TransferSrcTokenBeforeSwap == nil &&
		!boolValue(curExchangeParam.SkipApproval) &&
		curExchangeParam.ApproveData != nil {
		approveCallData, err := buildApproveCallData(
			b.context,
			curExchangeParam.ApproveData.Target,
			curExchangeParam.ApproveData.Token,
			flags.approves[index],
			boolValue(curExchangeParam.Permit2Approval),
			maxUint,
		)
		if err != nil {
			return "", err
		}
		swapCallData, err = concatHex(string(approveCallData), string(swapCallData))
		if err != nil {
			return "", err
		}
	}

	if curExchangeParam.NeedWrapNative.Value && wethPlan != nil {
		if wethPlan.Deposit != nil && isETHAddress(swap.SrcToken) {
			var prevExchangeParam *resolved.DexExchangeBuildParam
			if index > 0 {
				prevExchangeParam = &exchangeParams[index-1]
			}
			if prevExchangeParam == nil || !prevExchangeParam.NeedWrapNative.Value {
				approveWethCallData := resolved.HexBytes("0x")
				if curExchangeParam.ApproveData != nil &&
					curExchangeParam.TransferSrcTokenBeforeSwap == nil &&
					!boolValue(curExchangeParam.SkipApproval) {
					approveWethCallData, err = buildApproveCallData(
						b.context,
						curExchangeParam.ApproveData.Target,
						curExchangeParam.ApproveData.Token,
						flags.approves[index],
						boolValue(curExchangeParam.Permit2Approval),
						maxUint,
					)
					if err != nil {
						return "", err
					}
				}

				depositCallData, err := buildWrapEthCallData(
					getWETHAddress(curExchangeParam, b.context),
					wethPlan.Deposit.Calldata,
					sendEthEqualToFromAmountDontCheckBalanceAfterSwap,
					0,
				)
				if err != nil {
					return "", err
				}
				swapCallData, err = concatHex(
					string(approveWethCallData),
					string(depositCallData),
					string(swapCallData),
				)
				if err != nil {
					return "", err
				}
			}
		}

		if wethPlan.Withdraw != nil && isETHAddress(swap.DestToken) {
			var nextExchangeParam *resolved.DexExchangeBuildParam
			if index+1 < len(exchangeParams) {
				nextExchangeParam = &exchangeParams[index+1]
			}
			if nextExchangeParam == nil || !nextExchangeParam.NeedWrapNative.Value {
				withdrawCallData, err := buildUnwrapEthCallData(
					getWETHAddress(curExchangeParam, b.context),
					wethPlan.Withdraw.Calldata,
				)
				if err != nil {
					return "", err
				}
				swapCallData, err = concatHex(string(swapCallData), string(withdrawCallData))
				if err != nil {
					return "", err
				}
			}
		}
	}

	return swapCallData, nil
}

func (b Executor01Builder) buildDexCallData(
	priceRoute executorRoute,
	exchangeParams []resolved.DexExchangeBuildParam,
	routeIndex int,
	swapIndex int,
	swapExchangeIndex int,
	exchangeParamIndex int,
	dexFlag flag,
) (resolved.HexBytes, error) {
	exchangeParam := exchangeParams[exchangeParamIndex]
	swap := priceRoute.BestRoute[routeIndex].Swaps[swapIndex]
	exchangeData := resolved.HexBytes(lowerHex(string(exchangeParam.ExchangeData)))

	dontCheckBalanceAfterSwap := int(dexFlag)%3 == 0
	checkDestTokenBalanceAfterSwap := int(dexFlag)%3 == 2
	insertFromAmount := int(dexFlag)%4 == 3 || int(dexFlag)%4 == 2

	returnAmountPos := defaultReturnAmountPos
	if exchangeParam.ReturnAmountPos != nil {
		returnAmountPos = *exchangeParam.ReturnAmountPos
	}

	srcTokenPos := 0
	if checkDestTokenBalanceAfterSwap && !dontCheckBalanceAfterSwap {
		destTokenAddress := swap.DestToken
		if isETHAddress(swap.DestToken) {
			destTokenAddress = getWETHAddress(exchangeParam, b.context)
		}
		lowercasedDestTokenAddress := resolved.Address(lowerHex(string(destTokenAddress)))

		var err error
		exchangeData, err = addTokenAddressToCallData(exchangeData, lowercasedDestTokenAddress)
		if err != nil {
			return "", err
		}
		rawIndex := strings.Index(strip0x(string(exchangeData)), strip0x(string(lowercasedDestTokenAddress)))
		if rawIndex == -1 {
			return "", fmt.Errorf("destination token address not found in exchangeData")
		}
		// 24 hex chars are the 12 zero bytes before an ABI-word address.
		srcTokenPos = (rawIndex - 24) / 2
	}

	fromAmountPos := 0
	if insertFromAmount {
		// Zero is falsy in the legacy truthiness check, so it falls through
		// to the calldata search.
		if exchangeParam.InsertFromAmountPos != nil && *exchangeParam.InsertFromAmountPos != 0 {
			fromAmountPos = *exchangeParam.InsertFromAmountPos
		} else {
			var err error
			fromAmountPos, err = findAmountPosWithFallback(
				exchangeData,
				swap.SwapExchanges[swapExchangeIndex].SrcAmount,
				boolValue(exchangeParam.AmountsPacked128),
			)
			if err != nil {
				return "", err
			}
		}
	}

	specialFlag := specialDexDefault
	if exchangeParam.SpecialDexFlag != nil {
		specialFlag = specialDex(*exchangeParam.SpecialDexFlag)
	}

	finalFlag := dexFlag
	if boolValue(exchangeParam.AmountsPacked128) {
		finalFlag = flag(int(dexFlag) | 0x8000)
	}

	return buildExecutor0102CallData(
		exchangeParam.TargetExchange,
		exchangeData,
		fromAmountPos,
		srcTokenPos,
		specialFlag,
		finalFlag,
		returnAmountPos,
	)
}
