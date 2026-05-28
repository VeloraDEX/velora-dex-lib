# velora-dex-lib

Go module for ParaSwap/Velora transaction-building and DEX encoding logic.

The module currently exposes the Go implementation of the generic and direct
V6 transaction-builder paths, executor bytecode builders, resolved transaction
encoding, a small DEX encoder registry, and Augustus approval checking helpers.

## Install

From the consuming service:

```bash
go get github.com/VeloraDEX/velora-dex-lib@v0.1.0
```

Use a version tag in production. During active development a branch or commit
SHA can be used temporarily. No GitHub authentication is required when this
repository is public and the consuming environment does not force this module
path through private-module settings such as `GOPRIVATE`.

## Main Packages

- `txbuilder/builder`: public generic and direct swap builder APIs.
- `txbuilder/approvals`: Augustus approval checker with TypeScript-compatible
  cache keys and Multicall3 allowance reads.
- `txbuilder/resolved`: resolved build input validation and Augustus V6
  calldata encoding.
- `txbuilder/executor`: Executor01/02/03/WETH bytecode builders.
- `txbuilder/dex/registry`: exact-key DEX encoder registry.

See [`docs/TXBUILDER_USAGE.md`](docs/TXBUILDER_USAGE.md) for detailed
construction, dependency wiring, and runtime integration notes.

## Basic Usage

```go
package example

import (
    "context"

    "github.com/VeloraDEX/velora-dex-lib/txbuilder/builder"
    "github.com/VeloraDEX/velora-dex-lib/txbuilder/executor"
    "github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

func Build(
    ctx context.Context,
    req builder.BuildRequest,
    dexRegistry builder.DexRegistry,
    approvalChecker builder.ApprovalChecker,
    wethProvider builder.WethCallDataProvider,
) (resolved.BuildOutput, error) {
    deps := builder.Deps{
        EncodingContext: resolved.EncodingContext{
            Network:                   8453,
            AugustusV6Address:         "0x...",
            WrappedNativeTokenAddress: "0x4200000000000000000000000000000000000006",
            ExecutorsAddresses: map[resolved.ExecutorType]resolved.Address{
                resolved.Executor01:   "0x...",
                resolved.Executor02:   "0x...",
                resolved.Executor03:   "0x...",
                resolved.ExecutorWETH: "0x4200000000000000000000000000000000000006",
            },
        },
        AugustusV6ABI:   resolved.MustLoadAugustusV6ABI(),
        ExecutorFactory: executor.NewFactory(),
        DexRegistry:     dexRegistry,
        ApprovalChecker: approvalChecker,
        WethProvider:    wethProvider,
    }

    return builder.BuildGeneric(ctx, req, deps)
}
```

`BuildGeneric` returns `resolved.BuildOutput`, whose `TxObject.Data` field is
the final Augustus V6 transaction calldata. For V6 direct methods, wire
`Deps.DirectDexRegistry` and call `builder.BuildDirect`, which returns
`resolved.DirectBuildOutput`.

## Runtime Dependencies

The consuming service must provide:

- network-specific `resolved.EncodingContext`.
- a `builder.DexRegistry` containing all DEX route labels the service accepts.
- a `builder.DirectDexRegistry` for direct methods when using
  `builder.BuildDirect`.
- a production `builder.ApprovalChecker`, typically backed by existing
  allowance state, Redis, or RPC calls. `txbuilder/approvals` provides the
  reusable Augustus checker; consuming services wire their Redis and RPC clients
  through its cache and contract-caller interfaces.
- a `builder.WethCallDataProvider` for routes that need aggregate WETH deposit
  or withdraw calldata.

## Notes

- DEX registry keys are exact-match aliases. Register both canonical keys and
  public route labels when they differ.
- `Options.SkipApprovalCheck` is intended for tests or trusted pre-approved
  routes. In production, prefer a real `ApprovalChecker`.
- This module is intended to be imported by Go services. TypeScript parity
  fixture generation remains in the source `paraswap-dex-lib` repository.

## License

This repository is licensed under GPL-3.0, matching the public
[`paraswap-dex-lib`](https://github.com/VeloraDEX/paraswap-dex-lib) repository.
