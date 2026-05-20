package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const tsDexLibRoot = "/Users/danylokaniev/work/paraswap/paraswap-dex-lib"
const tsMainnetWETH = resolved.Address("0xc02aaa39b223fe8d0a0e5c4f27ead9083c756cc2")

type tsOptimalRoute struct {
	SrcToken   resolved.Address          `json:"srcToken"`
	DestToken  resolved.Address          `json:"destToken"`
	DestAmount resolved.DecimalString    `json:"destAmount"`
	BestRoute  []resolved.RoutePlanRoute `json:"bestRoute"`
}

func TestFeature4MatchesRecordedTSBytecode(t *testing.T) {
	if _, err := os.Stat(tsDexLibRoot); err != nil {
		t.Skipf("TS dex-lib repo unavailable at %s: %v", tsDexLibRoot, err)
	}

	executor02Golden := loadTSSnapshotObject(
		t,
		"missing executor TS parity fixtures records Executor02 unwrap and custom-WETH reference bytecode 1",
	)
	executor02ReturnInsertGolden := loadTSSnapshotObject(
		t,
		"missing executor TS parity fixtures records Executor02 reference bytecode for return/insert overrides 1",
	)
	executor03Golden := loadTSSnapshotObject(
		t,
		"missing executor TS parity fixtures records Executor03 unwrap and custom-WETH reference bytecode 1",
	)

	testCases := []struct {
		name  string
		want  string
		build func(t *testing.T) (resolved.HexBytes, error)
	}{
		{
			name: "Executor02 weth source unwrap before dex call",
			want: executor02Golden["weth source unwrap before dex call"],
			build: func(t *testing.T) (resolved.HexBytes, error) {
				route, params := loadExecutor02NativeDestCase(t, 1)
				setFirstSwapSourceToken(&route, tsMainnetWETH)
				needUnwrapNative := true
				params[0].NeedUnwrapNative = &needUnwrapNative
				return NewExecutor02Builder(tsMainnetEncodingContext()).BuildBytecode(
					buildTSParityInput(route, params, nil),
				)
			},
		},
		{
			name: "Executor02 weth destination wrap after dex call",
			want: executor02Golden["weth destination wrap after dex call"],
			build: func(t *testing.T) (resolved.HexBytes, error) {
				route, params := loadExecutor02NativeDestCase(t, 1)
				setFirstSwapDestToken(&route, tsMainnetWETH)
				needUnwrapNative := true
				params[0].NeedUnwrapNative = &needUnwrapNative
				return NewExecutor02Builder(tsMainnetEncodingContext()).BuildBytecode(
					buildTSParityInput(route, params, nil),
				)
			},
		},
		{
			name: "Executor02 root/native unwrap fallback",
			want: executor02ReturnInsertGolden["returnAmountPos ignored for root unwrap fallback"],
			build: func(t *testing.T) (resolved.HexBytes, error) {
				route, params := loadExecutor02NativeDestFullCase(t)
				returnAmountPos := 7
				for index := range params {
					params[index].ReturnAmountPos = &returnAmountPos
				}
				return NewExecutor02Builder(tsMainnetEncodingContext()).BuildBytecode(
					buildTSParityInput(route, params, loadExecutor02NativeDestWethPlan(t)),
				)
			},
		},
		{
			name: "Executor02 mixed needWrapNative last-swap unwrap",
			want: executor02Golden["mixed needWrapNative last-swap unwrap"],
			build: func(t *testing.T) (resolved.HexBytes, error) {
				route, params := loadExecutor02NativeDestCase(t, 2)
				params[0].NeedWrapNative = resolved.RawBool{Value: true, Valid: true, Present: true}
				params[1].NeedWrapNative = resolved.RawBool{Value: false, Valid: true, Present: true}
				return NewExecutor02Builder(tsMainnetEncodingContext()).BuildBytecode(
					buildTSParityInput(route, params, loadExecutor02NativeDestWethPlan(t)),
				)
			},
		},
		{
			name: "Executor02 custom wethAddress",
			want: executor02Golden["custom wethAddress"],
			build: func(t *testing.T) (resolved.HexBytes, error) {
				route, params := loadExecutor02NativeDestCase(t, 1)
				params[0].WethAddress = &testCustomWETH
				return NewExecutor02Builder(tsMainnetEncodingContext()).BuildBytecode(
					buildTSParityInput(route, params, loadExecutor02NativeDestWethPlan(t)),
				)
			},
		},
		{
			name: "Executor03 weth source unwrap before dex call",
			want: executor03Golden["weth source unwrap before dex call"],
			build: func(t *testing.T) (resolved.HexBytes, error) {
				route, params := loadExecutor03SimpleCase(t)
				setFirstSwapSourceToken(&route, tsMainnetWETH)
				needUnwrapNative := true
				params[0].NeedUnwrapNative = &needUnwrapNative
				input := buildTSParityInput(route, params, nil)
				input.ExecutorType = resolved.Executor03
				return NewExecutor03Builder(tsMainnetEncodingContext()).BuildBytecode(input)
			},
		},
		{
			name: "Executor03 weth destination wrap after dex call",
			want: executor03Golden["weth destination wrap after dex call"],
			build: func(t *testing.T) (resolved.HexBytes, error) {
				route, params := loadExecutor03SimpleCase(t)
				setFirstSwapDestToken(&route, tsMainnetWETH)
				needUnwrapNative := true
				params[0].NeedUnwrapNative = &needUnwrapNative
				input := buildTSParityInput(route, params, nil)
				input.ExecutorType = resolved.Executor03
				return NewExecutor03Builder(tsMainnetEncodingContext()).BuildBytecode(input)
			},
		},
		{
			name: "Executor03 custom wethAddress",
			want: executor03Golden["custom wethAddress"],
			build: func(t *testing.T) (resolved.HexBytes, error) {
				route, params := loadExecutor03SimpleCase(t)
				params[0].WethAddress = &testCustomWETH
				input := buildTSParityInput(route, params, nil)
				input.ExecutorType = resolved.Executor03
				return NewExecutor03Builder(tsMainnetEncodingContext()).BuildBytecode(input)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.want == "" {
				t.Fatalf("missing TS golden for %s", tc.name)
			}
			got, err := tc.build(t)
			if err != nil {
				t.Fatalf("BuildBytecode() error = %v", err)
			}
			if !strings.EqualFold(string(got), tc.want) {
				t.Fatalf("bytecode mismatch\nwant: %s\n got: %s", tc.want, got)
			}
		})
	}
}

func loadExecutor02NativeDestCase(
	t *testing.T,
	exchangeCount int,
) (executorRoute, []resolved.DexExchangeBuildParam) {
	t.Helper()

	route, params := loadExecutor02NativeDestFullCase(t)
	if exchangeCount > len(route.BestRoute[0].Swaps[0].SwapExchanges) ||
		exchangeCount > len(params) {
		t.Fatalf("executor02 native-dest fixture has fewer than %d exchanges", exchangeCount)
	}
	route.BestRoute[0].Swaps[0].SwapExchanges =
		route.BestRoute[0].Swaps[0].SwapExchanges[:exchangeCount]
	return route, params[:exchangeCount]
}

func loadExecutor02NativeDestFullCase(t *testing.T) (executorRoute, []resolved.DexExchangeBuildParam) {
	t.Helper()

	route := loadTSRoute(
		t,
		"src/executor/fixtures/executor02/routes/price-route-simpleSwap-sushiv3-balancerv1-usdc-eth.json",
	)
	params := loadTSExchangeParams(
		t,
		"src/executor/fixtures/executor02/exchange-params/price-route-simpleSwap-sushiv3-balancerv1-usdc-eth.json",
	)
	return route, params
}

func loadExecutor03SimpleCase(t *testing.T) (executorRoute, []resolved.DexExchangeBuildParam) {
	t.Helper()

	route := loadTSRoute(
		t,
		"src/executor/fixtures/executor01/routes/price-route-simpleSwap-univ3-usdc-usdt.json",
	)
	params := loadTSExchangeParams(
		t,
		"src/executor/fixtures/executor01/exchange-params/price-route-simpleSwap-univ3-usdc-usdt.json",
	)
	route.BestRoute[0].Swaps[0].SwapExchanges =
		route.BestRoute[0].Swaps[0].SwapExchanges[:1]
	return route, params[:1]
}

func loadExecutor02NativeDestWethPlan(t *testing.T) *resolved.WethPlan {
	t.Helper()

	var wethPlan resolved.WethPlan
	readTSJSON(
		t,
		"src/executor/fixtures/executor02/maybe-weth-calldata/price-route-simpleSwap-sushiv3-balancerv1-usdc-eth.json",
		&wethPlan,
	)
	return &wethPlan
}

func loadTSRoute(t *testing.T, relPath string) executorRoute {
	t.Helper()

	var route tsOptimalRoute
	readTSJSON(t, relPath, &route)
	return executorRoute{
		SrcToken:   route.SrcToken,
		DestToken:  route.DestToken,
		DestAmount: route.DestAmount,
		BestRoute:  route.BestRoute,
	}
}

func loadTSExchangeParams(t *testing.T, relPath string) []resolved.DexExchangeBuildParam {
	t.Helper()

	var params []resolved.DexExchangeBuildParam
	readTSJSON(t, relPath, &params)
	return params
}

func readTSJSON(t *testing.T, relPath string, dest any) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(tsDexLibRoot, relPath))
	if err != nil {
		t.Fatalf("read TS fixture %s: %v", relPath, err)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		t.Fatalf("decode TS fixture %s: %v", relPath, err)
	}
}

func loadTSSnapshotObject(t *testing.T, exportName string) map[string]string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(
		tsDexLibRoot,
		"src/executor/__snapshots__/executor-missing-fixtures-snapshot.test.ts.snap",
	))
	if err != nil {
		t.Fatalf("read TS snapshot: %v", err)
	}

	marker := fmt.Sprintf("exports[`%s`] = `", exportName)
	start := strings.Index(string(raw), marker)
	if start == -1 {
		t.Fatalf("snapshot export %q not found", exportName)
	}
	blockStart := start + len(marker)
	blockEnd := strings.Index(string(raw)[blockStart:], "`;")
	if blockEnd == -1 {
		t.Fatalf("snapshot export %q is unterminated", exportName)
	}

	block := string(raw)[blockStart : blockStart+blockEnd]
	matches := regexp.MustCompile(`(?m)^\s+"([^"]+)": "([^"]+)"`).FindAllStringSubmatch(block, -1)
	values := make(map[string]string, len(matches))
	for _, match := range matches {
		values[match[1]] = match[2]
	}
	return values
}

func buildTSParityInput(
	priceRoute executorRoute,
	exchangeParams []resolved.DexExchangeBuildParam,
	wethPlan *resolved.WethPlan,
) resolved.ExecutorBytecodeBuildInput {
	input := testBytecodeBuildInput(priceRoute, exchangeParams)
	input.WethPlan = wethPlan
	return input
}

func tsMainnetEncodingContext() resolved.EncodingContext {
	return resolved.EncodingContext{
		Network:                   1,
		AugustusV6Address:         resolved.Address("0x6a000f20005980200259b80c5102003040001068"),
		WrappedNativeTokenAddress: tsMainnetWETH,
	}
}

func setFirstSwapSourceToken(route *executorRoute, token resolved.Address) {
	route.SrcToken = token
	route.BestRoute[0].Swaps[0].SrcToken = token
}

func setFirstSwapDestToken(route *executorRoute, token resolved.Address) {
	route.DestToken = token
	route.BestRoute[0].Swaps[0].DestToken = token
}
