# Legacy Encoding Parity Gaps

**Audited:** 2026-07-29

**Go side:** `txbuilder/` (this repo)

**Legacy side:** `paraswap-dex-lib` at `b6699ba2f` — `src/executor/*`,
`src/generic-swap-transaction-builder.ts`, `src/types.ts`

Scope of the audit: features and `GetDexParam` / `DexExchangeParam` fields
consumed by the V6 encoding path. V5 routers and the adapter path are treated as
out of scope (see [Out of Scope](#7-out-of-scope)).

---

## 1. Not Ported At All: Revertable Fallback Groups

Tracked as [DEXLIB-203](https://linear.app/veloradex/issue/DEXLIB-203).

The largest gap. Legacy encodes `SpecialDex.REVERTABLE_FALLBACK_GROUP = 0xff`:
a step whose calldata payload is `[tryLen(4)][fallbackLen(4)][try-block][fallback-block]`.
Executor01 runs the try-block in a self-call and, on revert, runs the
fallback-block from the original pre-group input. Nothing in Go touches it.

| Legacy piece | Go status |
| --- | --- |
| `OptimalSwapExchange.fallback` (route input) | absent from `txbuilder/builder/types.go:64` `PriceRouteSwapExchange` |
| `DexExchangeBuildParam.fallbackParam` | absent from `txbuilder/resolved/input_types.go:50` |
| `DexExchangeParam.executorIsDestReceiver` | absent |
| `SpecialDex = 0xff` | absent from `txbuilder/executor/types.go:58-73` |
| `Executor01BytecodeBuilder.buildRevertableGroup` | absent |
| Executor02 equivalent (`Executor02BytecodeBuilder.ts:748,773`) | absent |
| Fallback wrap/unwrap folded into WETH-plan totals (`generic-swap-transaction-builder.ts:256-258`) | `txbuilder/builder/orchestration.go:216-220` sums primaries only |
| `buildFallbackBuildParam` — fallback gets its own approval check | absent |
| `ExecutorDetector.routeHasFallback` warning | absent from `txbuilder/builder/executor_detector.go` |

Implementing this requires, in order: the input-type field, the build-param
field, the `0xff` special flag, the group builder in Executor01/02, and the
WETH-plan accounting change.

---

## 2. Silent Divergences — ALL CLOSED (2026-07-29)

These produced wrong calldata; no validator gate caught them. All three were
fixed on 2026-07-29, fixtures in
`txbuilder/executor/executor_parity_fixes_test.go`.

### 2.1 Executor01/02 `fromAmountPos` never retried the negative `int256` encoding — CLOSED

Legacy routes all three builders through `findAmountPosWithFallback`
(`ExecutorBytecodeBuilder.ts:287-311`), which retries with
`abi.encode(['int256'], [-amount])` when the positive `uint256` encoding is not
found in the exchange data. Go had the fallback only in Executor03; on a miss
Executor01/02 inserted at `len(exchangeData)/2` — past the end of the calldata —
for any DEX that encodes its amount as a negative `int256` (Curve-family
`int256` args).

Fixed as suggested: `findAmountPosWithFallback` now lives in
`txbuilder/executor/base.go` and all three `buildDexCallData` paths call it.

### 2.2 `insertFromAmountPos == 0` truthiness divergence — CLOSED

Legacy tests truthiness (`if (exchangeParam.insertFromAmountPos)`) and falls
through to the calldata search when the value is `0`. Go now matches: a `0`
override falls through to the search in all three builders.

### 2.3 Executor03 exchange ordering comparator — CLOSED

Legacy uses `.sort(e => e.exchangeParam.needWrapNative ? 1 : -1)`
(`Executor03BytecodeBuilder.ts:457`) — a single-argument, non-transitive
comparator. Empirically (Node/V8, binary insertion sort): every non-wrap
exchange is moved to the front one at a time, so non-wrap exchanges come out in
**reverse** input order, wrap exchanges keep theirs. Note this diverges from a
stable sort even for two non-wrap exchanges (`[n1, n2] -> [n2, n1]`) — the
original audit's "identical output for two exchanges" claim was wrong.

`Executor03Builder.orderExchanges` now replicates the V8 result exactly
(`txbuilder/executor/executor03.go:133-155`) instead of using a stable sort.

---

## 3. Gated Params

Fields the validator rejects before bytecode generation.

| Param | Executor01 | Executor02 | Executor03 |
| --- | --- | --- | --- |
| `dexFuncHasRecipient=false` | supported | supported | supported |
| `needWrapNative=false` | **rejected** `executor01.go:120` | supported | supported |
| `needUnwrapNative` | supported (WETH-src and WETH-dest) | supported | supported |
| custom `wethAddress` | only with `needUnwrapNative` + WETH-src `executor01.go:128-134` | supported | non-wrapper routes only `executor03.go:103-112` |
| `amountsPacked128` | rejected `executor01.go:146` | rejected `executor02.go:217` | supported |
| `returnAmountPos` | supported | supported (suppressed on root unwrap, matches TS) | rejected `executor03.go:120` |
| `specialDexFlag` | whitelist | whitelist | whitelist |

### 3.1 `dexFuncHasRecipient=false` — CLOSED (2026-07-29, DEXLIB-202)

The gates were removed in commit `231d1db`; the flag logic and trailers were
already ported and are now live:

- flags: `executor01.go:283,300,355,373,386,391`,
  `executor02.go:281,298,376,396,409,414`, `executor03.go:226,649-656`
- trailers: `executor01.go:63-95` (route-level transfer + native send),
  `executor02.go:827,847` (per-exchange transfer + native send)
- Executor03 emits no trailer — parity with TS, where the transfer/send blocks
  are commented out (`Executor03BytecodeBuilder.ts:288-324`)

Fixtures: `txbuilder/executor/executor_no_recipient_test.go`. Same shape as the
already-closed `returnAmountPos` / `insertFromAmountPos` gaps — see
`docs/EXECUTOR_INSERT_FROM_AMOUNT_POS_GAP.md`.

### 3.2 Executor01 `needUnwrapNative` on a WETH-dest hop — CLOSED (2026-07-29)

The `isWETHDestWrap` branch (`Executor01BytecodeBuilder.ts:308-315` — the
`deposit` wrap-after-swap step) was genuinely absent, so the gate was
load-bearing. Both are fixed: the branch is ported into
`Executor01Builder.buildSingleSwapCallData` and the `needUnwrapNative` gate now
accepts WETH-src or WETH-dest. Fixture:
`TestExecutor01WETHDestWrapAfterSwap`.

### 3.3 `specialDexFlag` whitelist

`validateSpecialDexFlagOverride` (`txbuilder/executor/base.go:131-153`)
whitelists 11 flags. Correctly excluded because they are executor-internal:
`SEND_NATIVE = 4`, `EXECUTE_VERTICAL_BRANCHING = 10`. Missing entirely:
`REVERTABLE_FALLBACK_GROUP = 0xff` (see section 1).

---

## 4. Structural / Route-Shape Gaps

### 4.1 Executor02 root deposit on a single-swap route — CLOSED (2026-07-29)

Go used to hard-error on `needWrapEth && routeNeedsRootWrapEth` when the route
was neither multi-swap nor mega-swap. Now ports the legacy behavior
(`Executor02BytecodeBuilder.ts:1890-1911`): single-swap routes prefix the root
deposit with `[len(16)][percent*100(16)]`. Fixture:
`TestExecutor02RootDepositOnSingleSwapRoute`.

### 4.2 Consistent-with-detector restrictions

These reject shapes that `DetectExecutor` cannot produce, so they are consistent
rather than gaps:

- Executor01 rejects more than one route (`executor01.go:105`) and more than one
  `swapExchange` per swap (`executor01.go:112`).
- Executor03 rejects more than one route and more than one swap
  (`executor03.go:91-96`).

Executor01's `isMegaSwap` / `isMultiSwap` flag paths (`executor01.go:176-180`)
are unreachable as a result — the same is true in legacy.

BUY-side multi-swap is unsupported on both sides.

---

## 5. Confirmed At Parity

Checked, no gap found:

- Fee packing, including the `isSkipBlacklist` / `isCapSurplus` /
  `isDirectFeeTransfer` / `isSurplusToUser` / referral bits
  (`txbuilder/resolved/fees.go` vs `buildFeesV6`).
- `getDexCallsParams` equivalent: recipient resolution, BUY-side `srcAmount`
  mul-div, `forceUnwrap` (`txbuilder/builder/orchestration.go:82-159`).
- `hasAnyRouteWithEthAndDifferentNeedWrapNative` and the WETH-plan skip
  conditions.
- Executor detection, including the WETH single-wrap route and the deliberate
  Arbitrum omission (`txbuilder/builder/executor_detector.go`).
- All 10 V6 direct contract methods (`txbuilder/resolved/direct_validation.go:14-25`)
  against the legacy `getDirectFunctionNameV6` set: UniswapV2 sell/buy,
  UniswapV3 sell/buy, BalancerV2 sell/buy, CurveV1, CurveV2, MakerPSM,
  AugustusRFQ try-batch-fill.
- `permit2Approval` and the `DISABLED_MAX_UNIT_APPROVAL_TOKENS` reset-approval
  sequencing (`txbuilder/executor/base.go:477-737`).
- `transferSrcTokenBeforeSwap` on all three executors. Note this contradicts
  `docs/EXECUTOR_VALIDATION_ONLY_GATES_PLAN.md`, which still lists Executor02/03
  `transferSrcTokenBeforeSwap` under "Keep Gated For Now" — that entry is stale.
- WETH executor returns `0x` bytecode, matching `WETHBytecodeBuilder`.

---

## 6. Suggested Order of Work

1. **`amountsPacked128` for Executor01/02** — port `applyIs128` (`flag | 0x8000`)
   plus the 128-bit position search that Executor03 already has (the shared
   `findAmountPosWithFallback` now takes an `is128` flag, so most of this is
   wiring).
2. **Executor03 `returnAmountPos`** — needs a contract-level decision on mapping
   to `toAmountPos`.
3. **Revertable fallback groups** (section 1) — tracked as DEXLIB-203; largest,
   and depends on route input carrying `fallback`.

Done since the audit (all 2026-07-29):

- ~~`dexFuncHasRecipient=false` on all three executors~~ — DEXLIB-202,
  commit `231d1db`, see section 3.1.
- ~~negative `int256` amount-position fallback for Executor01/02~~ — section 2.1.
- ~~`insertFromAmountPos == 0` truthiness~~ — section 2.2.
- ~~Executor03 exchange ordering~~ — section 2.3.
- ~~Executor01 `isWETHDestWrap` + WETH-dest `needUnwrapNative` gate~~ — section 3.2.
- ~~Executor02 root deposit on single-swap routes~~ — section 4.1.

---

## 7. Out of Scope

Legacy V5 routers have no Go counterpart, which is consistent with
`IsGenericContractMethod` (`txbuilder/resolved/validation.go:66-71`) accepting
only `swapExactAmountIn`, `swapExactAmountOut`, and their `Pro` variants:

- `src/router/simpleswap.ts`, `multiswap.ts`, `megaswap.ts`, `buy.ts`,
  `simpleswapnft.ts`
- the adapter (`getAdapterParam`) path

---

## 8. Audit Caveat

At audit time `go build ./...` failed (`go.sum` lacked the entry for
`github.com/ethereum/go-ethereum v1.16.3`), so the original findings came from
reading both codebases, not from test evidence.

Resolved 2026-07-29 via `go mod tidy`: the full `txbuilder` suite now runs and
passes, except two pre-existing `txbuilder/builder` failures that read TS
fixtures from a `paraswap-dex-lib/tests/...` relative path not present in this
checkout (unrelated to any gap above).

---

## References

- `docs/EXECUTOR_INSERT_FROM_AMOUNT_POS_GAP.md` — closed gap, same shape as section 3.1
- `docs/EXECUTOR_VALIDATION_ONLY_GATES_PLAN.md` — partly stale, see section 5
- `docs/TXBUILDER_USAGE.md` — public surface
