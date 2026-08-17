package builder

import (
	"context"
	"encoding/json"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
)

// BuildGeneric lands in Phase 2 with this public shape:
// func BuildGeneric(ctx context.Context, req BuildRequest, deps Deps) (resolved.BuildOutput, error)

type BuildRequest struct {
	PriceRoute           PriceRoute              `json:"priceRoute"`
	MinMaxAmount         resolved.DecimalString  `json:"minMaxAmount"`
	QuotedAmount         *resolved.DecimalString `json:"quotedAmount,omitempty"`
	UserAddress          resolved.Address        `json:"userAddress"`
	ReferrerAddress      *resolved.Address       `json:"referrerAddress,omitempty"`
	PartnerAddress       resolved.Address        `json:"partnerAddress"`
	PartnerFeePercent    resolved.DecimalString  `json:"partnerFeePercent"`
	TakeSurplus          bool                    `json:"takeSurplus"`
	IsCapSurplus         *bool                   `json:"isCapSurplus,omitempty"`
	IsSurplusToUser      bool                    `json:"isSurplusToUser"`
	IsDirectFeeTransfer  bool                    `json:"isDirectFeeTransfer"`
	GasPrice             *resolved.DecimalString `json:"gasPrice,omitempty"`
	MaxFeePerGas         *resolved.DecimalString `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas *resolved.DecimalString `json:"maxPriorityFeePerGas,omitempty"`
	Permit               *resolved.HexBytes      `json:"permit,omitempty"`
	Deadline             resolved.DecimalString  `json:"deadline"`
	UUID                 string                  `json:"uuid"`
	Beneficiary          *resolved.Address       `json:"beneficiary,omitempty"`
	GetDexParamOptions   *GetDexParamOptions     `json:"getDexParamOptions,omitempty"`
}

// PriceRoute mirrors the priceRoute document /quote issues. Everything below
// BestRoute is declared for fidelity rather than for encoding: BuildGeneric
// reads none of it, but a BuildRequest is persisted verbatim as the
// post-mortem record of a build, and an undeclared field is a field that
// record loses. `others` stays undeclared on purpose — it is the bulkiest part
// of a price route, it has no bearing on what was encoded, and dropping it at
// the type is what already satisfies the "clear others before build" rule.
type PriceRoute struct {
	Network        int                    `json:"network"`
	BlockNumber    int64                  `json:"blockNumber"`
	ContractMethod string                 `json:"contractMethod"`
	Side           resolved.Side          `json:"side"`
	SrcToken       resolved.Address       `json:"srcToken"`
	DestToken      resolved.Address       `json:"destToken"`
	SrcAmount      resolved.DecimalString `json:"srcAmount"`
	DestAmount     resolved.DecimalString `json:"destAmount"`
	SrcUSD         *string                `json:"srcUSD,omitempty"`
	DestUSD        *string                `json:"destUSD,omitempty"`
	BestRoute      []PriceRouteRoute      `json:"bestRoute"`

	// Pointers throughout: a zero is meaningful for every one of these
	// (0-decimal tokens exist, partnerFee is 0 on most trades, and
	// maxImpactReached is usually false), so a value type with omitempty
	// would record "absent" for the common case.
	SrcDecimals        *int                    `json:"srcDecimals,omitempty"`
	DestDecimals       *int                    `json:"destDecimals,omitempty"`
	DestAmountAfterFee *resolved.DecimalString `json:"destAmountAfterFee,omitempty"`
	GasCost            *resolved.DecimalString `json:"gasCost,omitempty"`
	GasCostUSD         *string                 `json:"gasCostUSD,omitempty"`
	Version            *string                 `json:"version,omitempty"`
	ContractAddress    *resolved.Address       `json:"contractAddress,omitempty"`
	TokenTransferProxy *resolved.Address       `json:"tokenTransferProxy,omitempty"`
	Partner            *string                 `json:"partner,omitempty"`
	PartnerFee         *float64                `json:"partnerFee,omitempty"`
	MaxImpactReached   *bool                   `json:"maxImpactReached,omitempty"`

	// Hmac survives a round the other fields do not: the API re-decodes this
	// struct from the canonical bytes a signature was verified over, and
	// verification strips the signature to reproduce them, so it is carried
	// over from the parse of the original body instead.
	Hmac *string `json:"hmac,omitempty"`
}

type PriceRouteRoute struct {
	Percent float64          `json:"percent"`
	Swaps   []PriceRouteSwap `json:"swaps"`
}

type PriceRouteSwap struct {
	SrcToken      resolved.Address         `json:"srcToken"`
	DestToken     resolved.Address         `json:"destToken"`
	SrcAmount     *resolved.DecimalString  `json:"srcAmount,omitempty"`
	DestAmount    *resolved.DecimalString  `json:"destAmount,omitempty"`
	SwapExchanges []PriceRouteSwapExchange `json:"swapExchanges"`

	SrcDecimals  *int `json:"srcDecimals,omitempty"`
	DestDecimals *int `json:"destDecimals,omitempty"`
}

type PriceRouteSwapExchange struct {
	Exchange   string                 `json:"exchange"`
	Percent    float64                `json:"percent"`
	SrcAmount  resolved.DecimalString `json:"srcAmount"`
	DestAmount resolved.DecimalString `json:"destAmount"`
	Data       json.RawMessage        `json:"data,omitempty"`

	// The dex-specific Data blob identifies the pool for some exchanges and not
	// others, so this is the only field that reliably answers "which pool did
	// this leg touch" in a post-mortem.
	PoolIdentifiers []string `json:"poolIdentifiers,omitempty"`
}

type Deps struct {
	EncodingContext   resolved.EncodingContext
	AugustusV6ABI     *ethabi.ABI
	ExecutorFactory   resolved.ExecutorBytecodeBuilderFactory
	DexRegistry       DexRegistry
	DirectDexRegistry DirectDexRegistry
	ApprovalChecker   ApprovalChecker
	WethProvider      WethCallDataProvider
	Options           Options
}

type Options struct {
	SkipApprovalCheck bool
}

type GetDexParamOptions struct {
	NowTimestampMs *uint64 `json:"nowTimestampMs,omitempty"`
}

type DexRegistry interface {
	GetDexEncoder(ctx context.Context, network int, dexKey string) (DexEncoder, error)
}

type DexEncoder interface {
	NeedWrapNative(ctx context.Context, input NeedWrapNativeInput) (bool, error)
	GetDexParam(ctx context.Context, input DexParamInput) (DexExchangeParam, error)
}

type DirectDexRegistry interface {
	GetDirectDexEncoder(
		ctx context.Context,
		network int,
		dexKey string,
		contractMethod string,
	) (DirectDexEncoder, error)
}

type DirectDexEncoder interface {
	DirectContractMethodsV6() []string
	GetDirectParamV6(ctx context.Context, input DirectParamInput) (DirectParamResult, error)
}

type ApprovalRequest struct {
	RoutePositionKey string           `json:"routePositionKey,omitempty"`
	Token            resolved.Address `json:"token"`
	Target           resolved.Address `json:"target"`
	Permit2          bool             `json:"permit2"`
}

type ApprovalChecker interface {
	Check(ctx context.Context, spender resolved.Address, requests []ApprovalRequest) ([]bool, error)
}

type WethCallDataInput struct {
	SrcAmountWeth  resolved.DecimalString `json:"srcAmountWeth"`
	DestAmountWeth resolved.DecimalString `json:"destAmountWeth"`
	Side           resolved.Side          `json:"side"`
}

type WethCallDataProvider interface {
	GetDepositWithdrawCallData(ctx context.Context, input WethCallDataInput) (*resolved.WethPlan, error)
}

type NeedWrapNativeInput struct {
	Route        NeedWrapNativeRouteContext        `json:"route"`
	Swap         NeedWrapNativeSwapContext         `json:"swap"`
	SwapExchange NeedWrapNativeSwapExchangeContext `json:"swapExchange"`
}

type NeedWrapNativeRouteContext struct {
	Network      int                    `json:"network"`
	Side         resolved.Side          `json:"side"`
	RouteIndex   int                    `json:"routeIndex"`
	RoutePercent float64                `json:"routePercent"`
	BlockNumber  int64                  `json:"blockNumber"`
	SrcToken     resolved.Address       `json:"srcToken"`
	DestToken    resolved.Address       `json:"destToken"`
	SrcAmount    resolved.DecimalString `json:"srcAmount"`
	DestAmount   resolved.DecimalString `json:"destAmount"`
}

type NeedWrapNativeSwapContext struct {
	SwapIndex  int                    `json:"swapIndex"`
	SrcToken   resolved.Address       `json:"srcToken"`
	DestToken  resolved.Address       `json:"destToken"`
	SrcAmount  resolved.DecimalString `json:"srcAmount"`
	DestAmount resolved.DecimalString `json:"destAmount"`
}

type NeedWrapNativeSwapExchangeContext struct {
	SwapExchangeIndex int                    `json:"swapExchangeIndex"`
	Exchange          string                 `json:"exchange"`
	Percent           float64                `json:"percent"`
	SrcAmount         resolved.DecimalString `json:"srcAmount"`
	DestAmount        resolved.DecimalString `json:"destAmount"`
	Data              json.RawMessage        `json:"data,omitempty"`
}

type DexParamInput struct {
	NeedWrapNativeInput
	DexKey          string                 `json:"dexKey"`
	SrcToken        resolved.Address       `json:"srcToken"`
	DestToken       resolved.Address       `json:"destToken"`
	SrcAmount       resolved.DecimalString `json:"srcAmount"`
	DestAmount      resolved.DecimalString `json:"destAmount"`
	Recipient       resolved.Address       `json:"recipient"`
	ExecutorAddress resolved.Address       `json:"executorAddress"`
	Side            resolved.Side          `json:"side"`
	Data            json.RawMessage        `json:"data,omitempty"`
	Options         *GetDexParamOptions    `json:"options,omitempty"`
}

type DirectParamInput struct {
	DexKey         string                 `json:"dexKey"`
	Network        int                    `json:"network"`
	ContractMethod string                 `json:"contractMethod"`
	SrcToken       resolved.Address       `json:"srcToken"`
	DestToken      resolved.Address       `json:"destToken"`
	SrcAmount      resolved.DecimalString `json:"srcAmount"`
	DestAmount     resolved.DecimalString `json:"destAmount"`
	QuotedAmount   resolved.DecimalString `json:"quotedAmount"`
	Data           json.RawMessage        `json:"data,omitempty"`
	Side           resolved.Side          `json:"side"`
	Permit         resolved.HexBytes      `json:"permit"`
	UUID           string                 `json:"uuid"`
	PartnerAndFee  resolved.DecimalString `json:"partnerAndFee"`
	Beneficiary    resolved.Address       `json:"beneficiary"`
	BlockNumber    int64                  `json:"blockNumber"`
}

type DirectParamResult struct {
	Params json.RawMessage `json:"params"`
}

type DexExchangeParam struct {
	NeedWrapNative                        bool              `json:"needWrapNative"`
	NeedUnwrapNative                      *bool             `json:"needUnwrapNative,omitempty"`
	SkipApproval                          *bool             `json:"skipApproval,omitempty"`
	WethAddress                           *resolved.Address `json:"wethAddress,omitempty"`
	ExchangeData                          resolved.HexBytes `json:"exchangeData"`
	TargetExchange                        resolved.Address  `json:"targetExchange"`
	DexFuncHasRecipient                   bool              `json:"dexFuncHasRecipient"`
	SpecialDexFlag                        *int              `json:"specialDexFlag,omitempty"`
	TransferSrcTokenBeforeSwap            *resolved.Address `json:"transferSrcTokenBeforeSwap,omitempty"`
	Spender                               *resolved.Address `json:"spender,omitempty"`
	SendEthButSupportsInsertFromAmount    *bool             `json:"sendEthButSupportsInsertFromAmount,omitempty"`
	SpecialDexSupportsInsertFromAmount    *bool             `json:"specialDexSupportsInsertFromAmount,omitempty"`
	SwappedAmountNotPresentInExchangeData *bool             `json:"swappedAmountNotPresentInExchangeData,omitempty"`
	ReturnAmountPos                       *int              `json:"returnAmountPos,omitempty"`
	InsertFromAmountPos                   *int              `json:"insertFromAmountPos,omitempty"`
	AmountsPacked128                      *bool             `json:"amountsPacked128,omitempty"`
	Permit2Approval                       *bool             `json:"permit2Approval,omitempty"`
}
