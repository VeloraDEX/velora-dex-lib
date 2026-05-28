# Direct Method Builder Plan

## Goal

Add support for DEXes with Augustus V6 direct methods by exposing a public
direct builder path that calls DEX-specific `GetDirectParamV6` implementations,
similar to how the generic builder calls `GetDexParam`.

The current Go implementation can ABI-encode direct calldata from already
resolved params via `resolved.BuildDirectTransactionFromResolved`, but it does
not yet derive those params from route data.

## TypeScript Reference

Reference implementation:

`paraswap-dex-lib/src/generic-swap-transaction-builder.ts`

Relevant flow:

- `build(...)` detects V6 direct contract methods.
- `_buildDirect(...)` validates direct route shape.
- `_buildDirect(...)` builds `DirectParamInput`.
- The direct encoder registry returns a DEX encoder supporting the method.
- The DEX encoder calls `getDirectParamV6(...)`.
- Returned params are passed into `buildDirectTransactionFromResolved(...)`.

## Current Go State

Existing support:

- `txbuilder/builder/direct_build.go`
  - Exposes `builder.BuildDirect`.
  - Validates direct route shape.
  - Builds `DirectParamInput` and calls `GetDirectParamV6`.
  - Passes returned params into `resolved.BuildDirectTransactionFromResolved`.
- `txbuilder/builder/types.go`
  - Defines `DirectParamInput`, `DirectParamResult`, `DirectDexEncoder`, and
    `DirectDexRegistry`.
- `txbuilder/dex/registry/registry.go`
  - Supports generic encoders, direct encoders, and DEXes that provide both.
  - Validates direct method support during direct lookup.
- `txbuilder/resolved/direct_build.go`
  - Builds final direct transaction output from `DirectBuildInput`.
- `txbuilder/resolved/direct_calldata.go`
  - ABI-packs supplied direct params against the Augustus V6 ABI method.
- `txbuilder/resolved/direct_validation.go`
  - Whitelists supported direct methods and validates side consistency.
- `txbuilder/resolved/direct_ts_parity_test.go`
  - Reads the TypeScript direct resolved-build fixtures from
    `paraswap-dex-lib/tests/generic-swap-transaction-builder/fixtures/resolved-build/direct`.
  - Verifies Go `resolved.BuildDirectTransactionFromResolved` matches TS
    expected params, calldata, transaction target/source/value, and gas fields
    for every recorded direct method fixture.
- `txbuilder/builder/direct_build_test.go`
  - Reuses the same TS direct fixtures to verify builder orchestration,
    `DirectParamInput` construction, `GetDirectParamV6` wiring, and final
    direct transaction output.
- `txbuilder/dex/registry/registry_test.go`
  - Covers direct/generic partial support and duplicate-key behavior.

Deferred support:

- No unified `Build(...)` dispatcher that auto-selects generic versus direct.
  `BuildGeneric` and `BuildDirect` are currently explicit APIs.

## Proposed Design

### 1. Add Direct Encoder Contracts

Implemented in `txbuilder/builder/types.go`.

```go
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

type DirectDexEncoder interface {
	DirectContractMethodsV6() []string
	GetDirectParamV6(ctx context.Context, input DirectParamInput) (DirectParamResult, error)
}

type DirectDexRegistry interface {
	GetDirectDexEncoder(
		ctx context.Context,
		network int,
		dexKey string,
		contractMethod string,
	) (DirectDexEncoder, error)
}
```

Use `json.RawMessage` for direct params so DEX-specific tuple and array shapes
can pass through without forcing method-specific Go structs into the shared
builder package.

### 2. Extend Builder Dependencies

Implemented by adding a separate direct registry dependency:

```go
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
```

Keep the existing generic path unchanged:

- `BuildGeneric` should continue requiring `DexRegistry`.
- `BuildDirect` should require `DirectDexRegistry`.
- `BuildDirect` should ignore `ApprovalChecker`; direct approval behavior is
  DEX-owned and encoded through `GetDirectParamV6` params, matching the TS
  direct path.
- Existing users should not need a direct registry unless they call direct
  builder APIs.

### 3. Extend the DEX Registry Package

Implemented in `txbuilder/dex/registry`.

Proposed `Entry` shape:

```go
type Entry struct {
	Keys          []string
	Encoder       builder.DexEncoder
	DirectEncoder builder.DirectDexEncoder
}
```

Internally keep separate maps:

```go
encoders       map[string]builder.DexEncoder
directEncoders map[string]builder.DirectDexEncoder
```

Implement `GetDirectDexEncoder(...)` with checks equivalent to the TS adapter:

- registry is non-nil
- DEX key exists
- `contractMethod` is globally supported by `resolved.IsDirectContractMethod`
- direct encoder advertises the requested method in `DirectContractMethodsV6()`

The current `GetDexEncoder(...)` behavior should remain unchanged for generic
routes.

Partial support semantics:

- A registry entry may provide only `Encoder`, only `DirectEncoder`, or both.
- Generic lookup for a key with only `DirectEncoder` should return a clear
  generic encoder not found error.
- Direct lookup for a key with only `Encoder` should return a clear direct
  encoder not found error.
- Duplicate keys are rejected per capability map. A key may appear once for
  generic support and once for direct support, including in separate entries.
  Prefer registering both capabilities in a single `Entry` when a DEX supports
  both.
- Registering the same generic key twice is an error.
- Registering the same direct key twice is an error.

Example registration:

```go
dexRegistry := registry.MustNew(
	registry.Entry{
		Keys:    []string{"UniswapV2"},
		Encoder: uniswapV2GenericEncoder,
	},
	registry.Entry{
		Keys:          []string{"GenericRFQ"},
		DirectEncoder: genericRFQDirectEncoder,
	},
	registry.Entry{
		Keys:          []string{"BalancerV2"},
		Encoder:       balancerV2GenericEncoder,
		DirectEncoder: balancerV2DirectEncoder,
	},
)
```

### 4. Add `builder.BuildDirect`

Implemented:

```go
func BuildDirect(
	ctx context.Context,
	req BuildRequest,
	deps Deps,
) (resolved.DirectBuildOutput, error)
```

Behavior should mirror TS `_buildDirect`.

Validation and normalization:

- normalize request addresses and permit casing using existing helpers
- normalize encoding context
- validate `req.PriceRoute.Side` is `SELL` or `BUY`
- validate `req.PriceRoute.ContractMethod` with `resolved.IsDirectContractMethod`
- validate required dependencies:
  - `AugustusV6ABI`
  - `DirectDexRegistry`
  - `EncodingContext.Network`
  - `EncodingContext.AugustusV6Address`

Helper behavior should reuse the existing generic helper semantics:

- `resolveQuotedAmount`
  - if `req.QuotedAmount` is set and non-empty, use it
  - otherwise `SELL` falls back to `priceRoute.DestAmount`
  - otherwise `BUY` falls back to `priceRoute.SrcAmount`
- `resolvePermit`
  - if permit is unset or empty, use `"0x"`
  - otherwise normalize the hex string to lowercase
- `resolveBeneficiary`
  - if beneficiary is set, non-null, and different from user address, use the
    normalized beneficiary
  - otherwise use `resolved.NullAddress`

Direct route shape:

- `bestRoute` length must be `1`
- first route percent must be `100`
- first route must contain exactly one swap
- the first swap must contain at least one swap exchange because the direct
  DEX key and data are read from `swapExchanges[0]`
- for non-`swapOnAugustusRFQTryBatchFill`:
  - the swap must contain exactly one swap exchange
  - the swap exchange percent must be `100`
- for `swapOnAugustusRFQTryBatchFill`:
  - multiple swap exchanges are allowed
  - swap exchange percent is not enforced
  - the first swap exchange still supplies `dexKey` and `data`, matching TS
    `_buildDirect`
- the first swap exchange must have a non-empty exchange key

Amount handling:

```go
srcAmount := swapExchange.SrcAmount
destAmount := req.MinMaxAmount

if req.PriceRoute.Side == resolved.SideBuy {
	srcAmount = req.MinMaxAmount
	destAmount = swapExchange.DestAmount
}
```

`MinMaxAmount` side semantics:

- `SELL`: minimum destination amount
- `BUY`: maximum source amount

Fee handling:

- Build `resolved.FeeInput` from existing public `BuildRequest` fee fields:
  - `ReferrerAddress`
  - `PartnerAddress`
  - `PartnerFeePercent`
  - `TakeSurplus`
  - `IsCapSurplus`
  - `IsSurplusToUser`
  - `IsDirectFeeTransfer`
- Use existing `resolved.BuildFeesV6`.
- Convert the resulting `*big.Int` to decimal string for `DirectParamInput.PartnerAndFee`.

Direct param input:

```go
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
	Beneficiary:    resolveBeneficiary(req.UserAddress, req.Beneficiary),
	BlockNumber:    req.PriceRoute.BlockNumber,
}
```

Then:

- lookup direct encoder via `deps.DirectDexRegistry.GetDirectDexEncoder(...)`
- call `GetDirectParamV6(ctx, directParamInput)`
- build `resolved.DirectBuildInput`
- call `resolved.BuildDirectTransactionFromResolved(...)`

Error convention:

- Follow the existing generic builder style: return ordinary `fmt.Errorf`
  errors with contextual messages.
- Do not introduce typed or sentinel errors for the first direct-builder
  implementation.
- Wrap lower-level errors only when adding useful call-site context.

### 5. Optional Unified Dispatch API

After explicit `BuildDirect` support is in place, consider a separate
dispatcher API.

Dispatch rule:

```go
if resolved.IsDirectContractMethod(req.PriceRoute.ContractMethod) {
	return BuildDirect(ctx, req, deps)
}
return BuildGeneric(ctx, req, deps)
```

This should be a follow-up step, not part of the first direct-builder change, so
existing `BuildGeneric` behavior remains isolated and easy to validate.

Do not define the dispatcher return type in this phase. Decide later whether the
unified API should return a shared output struct, a generic-compatible shape, or
a small sum type over `resolved.BuildOutput` and `resolved.DirectBuildOutput`.

## Test Plan

### Existing Resolved-Layer Parity Coverage

The resolved direct encoder is already covered against TS-generated fixtures by
`txbuilder/resolved/direct_ts_parity_test.go`.

Fixture source:

```text
paraswap-dex-lib/tests/generic-swap-transaction-builder/fixtures/resolved-build/direct
```

Covered fixtures:

- `augustus-rfq-try-batch-fill.json`
- `balancer-v2-buy.json`
- `balancer-v2-sell.json`
- `curve-v1-sell.json`
- `curve-v2-sell.json`
- `lite-psm.json`
- `uniswap-v2-buy.json`
- `uniswap-v2-sell.json`
- `uniswap-v3-buy.json`
- `uniswap-v3-sell.json`

Current assertions:

- expected direct params match Go output params
- expected calldata matches `TxObject.Data`
- expected `from`, `to`, and `value` match
- expected gas fields match

Verification command:

```bash
go test ./txbuilder/resolved
go test ./...
```

### Builder-Layer Coverage Added

Unit tests cover:

- `BuildDirect` rejects unsupported direct methods.
- `BuildDirect` rejects invalid direct route shapes.
- `BuildDirect` errors when direct registry is missing.
- registry direct lookup rejects missing DEX key.
- registry direct lookup rejects globally unsupported direct method.
- registry direct lookup rejects a method not advertised by the DEX encoder.
- `DirectParamInput` is passed with:
  - normalized addresses
  - SELL amount mapping
  - BUY amount mapping
  - resolved quoted amount
  - resolved permit
  - resolved beneficiary
  - packed `partnerAndFee`
  - UUID
  - block number
  - raw swap exchange `data`
- returned direct params are passed unchanged into
  `resolved.BuildDirectTransactionFromResolved`.
- final calldata matches the TS direct fixture outputs by reusing fixture
  `input`, `expectedParams`, and `expectedTx`.
- existing `BuildGeneric` tests pass without a direct registry.

Use each fixture's `orchestration` block to drive `builder.BuildDirect` tests:

- `orchestration.priceRoute` should become `BuildRequest.PriceRoute`
- `orchestration.minMaxAmount` should become `BuildRequest.MinMaxAmount`
- `orchestration.quotedAmount` should become `BuildRequest.QuotedAmount`
- `orchestration.directDexKey` identifies the direct registry lookup key
- `expectedParams` should be returned by a fixture-backed fake
  `DirectDexEncoder.GetDirectParamV6`
- `expectedTx` should be compared to the final `resolved.DirectBuildOutput`

The fixture-backed fake direct encoder also captures
`DirectParamInput`, so tests can assert the Go builder matches TS input
construction before calldata encoding.

## Documentation Updates

Update:

- `README.md`
- `docs/TXBUILDER_USAGE.md`

Document:

- `builder.BuildDirect`
- `DirectDexEncoder`
- `GetDirectParamV6`
- direct registry wiring
- how generic and direct paths differ
- that direct params are DEX-owned and ABI-packed by the resolved layer

## Acceptance Criteria

- Consumers can register DEXes with direct method support.
- `builder.BuildDirect` calls the correct DEX `GetDirectParamV6`.
- Direct params are encoded into final Augustus V6 calldata.
- Existing generic builder behavior remains unchanged.
- Tests cover route validation, registry validation, direct param input
  construction, and final resolved calldata output.
