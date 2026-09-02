package executor

import (
	"regexp"
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const fbApproveSelector = "095ea7b3"

// Group step metadata for Executor02 (hex, no 0x): zero target(20) + size(4) +
// fromAmountPos(2)=0 + destTokenPos(2) + returnAmountPos(1)=0 + 0xff + flag(2).
var fbGroupStepMarker02 = regexp.MustCompile(`0{40}[0-9a-f]{8}0000[0-9a-f]{4}00ff[0-9a-f]{4}`)

func fbSplitSwap(srcToken, destToken resolved.Address) resolved.RoutePlanSwap {
	return resolved.RoutePlanSwap{
		SrcToken:   srcToken,
		DestToken:  destToken,
		SrcAmount:  fbSrcAmount,
		DestAmount: fbDestAmount,
		SwapExchanges: []resolved.RoutePlanSwapExchange{
			{Exchange: "dexA", Percent: 60, SrcAmount: "600000000", DestAmount: "300000000000000000"},
			{Exchange: "dexB", Percent: 40, SrcAmount: "400000000", DestAmount: "200000000000000000"},
		},
	}
}

func fbWethDepositPlan(t *testing.T) *resolved.WethPlan {
	t.Helper()
	return &resolved.WethPlan{
		Deposit: &resolved.WethSubPlan{
			Callee:   testWETH,
			Calldata: resolved.HexBytes("0x" + fbDepositSelector),
			Value:    fbSrcAmount,
		},
	}
}

func fbBuild02(
	t *testing.T,
	priceRoute executorRoute,
	exchangeParams []resolved.DexExchangeBuildParam,
	wethPlan *resolved.WethPlan,
) string {
	t.Helper()
	bytecode, err := NewExecutor02Builder(testEncodingContextWithAugustus()).BuildBytecode(
		fbBuildInputWithWeth(priceRoute, exchangeParams, wethPlan),
	)
	if err != nil {
		t.Fatalf("BuildBytecode() error = %v", err)
	}
	return fbInnerSteps(bytecode)
}

func TestExecutor02GroupWholeHopFlagAndDestTokenPos(t *testing.T) {
	// Two-hop route, one exchange per hop: the group sits directly in the
	// horizontal sequence (insidePath = false), so it must thread the running
	// amount using the vertical-branching flag semantics.
	priceRoute := fbRoute(
		fbSwap(testSrcToken, testDestToken),
		fbSwap(testDestToken, testSrcToken),
	)
	hop0 := fbParam(t, "deadbeef", false, true, fbTargetPrimary)
	hop1 := fbParam(t, "cafebabe", false, true, fbTargetHop)

	t.Run("mid-route hop threads with the src-balance check flag", func(t *testing.T) {
		fallbackParam := fbParam(t, "feedc0de", false, true, fbTargetFallback)
		primary := hop0
		primary.FallbackParam = &fallbackParam

		grouped := fbBuild02(t, priceRoute, []resolved.DexExchangeBuildParam{primary, hop1}, nil)
		loc := fbGroupStepMarker02.FindStringIndex(grouped)
		if loc == nil {
			t.Fatalf("no group step in calldata: %s", grouped)
		}
		step := grouped[loc[0]:]
		if flagHex := step[60:64]; fbHexUint(t, flagHex) != int(insertFromAmountCheckSrcTokenBalanceAfterSwap) {
			t.Errorf("group flag = %s, want %d", flagHex, insertFromAmountCheckSrcTokenBalanceAfterSwap)
		}

		// destTokenPos points at the hop's dest token inside the payload.
		destTokenPos := fbHexUint(t, step[52:56])
		payload := step[64:]
		tokenStart := (destTokenPos + 40) * 2
		if got := payload[tokenStart : tokenStart+40]; got != strip0x(string(testDestToken)) {
			t.Errorf("destTokenPos %d points at %s, want %s", destTokenPos, got, strip0x(string(testDestToken)))
		}
	})

	t.Run("final hop threads without a balance check", func(t *testing.T) {
		fallbackParam := fbParam(t, "feedc0de", false, true, fbTargetFallback)
		last := hop1
		last.FallbackParam = &fallbackParam

		grouped := fbBuild02(t, priceRoute, []resolved.DexExchangeBuildParam{hop0, last}, nil)
		loc := fbGroupStepMarker02.FindStringIndex(grouped)
		if loc == nil {
			t.Fatalf("no group step in calldata: %s", grouped)
		}
		step := grouped[loc[0]:]
		if flagHex := step[60:64]; fbHexUint(t, flagHex) != int(insertFromAmountDontCheckBalanceAfterSwap) {
			t.Errorf("group flag = %s, want %d", flagHex, insertFromAmountDontCheckBalanceAfterSwap)
		}
	})
}

func TestExecutor02GroupSplitMember(t *testing.T) {
	// Split member: the vertical-branching wrapper above the group does the
	// threading, so the group itself must skip the balance check (flag 0).
	priceRoute := fbRoute(
		fbSplitSwap(testSrcToken, testDestToken),
		fbSwap(testDestToken, testSrcToken),
	)
	member0 := fbParam(t, "deadbeef", false, true, fbTargetPrimary)
	member1 := fbParam(t, "beefdead", false, true, fbTargetOther)
	hop1 := fbParam(t, "cafebabe", false, true, fbTargetHop)

	plain := fbBuild02(t, priceRoute, []resolved.DexExchangeBuildParam{member0, member1, hop1}, nil)
	if fbGroupStepMarker02.MatchString(plain) {
		t.Fatal("plain build must not contain a group step")
	}

	fallbackParam := fbParam(t, "feedc0de", false, true, fbTargetFallback)
	groupedMember := member0
	groupedMember.FallbackParam = &fallbackParam
	grouped := fbBuild02(t, priceRoute, []resolved.DexExchangeBuildParam{groupedMember, member1, hop1}, nil)

	if grouped == plain {
		t.Fatal("grouped bytecode must differ from the plain build")
	}
	loc := fbGroupStepMarker02.FindStringIndex(grouped)
	if loc == nil {
		t.Fatalf("no group step in calldata: %s", grouped)
	}
	step := grouped[loc[0]:]
	if flagHex := step[60:64]; flagHex != "0000" {
		t.Errorf("split-member group flag = %s, want 0000", flagHex)
	}

	tryBlock, fallbackBlock := fbParseGroup(t, fbGroupStepMarker02, grouped)
	if !strings.Contains(fallbackBlock, strip0x(string(fbTargetFallback))) {
		t.Error("fallback block must encode the substituted target")
	}
	if strings.Contains(tryBlock, strip0x(string(fbTargetFallback))) {
		t.Error("try block must not encode the fallback target")
	}
}

func TestExecutor02GroupEthDestOutputNormalization(t *testing.T) {
	// Final hop USDT -> ETH split across two WETH-based members; the root
	// unwrap and send live OUTSIDE the group, shaped by the primary, so the
	// fallback block must end in the same state the try block would.
	priceRoute := fbRoute(
		fbSwap(testSrcToken, testDestToken),
		fbSplitSwap(testDestToken, resolved.NativeTokenAddress),
	)
	hop0 := fbParam(t, "cafebabe", false, true, fbTargetHop)
	wethMember0 := fbParam(t, "deadbeef", true, true, fbTargetPrimary)
	wethMember1 := fbParam(t, "beefdead", true, true, fbTargetOther)

	build := func(t *testing.T, member0 resolved.DexExchangeBuildParam) (string, string) {
		t.Helper()
		grouped := fbBuild02(
			t,
			priceRoute,
			[]resolved.DexExchangeBuildParam{hop0, member0, wethMember1},
			fbWethWithdrawPlan(t),
		)
		return fbParseGroup(t, fbGroupStepMarker02, grouped)
	}

	t.Run("raw-ETH fallback with recipient wraps to match the WETH-holding try", func(t *testing.T) {
		// Fallback params are built with recipient = executor, so even a
		// recipient-capable raw-ETH dex leaves its ETH on the executor — the
		// compensation deposit wraps it to match the try branch.
		fallbackParam := fbParam(t, "feedc0de", false, true, fbTargetFallback)
		member := wethMember0
		member.FallbackParam = &fallbackParam
		tryBlock, fallbackBlock := build(t, member)

		if fbCount(fallbackBlock, fbDepositSelector) != 1 {
			t.Error("fallback block must wrap its raw output")
		}
		if fbCount(tryBlock, fbDepositSelector) != 0 || fbCount(tryBlock, fbWithdrawSelector) != 0 {
			t.Error("try block holds WETH for the root unwrap — no wrap/unwrap inside it")
		}
	})

	t.Run("raw-ETH fallback without recipient wraps the same way", func(t *testing.T) {
		fallbackParam := fbParam(t, "feedc0de", false, false, fbTargetFallback)
		member := wethMember0
		member.FallbackParam = &fallbackParam
		tryBlock, fallbackBlock := build(t, member)

		if fbCount(fallbackBlock, fbDepositSelector) != 1 {
			t.Error("fallback block must wrap its raw output")
		}
		if fbCount(tryBlock, fbDepositSelector) != 0 {
			t.Error("try block must not deposit")
		}
	})

	t.Run("raw-ETH primary with WETH fallback unwraps and sends the fallback output", func(t *testing.T) {
		// The route machinery expects the raw primary's output already
		// delivered ('sent'), so the WETH-holding fallback must withdraw and
		// send from inside its block.
		rawPrimary := fbParam(t, "deadbeef", false, true, fbTargetPrimary)
		fallbackParam := fbParam(t, "feedc0de", true, true, fbTargetFallback)
		rawPrimary.FallbackParam = &fallbackParam
		tryBlock, fallbackBlock := build(t, rawPrimary)

		if fbCount(fallbackBlock, fbWithdrawSelector) != 1 {
			t.Error("fallback block must unwrap its WETH output")
		}
		if fbCount(tryBlock, fbWithdrawSelector) != 0 {
			t.Error("try block must not unwrap")
		}
		augustus := strip0x(string(testAugustusV6))
		if fbCount(fallbackBlock, augustus) == 0 {
			t.Error("fallback block must end with the native send to Augustus")
		}
		if fbCount(tryBlock, augustus) != 0 {
			t.Error("try block must not send")
		}
	})
}

func TestExecutor02GroupMidRouteEthDestMixedWrapnessGuard(t *testing.T) {
	// Mixed wrap-ness on a MID-ROUTE ETH-dest hop is not normalized on
	// Executor02: the unwrap is a shared cross-member step, so a wrapness
	// substitution inside one fallback branch can't be normalized locally.
	// The group is skipped and the route encodes exactly as without it.
	priceRoute := fbRoute(
		fbSwap(testSrcToken, resolved.NativeTokenAddress),
		fbSwap(resolved.NativeTokenAddress, testDestToken),
	)
	wethPrimary := fbParam(t, "deadbeef", true, true, fbTargetPrimary)
	hop1 := fbParam(t, "cafebabe", true, true, fbTargetHop)

	plain := fbBuild02(t, priceRoute, []resolved.DexExchangeBuildParam{wethPrimary, hop1}, fbWethWithdrawPlan(t))

	fallbackParam := fbParam(t, "feedc0de", false, true, fbTargetFallback)
	groupedPrimary := wethPrimary
	groupedPrimary.FallbackParam = &fallbackParam
	grouped := fbBuild02(t, priceRoute, []resolved.DexExchangeBuildParam{groupedPrimary, hop1}, fbWethWithdrawPlan(t))

	if grouped != plain {
		t.Error("mid-route ETH-dest mixed wrap-ness must run plain (guard intact)")
	}
	if fbGroupStepMarker02.MatchString(grouped) {
		t.Error("no group step must be encoded for the guarded shape")
	}
}

func TestExecutor02GroupEthSrcInputNormalization(t *testing.T) {
	// Native-src route: ETH -> USDT (member carries the fallback) -> USDC.
	priceRoute := fbRoute(
		fbSwap(resolved.NativeTokenAddress, testDestToken),
		fbSwap(testDestToken, testSrcToken),
	)
	hop1 := fbParam(t, "cafebabe", false, true, fbTargetHop)

	t.Run("external wrap with raw-ETH fallback: slice unwrapped inside the block", func(t *testing.T) {
		// The primary's wrap is the root deposit (outside the try block), so
		// it persists after a try revert — the branch holds WETH and the
		// raw-ETH fallback must unwrap its slice first.
		wethPrimary := fbParam(t, "deadbeef", true, true, fbTargetPrimary)
		fallbackParam := fbParam(t, "feedc0de", false, true, fbTargetFallback)
		wethPrimary.FallbackParam = &fallbackParam

		grouped := fbBuild02(t, priceRoute, []resolved.DexExchangeBuildParam{wethPrimary, hop1}, fbWethDepositPlan(t))
		tryBlock, fallbackBlock := fbParseGroup(t, fbGroupStepMarker02, grouped)

		if fbCount(fallbackBlock, fbWithdrawSelector) != 1 {
			t.Error("fallback block must unwrap the slice")
		}
		if fbCount(tryBlock, fbWithdrawSelector) != 0 {
			t.Error("try block must not unwrap")
		}
	})

	t.Run("external wrap with WETH fallback: approve prepended", func(t *testing.T) {
		wethPrimary := fbParam(t, "deadbeef", true, true, fbTargetPrimary)
		fallbackParam := fbParam(t, "feedc0de", true, true, fbTargetFallback)
		fallbackParam.ApproveData = &resolved.ApproveData{
			Target: fbTargetFallback,
			Token:  testWETH,
		}
		wethPrimary.FallbackParam = &fallbackParam

		grouped := fbBuild02(t, priceRoute, []resolved.DexExchangeBuildParam{wethPrimary, hop1}, fbWethDepositPlan(t))
		tryBlock, fallbackBlock := fbParseGroup(t, fbGroupStepMarker02, grouped)

		// TS parity: the fallback unit already carries its approve in the
		// ETH-src branch, and the group prepends one more for the
		// external-wrap shape — both ride inside the block.
		if fbCount(fallbackBlock, fbApproveSelector) != 2 {
			t.Errorf("fallback approve count = %d, want 2", fbCount(fallbackBlock, fbApproveSelector))
		}
		if fbCount(tryBlock, fbApproveSelector) != 0 {
			t.Error("try block must not approve (primary has no approveData)")
		}
	})

	t.Run("raw-native primary with wrap-needing fallback: approve+deposit prepended", func(t *testing.T) {
		// Root-wrap detection runs on the substituted params, so the fallback
		// unit wrongly skips its own deposit; the group prepends it.
		rawPrimary := fbParam(t, "deadbeef", false, true, fbTargetPrimary)
		fallbackParam := fbParam(t, "feedc0de", true, true, fbTargetFallback)
		fallbackParam.ApproveData = &resolved.ApproveData{
			Target: fbTargetFallback,
			Token:  testWETH,
		}
		rawPrimary.FallbackParam = &fallbackParam

		grouped := fbBuild02(t, priceRoute, []resolved.DexExchangeBuildParam{rawPrimary, hop1}, fbWethDepositPlan(t))
		tryBlock, fallbackBlock := fbParseGroup(t, fbGroupStepMarker02, grouped)

		if fbCount(fallbackBlock, fbDepositSelector) != 1 {
			t.Errorf("fallback deposit count = %d, want 1", fbCount(fallbackBlock, fbDepositSelector))
		}
		if fbCount(fallbackBlock, fbApproveSelector) == 0 {
			t.Error("fallback block must approve before the deposit")
		}
		if fbCount(tryBlock, fbDepositSelector) != 0 {
			t.Error("try block must not deposit (raw-native primary)")
		}
	})
}
