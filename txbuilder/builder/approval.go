package builder

import (
	"fmt"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

type routedApprovalRequest struct {
	routePositionKey string
	request          ApprovalRequest
	// isFallback routes the decision to the leg's FallbackParam instead of
	// the primary param. A fallback branch approves independently: its
	// token/target can differ from the primary's, and only one branch runs.
	isFallback bool
}

func buildDexExchangeApprovalRequests(
	context resolved.EncodingContext,
	priceRoute PriceRoute,
	routePlan resolved.RoutePlan,
	resolvedLegs []resolved.ResolvedLeg,
) ([]routedApprovalRequest, error) {
	legByKey := make(map[string]resolved.ResolvedLeg, len(resolvedLegs))
	for _, leg := range resolvedLegs {
		legByKey[resolved.ResolvedLegRoutePositionKey(leg)] = leg
	}

	routePositions := resolved.WalkRoutePlan(routePlan)
	requests := make([]routedApprovalRequest, 0)
	for _, routePosition := range routePositions {
		key := resolved.RoutePlanExchangeKey(routePosition)
		leg, ok := legByKey[key]
		if !ok {
			return nil, fmt.Errorf("missing resolved leg for route position %s", key)
		}
		swap := priceRoute.BestRoute[routePosition.RouteIndex].Swaps[routePosition.SwapIndex]
		if approval := getApprovalTokenAndTarget(swap, leg.ExchangeParam, context); approval != nil {
			requests = append(requests, routedApprovalRequest{
				routePositionKey: key,
				request: ApprovalRequest{
					Token:   approval.token,
					Target:  approval.target,
					Permit2: leg.ExchangeParam.Permit2Approval != nil && *leg.ExchangeParam.Permit2Approval,
				},
			})
		}
		if fallbackParam := leg.ExchangeParam.FallbackParam; fallbackParam != nil {
			if approval := getApprovalTokenAndTarget(swap, *fallbackParam, context); approval != nil {
				requests = append(requests, routedApprovalRequest{
					routePositionKey: key,
					isFallback:       true,
					request: ApprovalRequest{
						Token:   approval.token,
						Target:  approval.target,
						Permit2: fallbackParam.Permit2Approval != nil && *fallbackParam.Permit2Approval,
					},
				})
			}
		}
	}

	return requests, nil
}

type approvalTokenAndTarget struct {
	token  resolved.Address
	target resolved.Address
}

func getApprovalTokenAndTarget(
	swap PriceRouteSwap,
	exchangeParam resolved.DexExchangeBuildParam,
	context resolved.EncodingContext,
) *approvalTokenAndTarget {
	if exchangeParam.SkipApproval != nil && *exchangeParam.SkipApproval {
		return nil
	}

	target := exchangeParam.TargetExchange
	if exchangeParam.Spender != nil {
		target = *exchangeParam.Spender
	}

	if exchangeParam.NeedUnwrapNative != nil &&
		*exchangeParam.NeedUnwrapNative &&
		isWrappedNativeAddress(swap.SrcToken, context) {
		return nil
	}

	if !isNativeAddress(swap.SrcToken) && exchangeParam.TransferSrcTokenBeforeSwap == nil {
		return &approvalTokenAndTarget{
			token:  normalizeAddress(swap.SrcToken),
			target: normalizeAddress(target),
		}
	}

	if exchangeParam.NeedWrapNative.Value && isNativeAddress(swap.SrcToken) {
		token := context.WrappedNativeTokenAddress
		if exchangeParam.WethAddress != nil {
			token = *exchangeParam.WethAddress
		}
		return &approvalTokenAndTarget{
			token:  normalizeAddress(token),
			target: normalizeAddress(target),
		}
	}

	return nil
}

func applyDexExchangeApprovalDecisions(
	resolvedLegs []resolved.ResolvedLeg,
	approvalRequests []routedApprovalRequest,
	approvalDecisions []bool,
) ([]resolved.ResolvedLeg, error) {
	if len(approvalDecisions) != len(approvalRequests) {
		return nil, fmt.Errorf("approval decision length must match approval request count")
	}

	legByKey := make(map[string]resolved.ResolvedLeg, len(resolvedLegs))
	for _, leg := range resolvedLegs {
		leg.ExchangeParam.Spender = nil
		if leg.ExchangeParam.FallbackParam != nil {
			// Clone: the pointer is shared with the caller's slice, and the
			// fallback gets the same post-planning treatment as a primary.
			fallbackParam := *leg.ExchangeParam.FallbackParam
			fallbackParam.Spender = nil
			leg.ExchangeParam.FallbackParam = &fallbackParam
		}
		legByKey[resolved.ResolvedLegRoutePositionKey(leg)] = leg
	}

	for index, alreadyApproved := range approvalDecisions {
		request := approvalRequests[index]
		leg, ok := legByKey[request.routePositionKey]
		if !ok {
			return nil, fmt.Errorf("missing resolved leg for route position %s", request.routePositionKey)
		}
		if alreadyApproved {
			legByKey[request.routePositionKey] = leg
			continue
		}
		approveData := &resolved.ApproveData{
			Token:  normalizeAddress(request.request.Token),
			Target: normalizeAddress(request.request.Target),
		}
		if request.isFallback {
			if leg.ExchangeParam.FallbackParam == nil {
				return nil, fmt.Errorf("missing fallback param for route position %s", request.routePositionKey)
			}
			leg.ExchangeParam.FallbackParam.ApproveData = approveData
		} else {
			leg.ExchangeParam.ApproveData = approveData
		}
		legByKey[request.routePositionKey] = leg
	}

	out := make([]resolved.ResolvedLeg, len(resolvedLegs))
	for index, leg := range resolvedLegs {
		key := resolved.ResolvedLegRoutePositionKey(leg)
		updated, ok := legByKey[key]
		if !ok {
			return nil, fmt.Errorf("missing resolved leg for route position %s", key)
		}
		out[index] = updated
	}

	return out, nil
}
