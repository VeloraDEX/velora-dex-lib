package registry

import (
	"context"
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/builder"
	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

type testDexEncoder struct{}

func (testDexEncoder) NeedWrapNative(context.Context, builder.NeedWrapNativeInput) (bool, error) {
	return false, nil
}

func (testDexEncoder) GetDexParam(context.Context, builder.DexParamInput) (builder.DexExchangeParam, error) {
	return builder.DexExchangeParam{}, nil
}

type testDirectDexEncoder struct {
	methods []string
}

func (e testDirectDexEncoder) DirectContractMethodsV6() []string {
	return e.methods
}

func (testDirectDexEncoder) GetDirectParamV6(
	context.Context,
	builder.DirectParamInput,
) (builder.DirectParamResult, error) {
	return builder.DirectParamResult{}, nil
}

func TestRegistrySupportsGenericAndDirectEntries(t *testing.T) {
	registry := MustNew(
		Entry{
			Keys:    []string{"GenericOnly"},
			Encoder: testDexEncoder{},
		},
		Entry{
			Keys: []string{"DirectOnly"},
			DirectEncoder: testDirectDexEncoder{
				methods: []string{resolved.ContractMethodSwapExactAmountInOnUniswapV2},
			},
		},
		Entry{
			Keys:    []string{"Both"},
			Encoder: testDexEncoder{},
			DirectEncoder: testDirectDexEncoder{
				methods: []string{resolved.ContractMethodSwapExactAmountOutOnUniswapV2},
			},
		},
	)

	if _, err := registry.GetDexEncoder(context.Background(), 1, "GenericOnly"); err != nil {
		t.Fatalf("GetDexEncoder(GenericOnly) error = %v", err)
	}
	if _, err := registry.GetDirectDexEncoder(
		context.Background(),
		1,
		"DirectOnly",
		resolved.ContractMethodSwapExactAmountInOnUniswapV2,
	); err != nil {
		t.Fatalf("GetDirectDexEncoder(DirectOnly) error = %v", err)
	}
	if _, err := registry.GetDexEncoder(context.Background(), 1, "Both"); err != nil {
		t.Fatalf("GetDexEncoder(Both) error = %v", err)
	}
	if _, err := registry.GetDirectDexEncoder(
		context.Background(),
		1,
		"Both",
		resolved.ContractMethodSwapExactAmountOutOnUniswapV2,
	); err != nil {
		t.Fatalf("GetDirectDexEncoder(Both) error = %v", err)
	}
}

func TestRegistryPartialSupportErrorsAreExplicit(t *testing.T) {
	registry := MustNew(
		Entry{
			Keys:    []string{"GenericOnly"},
			Encoder: testDexEncoder{},
		},
		Entry{
			Keys: []string{"DirectOnly"},
			DirectEncoder: testDirectDexEncoder{
				methods: []string{resolved.ContractMethodSwapExactAmountInOnUniswapV2},
			},
		},
	)

	_, err := registry.GetDexEncoder(context.Background(), 1, "DirectOnly")
	assertErrorContains(t, err, "dex encoder not found")

	_, err = registry.GetDirectDexEncoder(
		context.Background(),
		1,
		"GenericOnly",
		resolved.ContractMethodSwapExactAmountInOnUniswapV2,
	)
	assertErrorContains(t, err, "direct dex encoder not found")

	_, err = registry.GetDirectDexEncoder(
		context.Background(),
		1,
		"DirectOnly",
		resolved.ContractMethodSwapExactAmountOutOnUniswapV2,
	)
	assertErrorContains(t, err, "does not support method")

	_, err = registry.GetDirectDexEncoder(context.Background(), 1, "DirectOnly", "notDirect")
	assertErrorContains(t, err, "unsupported V6 direct method")
}

func TestRegistryRejectsDuplicateKeysPerCapability(t *testing.T) {
	_, err := New(
		Entry{Keys: []string{"dup"}, Encoder: testDexEncoder{}},
		Entry{Keys: []string{"dup"}, Encoder: testDexEncoder{}},
	)
	assertErrorContains(t, err, "duplicate dex registry key")

	_, err = New(
		Entry{
			Keys: []string{"dup"},
			DirectEncoder: testDirectDexEncoder{
				methods: []string{resolved.ContractMethodSwapExactAmountInOnUniswapV2},
			},
		},
		Entry{
			Keys: []string{"dup"},
			DirectEncoder: testDirectDexEncoder{
				methods: []string{resolved.ContractMethodSwapExactAmountOutOnUniswapV2},
			},
		},
	)
	assertErrorContains(t, err, "duplicate direct dex registry key")

	if _, err = New(
		Entry{Keys: []string{"shared"}, Encoder: testDexEncoder{}},
		Entry{
			Keys: []string{"shared"},
			DirectEncoder: testDirectDexEncoder{
				methods: []string{resolved.ContractMethodSwapExactAmountInOnUniswapV2},
			},
		},
	); err != nil {
		t.Fatalf("New() should allow same key across separate capabilities, got %v", err)
	}
}

func TestRegistryRejectsEntryWithoutAnyEncoder(t *testing.T) {
	_, err := New(Entry{Keys: []string{"empty"}})
	assertErrorContains(t, err, "encoder or direct encoder is required")
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want containing %q", err.Error(), want)
	}
}
