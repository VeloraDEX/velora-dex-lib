package executor

import (
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const testApprovalSelector = "095ea7b3"

func TestExecutor01ApprovalCalldataHonorsSkipApproval(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	exchangeParams[0].ReturnAmountPos = nil
	exchangeParams[0].ApproveData = &resolved.ApproveData{
		Target: testTargetExchange,
		Token:  testSrcToken,
	}

	callData := buildExecutor01SingleSwapForApprovalTest(t, priceRoute, exchangeParams)
	if !strings.Contains(strip0x(string(callData)), testApprovalSelector) {
		t.Fatalf("approval calldata missing when ApproveData is present: %s", callData)
	}

	skipApproval := true
	exchangeParams[0].SkipApproval = &skipApproval
	callData = buildExecutor01SingleSwapForApprovalTest(t, priceRoute, exchangeParams)
	if strings.Contains(strip0x(string(callData)), testApprovalSelector) {
		t.Fatalf("approval calldata present with SkipApproval=true: %s", callData)
	}
}

func TestExecutor01ValidationAcceptsSkipApprovalAndNormalizedSpender(t *testing.T) {
	priceRoute, exchangeParams := testExecutorRouteAndParams(0)
	exchangeParams[0].ReturnAmountPos = nil
	skipApproval := true
	spender := testTargetExchange
	exchangeParams[0].SkipApproval = &skipApproval
	exchangeParams[0].Spender = &spender

	if err := NewExecutor01Builder(testEncodingContext()).validateExecutor01Input(priceRoute, exchangeParams, nil); err != nil {
		t.Fatalf("Executor01 valid skipApproval+spender rejected: %v", err)
	}
	if _, err := NewExecutor01Builder(testEncodingContext()).BuildBytecode(
		testBytecodeBuildInput(priceRoute, exchangeParams),
	); err != nil {
		t.Fatalf("Executor01 BuildBytecode() with skipApproval+spender error = %v", err)
	}

	skipApproval = false
	exchangeParams[0].SkipApproval = &skipApproval
	if err := NewExecutor01Builder(testEncodingContext()).validateExecutor01Input(priceRoute, exchangeParams, nil); err == nil {
		t.Fatalf("Executor01 accepted spender without approveData or skipApproval")
	}

	exchangeParams[0].ApproveData = &resolved.ApproveData{
		Target: spender,
		Token:  testSrcToken,
	}
	if err := NewExecutor01Builder(testEncodingContext()).validateExecutor01Input(priceRoute, exchangeParams, nil); err != nil {
		t.Fatalf("Executor01 valid spender+approveData rejected: %v", err)
	}
	if _, err := NewExecutor01Builder(testEncodingContext()).BuildBytecode(
		testBytecodeBuildInput(priceRoute, exchangeParams),
	); err != nil {
		t.Fatalf("Executor01 BuildBytecode() with spender+approveData error = %v", err)
	}
}

func buildExecutor01SingleSwapForApprovalTest(
	t *testing.T,
	priceRoute executorRoute,
	exchangeParams []resolved.DexExchangeBuildParam,
) resolved.HexBytes {
	t.Helper()

	builder := NewExecutor01Builder(testEncodingContext())
	flags, err := builder.buildFlags(priceRoute, exchangeParams, nil)
	if err != nil {
		t.Fatalf("buildFlags() error = %v", err)
	}
	callData, err := builder.buildSingleSwapCallData(priceRoute, exchangeParams, 0, flags, nil)
	if err != nil {
		t.Fatalf("buildSingleSwapCallData() error = %v", err)
	}
	return callData
}
