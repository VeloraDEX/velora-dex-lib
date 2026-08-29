package builder

import (
	"context"
	"fmt"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

func BuildDirect(ctx context.Context, req BuildRequest, deps Deps) (resolved.DirectBuildOutput, error) {
	if err := validateDirectDeps(deps); err != nil {
		return resolved.DirectBuildOutput{}, err
	}

	req = normalizeBuildRequest(req)
	encodingContext := normalizeEncodingContext(deps.EncodingContext)

	if req.PriceRoute.Side != resolved.SideSell && req.PriceRoute.Side != resolved.SideBuy {
		return resolved.DirectBuildOutput{}, fmt.Errorf("invalid side: %s", req.PriceRoute.Side)
	}
	if !resolved.IsDirectContractMethod(req.PriceRoute.ContractMethod) {
		return resolved.DirectBuildOutput{}, fmt.Errorf(
			"unsupported direct contract method for builder: %s",
			req.PriceRoute.ContractMethod,
		)
	}
	if req.PriceRoute.Network != encodingContext.Network {
		return resolved.DirectBuildOutput{}, fmt.Errorf(
			"network mismatch: input %d, context %d",
			req.PriceRoute.Network,
			encodingContext.Network,
		)
	}

	swapExchange, err := validateDirectRouteShape(req.PriceRoute)
	if err != nil {
		return resolved.DirectBuildOutput{}, err
	}

	dexKey := swapExchange.Exchange

	srcAmount := swapExchange.SrcAmount
	destAmount := req.MinMaxAmount
	if req.PriceRoute.Side == resolved.SideBuy {
		srcAmount = req.MinMaxAmount
		destAmount = swapExchange.DestAmount
	}

	partnerAndFee, err := resolved.BuildFeesV6(resolved.FeeInput{
		PartnerAddress:      req.PartnerAddress,
		PartnerFeePercent:   req.PartnerFeePercent,
		ReferrerAddress:     req.ReferrerAddress,
		TakeSurplus:         req.TakeSurplus,
		IsCapSurplus:        resolveIsCapSurplus(req.IsCapSurplus),
		IsSurplusToUser:     req.IsSurplusToUser,
		IsDirectFeeTransfer: req.IsDirectFeeTransfer,
	})
	if err != nil {
		return resolved.DirectBuildOutput{}, fmt.Errorf("build direct partnerAndFee: %w", err)
	}

	directParamInput := DirectParamInput{
		DexKey:         dexKey,
		Network:        req.PriceRoute.Network,
		ContractMethod: req.PriceRoute.ContractMethod,
		SrcToken:       req.PriceRoute.SrcToken,
		DestToken:      req.PriceRoute.DestToken,
		SrcAmount:      srcAmount,
		DestAmount:     destAmount,
		QuotedAmount:   resolveQuotedAmount(req.PriceRoute, req.QuotedAmount),
		Data:           swapExchange.Data,
		Side:           req.PriceRoute.Side,
		Permit:         resolvePermit(req.Permit),
		UUID:           req.UUID,
		PartnerAndFee:  resolved.DecimalString(partnerAndFee.String()),
		Beneficiary:    resolveBeneficiary(req.Beneficiary),
		BlockNumber:    req.PriceRoute.BlockNumber,
	}

	directDexEncoder, err := deps.DirectDexRegistry.GetDirectDexEncoder(
		ctx,
		directParamInput.Network,
		directParamInput.DexKey,
		directParamInput.ContractMethod,
	)
	if err != nil {
		return resolved.DirectBuildOutput{}, err
	}
	if directDexEncoder == nil {
		return resolved.DirectBuildOutput{}, fmt.Errorf("direct dex encoder is required for %s", dexKey)
	}

	directParamResult, err := directDexEncoder.GetDirectParamV6(ctx, directParamInput)
	if err != nil {
		return resolved.DirectBuildOutput{}, err
	}

	return resolved.BuildDirectTransactionFromResolved(
		resolved.DirectBuildInput{
			ContractMethod:    req.PriceRoute.ContractMethod,
			Params:            directParamResult.Params,
			UserAddress:       req.UserAddress,
			AugustusV6Address: encodingContext.AugustusV6Address,
			SrcToken:          req.PriceRoute.SrcToken,
			SrcAmount:         req.PriceRoute.SrcAmount,
			MinMaxAmount:      req.MinMaxAmount,
			Side:              req.PriceRoute.Side,
			Network:           req.PriceRoute.Network,
			Gas:               resolveGas(req),
		},
		resolved.DirectBuildDeps{
			AugustusV6ABI: deps.AugustusV6ABI,
		},
	)
}

func validateDirectDeps(deps Deps) error {
	if deps.AugustusV6ABI == nil {
		return fmt.Errorf("augustus V6 ABI is required")
	}
	if deps.DirectDexRegistry == nil {
		return fmt.Errorf("direct dex registry is required")
	}
	if deps.EncodingContext.Network == 0 {
		return fmt.Errorf("encoding context network is required")
	}
	if deps.EncodingContext.AugustusV6Address == "" {
		return fmt.Errorf("encoding context augustusV6Address is required")
	}
	return nil
}

func validateDirectRouteShape(priceRoute PriceRoute) (PriceRouteSwapExchange, error) {
	if len(priceRoute.BestRoute) != 1 ||
		priceRoute.BestRoute[0].Percent != 100 ||
		len(priceRoute.BestRoute[0].Swaps) != 1 {
		return PriceRouteSwapExchange{}, fmt.Errorf("DirectSwap invalid bestRoute")
	}

	swapExchanges := priceRoute.BestRoute[0].Swaps[0].SwapExchanges
	if len(swapExchanges) == 0 {
		return PriceRouteSwapExchange{}, fmt.Errorf("DirectSwap invalid bestRoute")
	}

	if priceRoute.ContractMethod != resolved.ContractMethodSwapOnAugustusRFQTryBatchFill &&
		(len(swapExchanges) != 1 || swapExchanges[0].Percent != 100) {
		return PriceRouteSwapExchange{}, fmt.Errorf("DirectSwap invalid bestRoute")
	}
	if swapExchanges[0].Exchange == "" {
		return PriceRouteSwapExchange{}, fmt.Errorf("direct dex key is required")
	}

	return swapExchanges[0], nil
}
