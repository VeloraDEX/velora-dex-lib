package resolved

import "fmt"

func BuildTransactionFromResolved(input BuildInput, deps BuildDeps) (out BuildOutput, err error) {
	// Surface ExecutorType to the caller even on the error paths
	// below — observability consumers (per-executor failure counts)
	// rely on knowing which executor was attempted, not just that
	// "something failed."
	out.ExecutorType = input.ExecutorType

	validated, err := validateBuildInput(input, deps)
	if err != nil {
		return
	}

	if deps.ExecutorBytecodeBuilderFactory == nil {
		err = fmt.Errorf("executor bytecode builder factory is required")
		return
	}
	if input.ExecutorType == Executor02 &&
		(input.Side == SideBuy ||
			input.ContractMethod == ContractMethodSwapExactAmountOut ||
			input.ContractMethod == ContractMethodSwapExactAmountOutPro) {
		err = fmt.Errorf("Executor02 BUY routes are not implemented in Phase 2c")
		return
	}
	if input.ExecutorType == Executor03 &&
		(input.Side != SideBuy ||
			(input.ContractMethod != ContractMethodSwapExactAmountOut &&
				input.ContractMethod != ContractMethodSwapExactAmountOutPro)) {
		err = fmt.Errorf("Executor03 non-BUY routes are not implemented in Phase 2d")
		return
	}

	bytecodeBuilder, berr := deps.ExecutorBytecodeBuilderFactory.CreateExecutorBytecodeBuilder(
		input.ExecutorType,
		deps.EncodingContext,
	)
	if berr != nil {
		err = berr
		return
	}
	if bytecodeBuilder == nil {
		err = fmt.Errorf("executor bytecode builder is required")
		return
	}

	bytecode, err := bytecodeBuilder.BuildBytecode(ExecutorBytecodeBuildInput{
		ExecutorType: input.ExecutorType,
		Context:      deps.EncodingContext,
		RoutePlan:    validated.routePlan,
		ResolvedLegs: validated.resolvedLegs,
		Sender:       input.UserAddress,
		SrcToken:     input.SrcToken,
		DestToken:    input.DestToken,
		DestAmount:   input.DestAmount,
		WethPlan:     validated.wethPlan,
	})
	if err != nil {
		return
	}

	params, err := BuildGenericSwapParams(input, validated.fee, string(bytecode))
	if err != nil {
		return
	}

	data, err := encodeGenericCalldata(input, validated.fee, bytecode, deps.AugustusV6ABI)
	if err != nil {
		return
	}

	value, err := BuildTxValue(input)
	if err != nil {
		return
	}

	txObject := TxObject{
		From:                 input.UserAddress,
		To:                   input.AugustusV6Address,
		Value:                DecimalString(value),
		Data:                 data,
		ChainID:              input.Network,
		GasPrice:             "",
		MaxFeePerGas:         "",
		MaxPriorityFeePerGas: "",
	}
	if input.Gas != nil {
		txObject.GasPrice = input.Gas.GasPrice
		txObject.MaxFeePerGas = input.Gas.MaxFeePerGas
		txObject.MaxPriorityFeePerGas = input.Gas.MaxPriorityFeePerGas
	}

	out.Params = params
	out.TxObject = txObject
	return
}
