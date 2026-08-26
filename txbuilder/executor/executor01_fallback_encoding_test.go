package executor

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const (
	fbSrcAmount  = resolved.DecimalString("1000000000")
	fbDestAmount = resolved.DecimalString("500000000000000000")

	fbWithdrawSelector = "2e1a7d4d"
	fbDepositSelector  = "d0e30db0"

	// Targets are distinct from every token constant so contains/position
	// assertions never collide with a token address embedded in a step.
	fbTargetFallback resolved.Address = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fbTargetPrimary  resolved.Address = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	fbTargetHop      resolved.Address = "0xcccccccccccccccccccccccccccccccccccccccc"
	fbTargetOther    resolved.Address = "0xdddddddddddddddddddddddddddddddddddddddd"
)

// Group step metadata for Executor01 (hex, no 0x): zero target(20) + size(4) +
// zero positions(5) + 0xff + zero flag(2).
var fbGroupStepMarker01 = regexp.MustCompile(`0{40}[0-9a-f]{8}0{10}ff0000`)

// fbInnerSteps strips the outer [offset(32)][length(32)] wrapper that
// BuildBytecode prepends, leaving just the concatenated steps (hex, no 0x).
func fbInnerSteps(bytecode resolved.HexBytes) string {
	return strip0x(string(bytecode))[128:]
}

func fbHexUint(t *testing.T, raw string) int {
	t.Helper()
	value, err := strconv.ParseUint(raw, 16, 32)
	if err != nil {
		t.Fatalf("parse hex %q: %v", raw, err)
	}
	return int(value)
}

// fbParseGroup locates the first group step and returns its try and fallback
// blocks (hex, no 0x).
func fbParseGroup(t *testing.T, marker *regexp.Regexp, steps string) (tryBlock, fallbackBlock string) {
	t.Helper()
	loc := marker.FindStringIndex(steps)
	if loc == nil {
		t.Fatalf("no group step in calldata: %s", steps)
	}
	payload := steps[loc[0]+64+56:]
	tryLen := fbHexUint(t, payload[0:8]) * 2
	fallbackLen := fbHexUint(t, payload[8:16]) * 2
	if len(payload) < 16+tryLen+fallbackLen {
		t.Fatalf("group payload shorter than its declared blocks")
	}
	return payload[16 : 16+tryLen], payload[16+tryLen : 16+tryLen+fallbackLen]
}

func fbCount(haystack, needle string) int {
	return strings.Count(haystack, needle)
}

func fbRawBool(value bool) resolved.RawBool {
	return resolved.RawBool{Value: value, Valid: true, Present: true}
}

func fbPad32(t *testing.T, amount resolved.DecimalString) string {
	t.Helper()
	encoded, err := encodeUint256Decimal(amount)
	if err != nil {
		t.Fatalf("encode amount %s: %v", amount, err)
	}
	return strip0x(encoded)
}

// fbParam mirrors the hand-rolled params of the TS mixed-wrap-ness tests: a
// 4-byte selector followed by the src amount, with insertFromAmountPos fixed.
func fbParam(t *testing.T, selector string, needWrapNative, dexFuncHasRecipient bool, target resolved.Address) resolved.DexExchangeBuildParam {
	t.Helper()
	insertFromAmountPos := 4
	return resolved.DexExchangeBuildParam{
		NeedWrapNative:      fbRawBool(needWrapNative),
		DexFuncHasRecipient: dexFuncHasRecipient,
		ExchangeData:        resolved.HexBytes("0x" + selector + fbPad32(t, fbSrcAmount)),
		InsertFromAmountPos: &insertFromAmountPos,
		TargetExchange:      target,
	}
}

func fbSwap(srcToken, destToken resolved.Address) resolved.RoutePlanSwap {
	return resolved.RoutePlanSwap{
		SrcToken:   srcToken,
		DestToken:  destToken,
		SrcAmount:  fbSrcAmount,
		DestAmount: fbDestAmount,
		SwapExchanges: []resolved.RoutePlanSwapExchange{
			{Exchange: "Dexalot", Percent: 100, SrcAmount: fbSrcAmount, DestAmount: fbDestAmount},
		},
	}
}

func fbRoute(swaps ...resolved.RoutePlanSwap) executorRoute {
	return executorRoute{
		SrcToken:   swaps[0].SrcToken,
		DestToken:  swaps[len(swaps)-1].DestToken,
		DestAmount: fbDestAmount,
		BestRoute:  []resolved.RoutePlanRoute{{Percent: 100, Swaps: swaps}},
	}
}

func fbWethWithdrawPlan(t *testing.T) *resolved.WethPlan {
	t.Helper()
	return &resolved.WethPlan{
		Withdraw: &resolved.WethSubPlan{
			Callee:   testWETH,
			Calldata: resolved.HexBytes("0x" + fbWithdrawSelector + fbPad32(t, fbDestAmount)),
			Value:    "0",
		},
	}
}

func fbBuildInputWithWeth(
	priceRoute executorRoute,
	exchangeParams []resolved.DexExchangeBuildParam,
	wethPlan *resolved.WethPlan,
) resolved.ExecutorBytecodeBuildInput {
	input := testBytecodeBuildInput(priceRoute, exchangeParams)
	input.WethPlan = wethPlan
	return input
}

func TestExecutor01RevertableGroupLayout(t *testing.T) {
	builder := NewExecutor01Builder(testEncodingContextWithAugustus())

	priceRoute, primary := testExecutorRouteAndParams(0)
	primary[0].ReturnAmountPos = nil

	fallbackParam := primary[0]
	fallbackParam.TargetExchange = fbTargetFallback

	primaryOnly, err := builder.BuildBytecode(testBytecodeBuildInput(priceRoute, primary))
	if err != nil {
		t.Fatalf("primary-only BuildBytecode() error = %v", err)
	}
	fallbackOnly, err := builder.BuildBytecode(
		testBytecodeBuildInput(priceRoute, []resolved.DexExchangeBuildParam{fallbackParam}),
	)
	if err != nil {
		t.Fatalf("fallback-only BuildBytecode() error = %v", err)
	}

	groupedParams := []resolved.DexExchangeBuildParam{primary[0]}
	groupedParams[0].FallbackParam = &fallbackParam
	grouped, err := builder.BuildBytecode(testBytecodeBuildInput(priceRoute, groupedParams))
	if err != nil {
		t.Fatalf("grouped BuildBytecode() error = %v", err)
	}

	if grouped == primaryOnly {
		t.Fatal("grouped bytecode must differ from the plain primary encoding")
	}

	tryBlock := fbInnerSteps(primaryOnly)
	fallbackBlock := fbInnerSteps(fallbackOnly)
	step := fbInnerSteps(grouped)

	// --- metadata word (EXECUTOR_01_02 layout) ---
	// [addr(20)][size(4)][fromAmtPos(2)][srcTokPos(2)][retPos(1)][specialDex(1)][flag(2)][zeros(28)][payload]
	if addr := step[0:40]; addr != strings.Repeat("0", 40) {
		t.Errorf("group target = %s, want zero address", addr)
	}
	if special := step[58:60]; special != "ff" {
		t.Errorf("specialDex byte = %s, want ff", special)
	}
	if flagHex := step[60:64]; flagHex != "0000" {
		t.Errorf("group flag = %s, want 0000", flagHex)
	}

	// --- payload: [28-byte padding][tryLen(4)][fallbackLen(4)][try][fallback] ---
	payload := step[64+28*2:]
	tryLen := fbHexUint(t, payload[0:8])
	fallbackLen := fbHexUint(t, payload[8:16])
	blocks := payload[16:]

	if tryLen != len(tryBlock)/2 {
		t.Errorf("tryLen = %d, want %d", tryLen, len(tryBlock)/2)
	}
	if fallbackLen != len(fallbackBlock)/2 {
		t.Errorf("fallbackLen = %d, want %d", fallbackLen, len(fallbackBlock)/2)
	}
	if got := blocks[:len(tryBlock)]; got != tryBlock {
		t.Errorf("try block does not match the plain primary encoding")
	}
	if got := blocks[len(tryBlock):]; got != fallbackBlock {
		t.Errorf("fallback block does not match the plain fallback encoding")
	}

	// calldataSize in the metadata word = payload bytes + 28 (standard packing).
	if size := fbHexUint(t, step[40:48]); size != len(payload)/2+28 {
		t.Errorf("calldata size = %d, want %d", size, len(payload)/2+28)
	}
}

func TestExecutor01GroupRecipientMismatchAppendsForward(t *testing.T) {
	// Recipient-capable primary (BuildBytecode appends no route-level forward)
	// with a false-recipient fallback: the executor->Augustus transfer must
	// ride INSIDE the fallback block, amount inserted at runtime.
	builder := NewExecutor01Builder(testEncodingContextWithAugustus())

	priceRoute := fbRoute(fbSwap(testSrcToken, testDestToken))
	primary := fbParam(t, "deadbeef", false, true, fbTargetPrimary)
	fallbackParam := fbParam(t, "feedc0de", false, false, fbTargetFallback)
	primary.FallbackParam = &fallbackParam

	grouped, err := builder.BuildBytecode(
		testBytecodeBuildInput(priceRoute, []resolved.DexExchangeBuildParam{primary}),
	)
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}
	tryBlock, fallbackBlock := fbParseGroup(t, fbGroupStepMarker01, fbInnerSteps(grouped))

	transferCallData, err := buildERC20TransferCalldata(testAugustusV6, priceRoute.DestAmount)
	if err != nil {
		t.Fatalf("buildERC20TransferCalldata() error = %v", err)
	}
	wrappedTransfer, err := buildTransferCallData(transferCallData, priceRoute.DestToken)
	if err != nil {
		t.Fatalf("buildTransferCallData() error = %v", err)
	}
	trailer := strip0x(string(wrappedTransfer))

	if !strings.HasSuffix(fallbackBlock, trailer) {
		t.Error("fallback block must end with the executor->Augustus transfer")
	}
	if fbCount(tryBlock, testTransferSelector) != 0 {
		t.Error("try block must not carry a transfer trailer")
	}
	// Nothing after the group step: the recipient-capable primary needs no
	// route-level forward.
	steps := fbInnerSteps(grouped)
	loc := fbGroupStepMarker01.FindStringIndex(steps)
	size := fbHexUint(t, steps[loc[0]+40:loc[0]+48])
	if len(steps) != loc[0]+64+56+(size-28)*2 {
		t.Error("group must be the last step in the route calldata")
	}
}

func TestExecutor01GroupFinalHopEthDestMixedWrapness(t *testing.T) {
	builder := NewExecutor01Builder(testEncodingContextWithAugustus())
	wethPlan := fbWethWithdrawPlan(t)

	priceRoute := fbRoute(fbSwap(testSrcToken, resolved.NativeTokenAddress))
	rawPrimary := fbParam(t, "deadbeef", false, true, fbTargetPrimary)
	wethFallback := fbParam(t, "feedc0de", true, true, fbTargetFallback)

	finalFlagCalldata, err := buildFinalSpecialFlagCalldata(testEncodingContextWithAugustus())
	if err != nil {
		t.Fatalf("buildFinalSpecialFlagCalldata() error = %v", err)
	}
	finalFlagHex := strip0x(string(finalFlagCalldata))

	// Plain builds of each alternative as its own route: [unit][route-level send].
	plainPrimaryBytecode, err := builder.BuildBytecode(
		fbBuildInputWithWeth(priceRoute, []resolved.DexExchangeBuildParam{rawPrimary}, wethPlan),
	)
	if err != nil {
		t.Fatalf("plain primary BuildBytecode() error = %v", err)
	}
	plainFallbackBytecode, err := builder.BuildBytecode(
		fbBuildInputWithWeth(priceRoute, []resolved.DexExchangeBuildParam{wethFallback}, wethPlan),
	)
	if err != nil {
		t.Fatalf("plain fallback BuildBytecode() error = %v", err)
	}
	plainPrimary := fbInnerSteps(plainPrimaryBytecode)
	plainFallback := fbInnerSteps(plainFallbackBytecode)
	if !strings.HasSuffix(plainPrimary, finalFlagHex) || !strings.HasSuffix(plainFallback, finalFlagHex) {
		t.Fatal("plain builds must end with the route-level native send")
	}

	groupedPrimary := rawPrimary
	groupedPrimary.FallbackParam = &wethFallback
	groupedBytecode, err := builder.BuildBytecode(
		fbBuildInputWithWeth(priceRoute, []resolved.DexExchangeBuildParam{groupedPrimary}, wethPlan),
	)
	if err != nil {
		t.Fatalf("grouped BuildBytecode() error = %v", err)
	}
	grouped := fbInnerSteps(groupedBytecode)

	// The group is encoded (no guard drops it on the final hop).
	loc := fbGroupStepMarker01.FindStringIndex(grouped)
	if loc == nil {
		t.Fatalf("no group step in calldata: %s", grouped)
	}

	payload := grouped[64+28*2:]
	tryLen := fbHexUint(t, payload[0:8]) * 2
	fallbackLen := fbHexUint(t, payload[8:16]) * 2
	tryBlock := payload[16 : 16+tryLen]
	fallbackBlock := payload[16+tryLen : 16+tryLen+fallbackLen]

	// Nothing after the group step — the route-level native send is suppressed
	// (the fallback block carries its own; the try branch delivered itself).
	if len(grouped) != 64+28*2+16+tryLen+fallbackLen {
		t.Error("route-level native send must be suppressed for this shape")
	}

	// The try block is the primary's unit, byte-identical to a plain build
	// (minus the route-level send, which a plain build appends after it).
	if tryBlock != strings.TrimSuffix(plainPrimary, finalFlagHex) {
		t.Error("try block must equal the plain primary unit")
	}

	// The fallback block is exactly what a standalone route through the
	// fallback would encode: its unit (dex call + unwrap) + the native send.
	if fallbackBlock != plainFallback {
		t.Error("fallback block must equal the plain fallback route encoding")
	}
	if !strings.HasSuffix(fallbackBlock, finalFlagHex) {
		t.Error("fallback block must end with the native send")
	}
	if fbCount(fallbackBlock, fbWithdrawSelector) == 0 {
		t.Error("fallback block must carry the WETH withdraw")
	}
}

func TestExecutor01GroupFinalHopMultihop(t *testing.T) {
	builder := NewExecutor01Builder(testEncodingContextWithAugustus())
	wethPlan := fbWethWithdrawPlan(t)

	priceRoute := fbRoute(
		fbSwap(testSrcToken, testDestToken),
		fbSwap(testDestToken, resolved.NativeTokenAddress),
	)
	hop1 := fbParam(t, "cafebabe", false, false, fbTargetHop)
	rawPrimary := fbParam(t, "deadbeef", false, true, fbTargetPrimary)
	wethFallback := fbParam(t, "feedc0de", true, true, fbTargetFallback)
	rawPrimary.FallbackParam = &wethFallback

	groupedBytecode, err := builder.BuildBytecode(
		fbBuildInputWithWeth(priceRoute, []resolved.DexExchangeBuildParam{hop1, rawPrimary}, wethPlan),
	)
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}
	grouped := fbInnerSteps(groupedBytecode)

	// The group is encoded and is the LAST step — no route-level native send
	// after it (grouped calldata ends exactly where the group step ends).
	loc := fbGroupStepMarker01.FindStringIndex(grouped)
	if loc == nil {
		t.Fatalf("no group step in calldata: %s", grouped)
	}
	size := fbHexUint(t, grouped[loc[0]+40:loc[0]+48])
	if len(grouped) != loc[0]+64+56+(size-28)*2 {
		t.Error("group must be the last step; the route-level send is suppressed")
	}
}

// --- MID-ROUTE ETH-dest mixed wrap-ness: the fallback block must end in the
// same token form the primary threads to the next hop. ---

func fbMidRouteBuild(
	t *testing.T,
	builder Executor01Builder,
	primaryHop, fallbackHop, nextHop resolved.DexExchangeBuildParam,
) string {
	t.Helper()
	priceRoute := fbRoute(
		fbSwap(testSrcToken, resolved.NativeTokenAddress),
		fbSwap(resolved.NativeTokenAddress, testDestToken),
	)
	primaryHop.FallbackParam = &fallbackHop
	bytecode, err := builder.BuildBytecode(fbBuildInputWithWeth(
		priceRoute,
		[]resolved.DexExchangeBuildParam{primaryHop, nextHop},
		fbWethWithdrawPlan(t),
	))
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}
	return fbInnerSteps(bytecode)
}

func TestExecutor01GroupMidRouteEthDestMixedWrapness(t *testing.T) {
	builder := NewExecutor01Builder(testEncodingContextWithAugustus())

	rawPrimary := fbParam(t, "deadbeef", false, true, fbTargetPrimary)
	wethFallback := fbParam(t, "feedc0de", true, true, fbTargetFallback)
	rawNextHop := fbParam(t, "cafebabe", false, true, fbTargetHop)
	wethNextHop := rawNextHop
	wethNextHop.NeedWrapNative = fbRawBool(true)

	t.Run("raw primary + WETH fallback + raw next hop: fallback unwraps", func(t *testing.T) {
		tryBlock, fallbackBlock := fbParseGroup(t, fbGroupStepMarker01,
			fbMidRouteBuild(t, builder, rawPrimary, wethFallback, rawNextHop))
		if fbCount(tryBlock, fbWithdrawSelector) != 0 {
			t.Error("try block must not unwrap")
		}
		if fbCount(fallbackBlock, fbWithdrawSelector) != 1 {
			t.Error("fallback block must unwrap once")
		}
		if fbCount(fallbackBlock, fbDepositSelector) != 0 {
			t.Error("fallback block must not deposit")
		}
	})

	t.Run("raw primary + WETH fallback + WETH next hop: fallback unwraps to the raw threading form", func(t *testing.T) {
		// Threading form is raw native (the next unit deposits its own input,
		// as shaped by the raw primary), so the WETH-holding fallback unwraps.
		tryBlock, fallbackBlock := fbParseGroup(t, fbGroupStepMarker01,
			fbMidRouteBuild(t, builder, rawPrimary, wethFallback, wethNextHop))
		if fbCount(tryBlock, fbWithdrawSelector) != 0 {
			t.Error("try block must not unwrap")
		}
		if fbCount(fallbackBlock, fbWithdrawSelector) != 1 {
			t.Error("fallback block must unwrap once")
		}
		if fbCount(fallbackBlock, fbDepositSelector) != 0 {
			t.Error("fallback block must not deposit")
		}
	})

	t.Run("WETH primary + raw fallback + WETH next hop: fallback wraps to the WETH threading form", func(t *testing.T) {
		wethPrimary := wethFallback
		wethPrimary.TargetExchange = fbTargetPrimary
		rawFallback := rawPrimary
		rawFallback.TargetExchange = fbTargetFallback
		// The primary keeps WETH (its unit skips the unwrap before a
		// needWrapNative hop), so the raw-native fallback must wrap its output.
		tryBlock, fallbackBlock := fbParseGroup(t, fbGroupStepMarker01,
			fbMidRouteBuild(t, builder, wethPrimary, rawFallback, wethNextHop))
		if fbCount(tryBlock, fbWithdrawSelector) != 0 || fbCount(tryBlock, fbDepositSelector) != 0 {
			t.Error("try block must not carry a normalization step")
		}
		if fbCount(fallbackBlock, fbDepositSelector) != 1 {
			t.Error("fallback block must deposit once")
		}
		if fbCount(fallbackBlock, fbWithdrawSelector) != 0 {
			t.Error("fallback block must not unwrap")
		}
	})
}

func TestExecutor01ExecutorIsDestReceiverFlags(t *testing.T) {
	builder := NewExecutor01Builder(testEncodingContextWithAugustus())

	t.Run("simple swap forces the src-token balance check", func(t *testing.T) {
		priceRoute := fbRoute(fbSwap(testSrcToken, testDestToken))
		param := fbParam(t, "deadbeef", false, true, fbTargetPrimary)
		param.ExecutorIsDestReceiver = true

		flags, err := builder.buildFlags(priceRoute, []resolved.DexExchangeBuildParam{param}, nil)
		if err != nil {
			t.Fatalf("buildFlags() error = %v", err)
		}
		if flags.dexes[0] != insertFromAmountCheckSrcTokenBalanceAfterSwap {
			t.Errorf("dex flag = %d, want %d", flags.dexes[0], insertFromAmountCheckSrcTokenBalanceAfterSwap)
		}
	})

	t.Run("multi swap forces the balance check on the last swap", func(t *testing.T) {
		priceRoute := fbRoute(
			fbSwap(testSrcToken, testDestToken),
			fbSwap(testDestToken, testSrcToken),
		)
		hop1 := fbParam(t, "cafebabe", false, true, fbTargetHop)
		last := fbParam(t, "deadbeef", false, true, fbTargetPrimary)
		last.ExecutorIsDestReceiver = true

		flags, err := builder.buildFlags(priceRoute, []resolved.DexExchangeBuildParam{hop1, last}, nil)
		if err != nil {
			t.Fatalf("buildFlags() error = %v", err)
		}
		if flags.dexes[1] != insertFromAmountCheckSrcTokenBalanceAfterSwap {
			t.Errorf("last dex flag = %d, want %d", flags.dexes[1], insertFromAmountCheckSrcTokenBalanceAfterSwap)
		}
	})
}
