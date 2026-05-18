# Go Generic Swap Builder Usage

This document describes how to construct and call the Go version of the
`GenericSwapTransactionBuilder` for generic V6 routes.

## Entry Point

Use:

```go
output, err := builder.BuildGeneric(ctx, req, deps)
```

`BuildGeneric` converts a public `builder.BuildRequest` into a resolved
generic build input, asks DEX encoders for per-leg calldata, inserts approval
data when needed, builds executor bytecode, ABI-encodes Augustus V6 calldata,
and returns:

```go
type resolved.BuildOutput struct {
    Params   []any
    TxObject resolved.TxObject
}
```

`TxObject.Data` is the final transaction calldata.

## Build Request

`builder.BuildRequest` is the public input. It mirrors the TypeScript
`GenericSwapTransactionBuilder.build({ ... })` generic-route path:

- `PriceRoute`: route, side, method, tokens, amounts, and `bestRoute`.
- `MinMaxAmount`: min received for SELL, max spent for BUY.
- `QuotedAmount`: optional; defaults from the route when omitted.
- `UserAddress`, `PartnerAddress`, `ReferrerAddress`, fee flags.
- Optional gas fields, `Permit`, `UUID`, and `Beneficiary`.
- `Deadline` is accepted at the boundary but is not encoded in V6 generic
  calldata.

All addresses should be `0x`-prefixed 20-byte hex strings. The builder
normalizes public input to lowercase before calling the resolved layer.

Supported generic methods are:

- `swapExactAmountIn`
- `swapExactAmountOut`
- `swapExactAmountInPro`
- `swapExactAmountOutPro`

## Required Dependencies

`builder.Deps` wires the builder to the runtime environment:

```go
deps := builder.Deps{
    EncodingContext: resolved.EncodingContext{
        Network:                   network,
        AugustusV6Address:         augustus,
        WrappedNativeTokenAddress: wrappedNative,
        ExecutorsAddresses: map[resolved.ExecutorType]resolved.Address{
            resolved.Executor01:   executor01,
            resolved.Executor02:   executor02,
            resolved.Executor03:   executor03,
            resolved.ExecutorWETH: wrappedNative,
        },
    },
    AugustusV6ABI:   resolved.MustLoadAugustusV6ABI(),
    ExecutorFactory: executor.NewFactory(),
    DexRegistry:     dexRegistry,
    ApprovalChecker: approvalChecker,
    WethProvider:    wethProvider,
}
```

`EncodingContext` must match the request network and executor addresses. For
`ExecutorWETH`, use the wrapped-native address.

## DEX Registry And Encoders

The builder is DEX-agnostic. It calls a registry by the route's
`swapExchange.exchange` label:

```go
type DexRegistry interface {
    GetDexEncoder(ctx context.Context, network int, dexKey string) (builder.DexEncoder, error)
}
```

Each encoder implements:

```go
type DexEncoder interface {
    NeedWrapNative(ctx context.Context, input builder.NeedWrapNativeInput) (bool, error)
    GetDexParam(ctx context.Context, input builder.DexParamInput) (builder.DexExchangeParam, error)
}
```

`NeedWrapNative` decides whether native tokens should be wrapped before this
leg. `GetDexParam` returns the DEX calldata and executor metadata for that leg.

For simple alias-based registration:

```go
tesseraEncoder := tessera.New(tessera.DefaultConfig())
dexRegistry := registry.MustNew(registry.Entry{
    Keys:    []string{"tessera", "Tessera"},
    Encoder: tesseraEncoder,
})
```

The registry performs exact key matching. Register every route label or alias
you expect to accept.

## Approval Checker

Approvals are supplied through:

```go
type ApprovalChecker interface {
    Check(ctx context.Context, spender resolved.Address, requests []builder.ApprovalRequest) ([]bool, error)
}
```

`spender` is the selected executor address. Return one boolean per request:

- `true`: already approved, no approve calldata is inserted.
- `false`: not approved, the builder inserts `approveData` for that leg.

For tests or trusted pre-approved routes, set:

```go
deps.Options.SkipApprovalCheck = true
```

When `SkipApprovalCheck` is true, the builder skips the checker and treats all
approval requests as missing.

## WETH Provider

Routes that require aggregate WETH deposit or withdraw calldata use:

```go
type WethCallDataProvider interface {
    GetDepositWithdrawCallData(ctx context.Context, input builder.WethCallDataInput) (*resolved.WethPlan, error)
}
```

If your supported routes do not require WETH plan calldata, this can be nil.
For native/wrapped multi-leg routes, provide a network-correct implementation.

## Runtime Dependencies

The consuming service must provide:

- network-specific `resolved.EncodingContext`.
- a `builder.DexRegistry` containing all DEX route labels the service accepts.
- a production `builder.ApprovalChecker`, typically backed by existing
  allowance state, Redis, or RPC calls.
- a `builder.WethCallDataProvider` for routes that need aggregate WETH deposit
  or withdraw calldata.

## Notes

- DEX registry keys are exact-match aliases. Register both canonical keys and
  public route labels when they differ.
- `Options.SkipApprovalCheck` is intended for tests or trusted pre-approved
  routes. In production, prefer a real `ApprovalChecker`.
- TypeScript parity fixture generation remains in the source
  `paraswap-dex-lib` repository.

