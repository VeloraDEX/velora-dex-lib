package builder

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const tsDirectBuilderFixtureDir = "paraswap-dex-lib/tests/generic-swap-transaction-builder/fixtures/resolved-build/direct"

type directBuilderTSFixture struct {
	Name           string                     `json:"name"`
	Input          resolved.DirectBuildInput  `json:"input"`
	ExpectedParams json.RawMessage            `json:"expectedParams"`
	ExpectedTx     directBuilderFixtureTx     `json:"expectedTx"`
	Orchestration  directBuilderOrchestration `json:"orchestration"`
}

type directBuilderFixtureTx struct {
	From                 resolved.Address       `json:"from"`
	To                   resolved.Address       `json:"to"`
	Value                resolved.DecimalString `json:"value"`
	Data                 resolved.HexBytes      `json:"data"`
	GasPrice             resolved.DecimalString `json:"gasPrice,omitempty"`
	MaxFeePerGas         resolved.DecimalString `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas resolved.DecimalString `json:"maxPriorityFeePerGas,omitempty"`
}

type directBuilderOrchestration struct {
	DirectDexKey string                 `json:"directDexKey"`
	MinMaxAmount resolved.DecimalString `json:"minMaxAmount"`
	PriceRoute   PriceRoute             `json:"priceRoute"`
	QuotedAmount resolved.DecimalString `json:"quotedAmount"`
}

type recordingDirectDexRegistry struct {
	encoder *recordingDirectDexEncoder
	lookup  directDexLookup
}

type directDexLookup struct {
	network        int
	dexKey         string
	contractMethod string
}

func (r *recordingDirectDexRegistry) GetDirectDexEncoder(
	_ context.Context,
	network int,
	dexKey string,
	contractMethod string,
) (DirectDexEncoder, error) {
	r.lookup = directDexLookup{
		network:        network,
		dexKey:         dexKey,
		contractMethod: contractMethod,
	}
	if r.encoder == nil {
		return nil, nil
	}
	return r.encoder, nil
}

type recordingDirectDexEncoder struct {
	method string
	params json.RawMessage
	got    DirectParamInput
}

func (e *recordingDirectDexEncoder) DirectContractMethodsV6() []string {
	return []string{e.method}
}

func (e *recordingDirectDexEncoder) GetDirectParamV6(
	_ context.Context,
	input DirectParamInput,
) (DirectParamResult, error) {
	e.got = input
	return DirectParamResult{Params: e.params}, nil
}

type panicApprovalChecker struct{}

func (panicApprovalChecker) Check(
	context.Context,
	resolved.Address,
	[]ApprovalRequest,
) ([]bool, error) {
	panic("BuildDirect must not call ApprovalChecker")
}

func TestBuildDirectMatchesTSFixtures(t *testing.T) {
	fixtureDir := resolveDirectBuilderFixtureDir(t)
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Skipf("TS direct resolved fixtures unavailable at %s: %v", fixtureDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		t.Run(strings.TrimSuffix(entry.Name(), ".json"), func(t *testing.T) {
			fixture := loadDirectBuilderTSFixture(t, fixtureDir, entry.Name())
			encoder := &recordingDirectDexEncoder{
				method: fixture.Orchestration.PriceRoute.ContractMethod,
				params: fixture.ExpectedParams,
			}
			registry := &recordingDirectDexRegistry{encoder: encoder}

			req := directBuildRequestFromFixture(fixture)
			got, err := BuildDirect(context.Background(), req, Deps{
				EncodingContext: resolved.EncodingContext{
					Network:           fixture.Orchestration.PriceRoute.Network,
					AugustusV6Address: fixture.Input.AugustusV6Address,
				},
				AugustusV6ABI:     resolved.MustLoadAugustusV6ABI(),
				DirectDexRegistry: registry,
				ApprovalChecker:   panicApprovalChecker{},
			})
			if err != nil {
				t.Fatalf("BuildDirect() error = %v", err)
			}

			assertDirectLookupMatchesFixture(t, registry.lookup, fixture)
			assertDirectParamInputMatchesFixture(t, encoder.got, fixture, req)
			assertBuilderJSONEqual(t, got.Params, fixture.ExpectedParams, "params")
			assertBuilderDirectTxMatchesFixture(t, got.TxObject, fixture.ExpectedTx)
			if got.TxObject.ChainID != fixture.Orchestration.PriceRoute.Network {
				t.Fatalf("tx.chainId = %d, want %d", got.TxObject.ChainID, fixture.Orchestration.PriceRoute.Network)
			}
		})
	}
}

func TestBuildDirectValidationErrors(t *testing.T) {
	validReq := BuildRequest{
		PriceRoute: PriceRoute{
			Network:        1,
			BlockNumber:    1,
			ContractMethod: resolved.ContractMethodSwapExactAmountInOnUniswapV2,
			Side:           resolved.SideSell,
			SrcToken:       resolved.NativeTokenAddress,
			DestToken:      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SrcAmount:      "1000",
			DestAmount:     "995",
			BestRoute: []PriceRouteRoute{
				{
					Percent: 100,
					Swaps: []PriceRouteSwap{
						{
							SrcToken:  resolved.NativeTokenAddress,
							DestToken: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
							SwapExchanges: []PriceRouteSwapExchange{
								{
									Exchange:   "UniswapV2",
									Percent:    100,
									SrcAmount:  "1000",
									DestAmount: "995",
								},
							},
						},
					},
				},
			},
		},
		MinMaxAmount:      "990",
		UserAddress:       "0x1111111111111111111111111111111111111111",
		PartnerFeePercent: "0",
	}
	validDeps := Deps{
		EncodingContext: resolved.EncodingContext{
			Network:           1,
			AugustusV6Address: "0x6a000f20005980200259b80c5102003040001068",
		},
		AugustusV6ABI: resolved.MustLoadAugustusV6ABI(),
		DirectDexRegistry: &recordingDirectDexRegistry{
			encoder: &recordingDirectDexEncoder{
				method: resolved.ContractMethodSwapExactAmountInOnUniswapV2,
				params: json.RawMessage(`[
					[
						"0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
						"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						"1000",
						"990",
						"995",
						"0x1111111111111111111111111111111111111111111111111111111111111111",
						"0x0000000000000000000000000000000000000000",
						"0x"
					],
					"0",
					"0x"
				]`),
			},
		},
	}

	_, err := BuildDirect(context.Background(), validReq, Deps{
		EncodingContext: validDeps.EncodingContext,
		AugustusV6ABI:   validDeps.AugustusV6ABI,
	})
	assertBuilderErrorContains(t, err, "direct dex registry is required")

	unsupportedReq := validReq
	unsupportedReq.PriceRoute.ContractMethod = resolved.ContractMethodSwapExactAmountIn
	_, err = BuildDirect(context.Background(), unsupportedReq, validDeps)
	assertBuilderErrorContains(t, err, "unsupported direct contract method")

	invalidRouteReq := validReq
	invalidRouteReq.PriceRoute.BestRoute = nil
	_, err = BuildDirect(context.Background(), invalidRouteReq, validDeps)
	assertBuilderErrorContains(t, err, "DirectSwap invalid bestRoute")

	emptyDexKeyReq := validReq
	emptyDexKeyReq.PriceRoute.BestRoute[0].Swaps[0].SwapExchanges[0].Exchange = ""
	_, err = BuildDirect(context.Background(), emptyDexKeyReq, validDeps)
	assertBuilderErrorContains(t, err, "direct dex key is required")
	validReq.PriceRoute.BestRoute[0].Swaps[0].SwapExchanges[0].Exchange = "UniswapV2"

	nilEncoderDeps := validDeps
	nilEncoderDeps.DirectDexRegistry = &recordingDirectDexRegistry{}
	_, err = BuildDirect(context.Background(), validReq, nilEncoderDeps)
	assertBuilderErrorContains(t, err, "direct dex encoder is required")
}

func TestBuildDirectForwardsExplicitPermitAndBeneficiary(t *testing.T) {
	fixture := loadDirectBuilderTSFixture(t, resolveDirectBuilderFixtureDir(t), "uniswap-v2-sell.json")
	encoder := &recordingDirectDexEncoder{
		method: fixture.Orchestration.PriceRoute.ContractMethod,
		params: fixture.ExpectedParams,
	}
	registry := &recordingDirectDexRegistry{encoder: encoder}

	req := directBuildRequestFromFixture(fixture)
	permit := resolved.HexBytes("0xABCD")
	beneficiary := resolved.Address("0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")
	req.Permit = &permit
	req.Beneficiary = &beneficiary

	_, err := BuildDirect(context.Background(), req, Deps{
		EncodingContext: resolved.EncodingContext{
			Network:           fixture.Orchestration.PriceRoute.Network,
			AugustusV6Address: fixture.Input.AugustusV6Address,
		},
		AugustusV6ABI:     resolved.MustLoadAugustusV6ABI(),
		DirectDexRegistry: registry,
	})
	if err != nil {
		t.Fatalf("BuildDirect() error = %v", err)
	}

	assertEqual(t, encoder.got.Permit, resolved.HexBytes("0xabcd"), "direct input permit")
	assertEqual(
		t,
		encoder.got.Beneficiary,
		resolved.Address("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		"direct input beneficiary",
	)
}

func TestBuildDirectForwardsUserBeneficiaryVerbatim(t *testing.T) {
	fixture := loadDirectBuilderTSFixture(t, resolveDirectBuilderFixtureDir(t), "uniswap-v2-sell.json")
	encoder := &recordingDirectDexEncoder{
		method: fixture.Orchestration.PriceRoute.ContractMethod,
		params: fixture.ExpectedParams,
	}
	registry := &recordingDirectDexRegistry{encoder: encoder}

	req := directBuildRequestFromFixture(fixture)
	beneficiary := resolved.Address(strings.ToUpper(string(req.UserAddress)))
	req.Beneficiary = &beneficiary

	_, err := BuildDirect(context.Background(), req, Deps{
		EncodingContext: resolved.EncodingContext{
			Network:           fixture.Orchestration.PriceRoute.Network,
			AugustusV6Address: fixture.Input.AugustusV6Address,
		},
		AugustusV6ABI:     resolved.MustLoadAugustusV6ABI(),
		DirectDexRegistry: registry,
	})
	if err != nil {
		t.Fatalf("BuildDirect() error = %v", err)
	}

	assertEqual(
		t,
		encoder.got.Beneficiary,
		normalizeAddress(req.UserAddress),
		"direct input beneficiary",
	)
}

func directBuildRequestFromFixture(fixture directBuilderTSFixture) BuildRequest {
	req := BuildRequest{
		PriceRoute:        fixture.Orchestration.PriceRoute,
		MinMaxAmount:      fixture.Orchestration.MinMaxAmount,
		QuotedAmount:      ptr(fixture.Orchestration.QuotedAmount),
		UserAddress:       fixture.Input.UserAddress,
		PartnerFeePercent: "0",
		UUID:              "direct-fixture-uuid",
	}
	if fixture.Input.Gas != nil {
		if fixture.Input.Gas.GasPrice != "" {
			req.GasPrice = ptr(fixture.Input.Gas.GasPrice)
		}
		if fixture.Input.Gas.MaxFeePerGas != "" {
			req.MaxFeePerGas = ptr(fixture.Input.Gas.MaxFeePerGas)
		}
		if fixture.Input.Gas.MaxPriorityFeePerGas != "" {
			req.MaxPriorityFeePerGas = ptr(fixture.Input.Gas.MaxPriorityFeePerGas)
		}
	}
	return req
}

func assertDirectLookupMatchesFixture(
	t *testing.T,
	got directDexLookup,
	fixture directBuilderTSFixture,
) {
	t.Helper()

	if got.network != fixture.Orchestration.PriceRoute.Network {
		t.Fatalf("lookup.network = %d, want %d", got.network, fixture.Orchestration.PriceRoute.Network)
	}
	if got.dexKey != fixture.Orchestration.DirectDexKey {
		t.Fatalf("lookup.dexKey = %s, want %s", got.dexKey, fixture.Orchestration.DirectDexKey)
	}
	if got.contractMethod != fixture.Orchestration.PriceRoute.ContractMethod {
		t.Fatalf(
			"lookup.contractMethod = %s, want %s",
			got.contractMethod,
			fixture.Orchestration.PriceRoute.ContractMethod,
		)
	}
}

func assertDirectParamInputMatchesFixture(
	t *testing.T,
	got DirectParamInput,
	fixture directBuilderTSFixture,
	req BuildRequest,
) {
	t.Helper()

	swapExchange := fixture.Orchestration.PriceRoute.BestRoute[0].Swaps[0].SwapExchanges[0]
	wantSrcAmount := swapExchange.SrcAmount
	wantDestAmount := fixture.Orchestration.MinMaxAmount
	if fixture.Orchestration.PriceRoute.Side == resolved.SideBuy {
		wantSrcAmount = fixture.Orchestration.MinMaxAmount
		wantDestAmount = swapExchange.DestAmount
	}

	assertEqual(t, got.DexKey, fixture.Orchestration.DirectDexKey, "direct input dexKey")
	assertEqual(t, got.Network, fixture.Orchestration.PriceRoute.Network, "direct input network")
	assertEqual(t, got.ContractMethod, fixture.Orchestration.PriceRoute.ContractMethod, "direct input contractMethod")
	assertEqual(t, got.SrcToken, fixture.Orchestration.PriceRoute.SrcToken, "direct input srcToken")
	assertEqual(t, got.DestToken, fixture.Orchestration.PriceRoute.DestToken, "direct input destToken")
	assertEqual(t, got.SrcAmount, wantSrcAmount, "direct input srcAmount")
	assertEqual(t, got.DestAmount, wantDestAmount, "direct input destAmount")
	assertEqual(t, got.QuotedAmount, fixture.Orchestration.QuotedAmount, "direct input quotedAmount")
	assertBuilderJSONEqual(t, got.Data, swapExchange.Data, "direct input data")
	assertEqual(t, got.Side, fixture.Orchestration.PriceRoute.Side, "direct input side")
	assertEqual(t, got.Permit, resolved.HexBytes("0x"), "direct input permit")
	assertEqual(t, got.UUID, req.UUID, "direct input uuid")
	partnerAndFee, err := resolved.BuildFeesV6(resolved.FeeInput{
		PartnerAddress:      req.PartnerAddress,
		PartnerFeePercent:   req.PartnerFeePercent,
		ReferrerAddress:     req.ReferrerAddress,
		TakeSurplus:         req.TakeSurplus,
		IsCapSurplus:        resolveIsCapSurplus(req.IsCapSurplus),
		IsSurplusToUser:     req.IsSurplusToUser,
		IsDirectFeeTransfer: req.IsDirectFeeTransfer,
	})
	if err != nil {
		t.Fatalf("build expected partnerAndFee: %v", err)
	}
	assertEqual(t, got.PartnerAndFee, resolved.DecimalString(partnerAndFee.String()), "direct input partnerAndFee")
	assertEqual(t, got.Beneficiary, resolved.NullAddress, "direct input beneficiary")
	assertEqual(t, got.BlockNumber, fixture.Orchestration.PriceRoute.BlockNumber, "direct input blockNumber")
}

func resolveDirectBuilderFixtureDir(t *testing.T) string {
	t.Helper()

	candidates := []string{
		tsDirectBuilderFixtureDir,
		filepath.Join("..", "..", "..", "..", tsDirectBuilderFixtureDir),
	}
	for _, candidate := range candidates {
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate
		}
	}
	return tsDirectBuilderFixtureDir
}

func loadDirectBuilderTSFixture(t *testing.T, dir string, name string) directBuilderTSFixture {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read TS direct fixture %s: %v", name, err)
	}

	var fixture directBuilderTSFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode TS direct fixture %s: %v", name, err)
	}
	if fixture.Orchestration.DirectDexKey == "" {
		t.Fatalf("TS direct fixture %s has empty orchestration.directDexKey", name)
	}
	return fixture
}

func assertBuilderDirectTxMatchesFixture(t *testing.T, got resolved.TxObject, want directBuilderFixtureTx) {
	t.Helper()

	assertEqual(t, got.From, want.From, "tx.from")
	assertEqual(t, got.To, want.To, "tx.to")
	assertEqual(t, got.Value, want.Value, "tx.value")
	if !strings.EqualFold(string(got.Data), string(want.Data)) {
		t.Fatalf("tx.data mismatch\nwant: %s\n got: %s", want.Data, got.Data)
	}
	assertEqual(t, got.GasPrice, want.GasPrice, "tx.gasPrice")
	assertEqual(t, got.MaxFeePerGas, want.MaxFeePerGas, "tx.maxFeePerGas")
	assertEqual(t, got.MaxPriorityFeePerGas, want.MaxPriorityFeePerGas, "tx.maxPriorityFeePerGas")
}

func assertBuilderJSONEqual(t *testing.T, got any, wantRaw json.RawMessage, field string) {
	t.Helper()

	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got %s: %v", field, err)
	}
	var gotDecoded any
	if err := json.Unmarshal(gotRaw, &gotDecoded); err != nil {
		t.Fatalf("decode got %s: %v", field, err)
	}
	var wantDecoded any
	if err := json.Unmarshal(wantRaw, &wantDecoded); err != nil {
		t.Fatalf("decode want %s: %v", field, err)
	}

	gotCanonical, err := json.Marshal(gotDecoded)
	if err != nil {
		t.Fatalf("canonical marshal got %s: %v", field, err)
	}
	wantCanonical, err := json.Marshal(wantDecoded)
	if err != nil {
		t.Fatalf("canonical marshal want %s: %v", field, err)
	}
	if string(gotCanonical) != string(wantCanonical) {
		t.Fatalf("%s mismatch\nwant: %s\n got: %s", field, wantCanonical, gotCanonical)
	}
}

func assertEqual[T comparable](t *testing.T, got T, want T, field string) {
	t.Helper()

	if got != want {
		t.Fatalf("%s = %v, want %v", field, got, want)
	}
}

func assertBuilderErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want containing %q", err.Error(), want)
	}
}
