package builder

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

// versionV6 is the only ParaSwapVersion this builder produces. The receiving
// schema accepts "5" or "6.2" and dex-lib reads the value only to gate a V6
// approval branch, so forwarding a route's own `version` would let a V5 price
// route describe a V6 encoding.
const versionV6 = "6.2"

// slippageFactorDecimals mirrors BigNumber.js DECIMAL_PLACES, which is what
// legacy divides under (neither TS repo overrides the default).
const slippageFactorDecimals = 20

// buildGetDexParamPreProcess assembles the per-leg RFQ preprocess context.
//
// Every field is either the leg's own or route-level; nothing is derived from
// the values that reach the dex encoder, because those are already
// WETH-substituted and, on SELL, carry destAmount "1".
//
// Missing token decimals fail the build. They are optional on a price route, but
// the receiving schema requires them and they set the size of the RFQ request,
// so defaulting them to 0 would ask for a firm quote on the wrong amount. Every
// price route this API issues carries them (the /quote response declares them
// non-optional), which leaves /transactions/raw as the only surface that can
// omit them — and a loud failure is what a parity surface wants.
func buildGetDexParamPreProcess(
	req BuildRequest,
	swap PriceRouteSwap,
	swapExchange PriceRouteSwapExchange,
	callParams genericDexCallParams,
	executorAddress resolved.Address,
) (*GetDexParamPreProcess, error) {
	srcToken, err := preProcessToken(swap.SrcToken, swap.SrcDecimals, "srcDecimals")
	if err != nil {
		return nil, err
	}
	destToken, err := preProcessToken(swap.DestToken, swap.DestDecimals, "destDecimals")
	if err != nil {
		return nil, err
	}

	slippageFactor, err := resolveSlippageFactor(req)
	if err != nil {
		return nil, err
	}

	// A raw build request may omit txOrigin; the public endpoint defaults it in
	// MapToBuildRequest. Defaulting here too keeps the field non-empty, which
	// the receiving schema requires.
	txOrigin := req.TxOrigin
	if txOrigin == "" {
		txOrigin = req.UserAddress
	}

	partner := ""
	if req.PriceRoute.Partner != nil {
		partner = *req.PriceRoute.Partner
	}

	return &GetDexParamPreProcess{
		SrcToken:                 srcToken,
		DestToken:                destToken,
		SrcAmount:                swapExchange.SrcAmount,
		DestAmount:               swapExchange.DestAmount,
		SlippageFactor:           slippageFactor,
		TxOrigin:                 normalizeAddress(txOrigin),
		UserAddress:              normalizeAddress(req.UserAddress),
		ExecutionContractAddress: normalizeAddress(executorAddress),
		Recipient:                callParams.recipient,
		Version:                  versionV6,
		Partner:                  partner,
		Special:                  req.Special,
	}, nil
}

// withPreProcess returns a copy of the route-level options carrying this leg's
// preprocess context. A copy, not a mutation: req.GetDexParamOptions is one
// object shared by every leg of the build, so writing into it would leave every
// leg with the last leg's context.
func withPreProcess(options *GetDexParamOptions, preProcess *GetDexParamPreProcess) *GetDexParamOptions {
	clone := GetDexParamOptions{PreProcess: preProcess}
	if options != nil {
		clone.NowTimestampMs = options.NowTimestampMs
	}
	return &clone
}

// preProcessToken pairs a leg token with its decimals.
func preProcessToken(address resolved.Address, decimals *int, field string) (PreProcessToken, error) {
	if decimals == nil {
		return PreProcessToken{}, fmt.Errorf(
			"priceRoute swap %s is required to build the RFQ preprocess context", field,
		)
	}
	if *decimals < 0 {
		return PreProcessToken{}, fmt.Errorf(
			"priceRoute swap %s must not be negative, got %d", field, *decimals,
		)
	}
	return PreProcessToken{Address: normalizeAddress(address), Decimals: *decimals}, nil
}

// resolveSlippageFactor ports the legacy one-liner
// `new BigNumber(minMaxAmount).div(side === SELL ? destAmount : srcAmount)`.
func resolveSlippageFactor(req BuildRequest) (string, error) {
	minMaxAmount, err := parseDecimal(req.MinMaxAmount, "minMaxAmount")
	if err != nil {
		return "", err
	}

	denominatorField := "priceRoute.destAmount"
	denominatorValue := req.PriceRoute.DestAmount
	if req.PriceRoute.Side == resolved.SideBuy {
		denominatorField = "priceRoute.srcAmount"
		denominatorValue = req.PriceRoute.SrcAmount
	}
	denominator, err := parseDecimal(denominatorValue, denominatorField)
	if err != nil {
		return "", err
	}

	return slippageFactorString(minMaxAmount, denominator)
}

// slippageFactorString mirrors BigNumber.js division under its defaults: 20
// decimal places, ROUND_HALF_UP. Both operands are non-negative integers, so
// half-up and half-away-from-zero coincide.
func slippageFactorString(minMaxAmount, denominator *big.Int) (string, error) {
	if denominator.Sign() <= 0 {
		return "", fmt.Errorf("slippage factor denominator must be positive, got %s", denominator)
	}

	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(slippageFactorDecimals), nil)
	quotient, remainder := new(big.Int).QuoRem(
		new(big.Int).Mul(minMaxAmount, scale), denominator, new(big.Int),
	)
	if new(big.Int).Lsh(remainder, 1).Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}

	digits := quotient.String()
	if len(digits) <= slippageFactorDecimals {
		digits = strings.Repeat("0", slippageFactorDecimals-len(digits)+1) + digits
	}
	split := len(digits) - slippageFactorDecimals
	fraction := strings.TrimRight(digits[split:], "0")
	if fraction == "" {
		return digits[:split], nil
	}
	return digits[:split] + "." + fraction, nil
}
