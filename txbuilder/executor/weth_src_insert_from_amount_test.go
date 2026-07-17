package executor

import (
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

func TestExecutor0102WETHSrcInsertFromAmountFlag(t *testing.T) {
	builders := []struct {
		name       string
		buildFlags func(executorRoute, []resolved.DexExchangeBuildParam) (flag, error)
	}{
		{
			name: "Executor01",
			buildFlags: func(priceRoute executorRoute, exchangeParams []resolved.DexExchangeBuildParam) (flag, error) {
				flags, err := NewExecutor01Builder(testEncodingContext()).buildFlags(priceRoute, exchangeParams, nil)
				if err != nil {
					return 0, err
				}
				return flags.dexes[0], nil
			},
		},
		{
			name: "Executor02",
			buildFlags: func(priceRoute executorRoute, exchangeParams []resolved.DexExchangeBuildParam) (flag, error) {
				flags, err := NewExecutor02Builder(testEncodingContext()).buildFlags(priceRoute, exchangeParams, nil)
				if err != nil {
					return 0, err
				}
				return flags.dexes[0], nil
			},
		},
	}

	cases := []struct {
		name           string
		supportsInsert bool
		want           flag
	}{
		{
			name:           "supports insert from amount",
			supportsInsert: true,
			want:           sendEthEqualToFromAmountPlusInsertFromAmountCheckSrcTokenBalanceAfterSwap,
		},
		{
			name:           "no insert support",
			supportsInsert: false,
			want:           sendEthEqualToFromAmountCheckSrcTokenBalanceAfterSwap,
		},
	}

	for _, b := range builders {
		t.Run(b.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					priceRoute, exchangeParams := testExecutorRouteAndParams(0)
					priceRoute.SrcToken = testWETH
					priceRoute.BestRoute[0].Swaps[0].SrcToken = testWETH
					exchangeParams[0].ReturnAmountPos = nil
					needUnwrapNative := true
					exchangeParams[0].NeedUnwrapNative = &needUnwrapNative
					if tc.supportsInsert {
						sendEthButInsert := true
						exchangeParams[0].SendEthButSupportsInsertFromAmount = &sendEthButInsert
					}

					got, err := b.buildFlags(priceRoute, exchangeParams)
					if err != nil {
						t.Fatalf("buildFlags() error = %v", err)
					}
					if got != tc.want {
						t.Fatalf("dex flag = %d, want %d", got, tc.want)
					}
				})
			}
		})
	}
}
