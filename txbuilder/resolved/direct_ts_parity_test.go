package resolved

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tsDirectFixtureDir = "paraswap-dex-lib/tests/generic-swap-transaction-builder/fixtures/resolved-build/direct"

type directTSFixture struct {
	Name           string           `json:"name"`
	Input          DirectBuildInput `json:"input"`
	ExpectedParams json.RawMessage  `json:"expectedParams"`
	ExpectedTx     directFixtureTx  `json:"expectedTx"`
}

type directFixtureTx struct {
	From                 Address       `json:"from"`
	To                   Address       `json:"to"`
	Value                DecimalString `json:"value"`
	Data                 HexBytes      `json:"data"`
	GasPrice             DecimalString `json:"gasPrice,omitempty"`
	MaxFeePerGas         DecimalString `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas DecimalString `json:"maxPriorityFeePerGas,omitempty"`
}

func TestDirectBuildMatchesTSResolvedFixtures(t *testing.T) {
	entries, err := os.ReadDir(tsDirectFixtureDir)
	if err != nil {
		t.Skipf("TS direct resolved fixtures unavailable at %s: %v", tsDirectFixtureDir, err)
	}

	abi := MustLoadAugustusV6ABI()
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		t.Run(strings.TrimSuffix(entry.Name(), ".json"), func(t *testing.T) {
			fixture := loadDirectTSFixture(t, entry.Name())
			got, err := BuildDirectTransactionFromResolved(fixture.Input, DirectBuildDeps{
				AugustusV6ABI: abi,
			})
			if err != nil {
				t.Fatalf("BuildDirectTransactionFromResolved() error = %v", err)
			}

			assertJSONEqual(t, got.Params, fixture.ExpectedParams, "params")
			assertDirectTxMatchesFixture(t, got.TxObject, fixture.ExpectedTx)
		})
	}
}

func loadDirectTSFixture(t *testing.T, name string) directTSFixture {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(tsDirectFixtureDir, name))
	if err != nil {
		t.Fatalf("read TS direct fixture %s: %v", name, err)
	}

	var fixture directTSFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode TS direct fixture %s: %v", name, err)
	}
	if len(fixture.Input.Params) == 0 {
		t.Fatalf("TS direct fixture %s has empty input.params", name)
	}
	return fixture
}

func assertDirectTxMatchesFixture(t *testing.T, got TxObject, want directFixtureTx) {
	t.Helper()

	if got.From != want.From {
		t.Fatalf("tx.from mismatch\nwant: %s\n got: %s", want.From, got.From)
	}
	if got.To != want.To {
		t.Fatalf("tx.to mismatch\nwant: %s\n got: %s", want.To, got.To)
	}
	if got.Value != want.Value {
		t.Fatalf("tx.value mismatch\nwant: %s\n got: %s", want.Value, got.Value)
	}
	if !strings.EqualFold(string(got.Data), string(want.Data)) {
		t.Fatalf("tx.data mismatch\nwant: %s\n got: %s", want.Data, got.Data)
	}
	if got.GasPrice != want.GasPrice {
		t.Fatalf("tx.gasPrice mismatch\nwant: %s\n got: %s", want.GasPrice, got.GasPrice)
	}
	if got.MaxFeePerGas != want.MaxFeePerGas {
		t.Fatalf("tx.maxFeePerGas mismatch\nwant: %s\n got: %s", want.MaxFeePerGas, got.MaxFeePerGas)
	}
	if got.MaxPriorityFeePerGas != want.MaxPriorityFeePerGas {
		t.Fatalf("tx.maxPriorityFeePerGas mismatch\nwant: %s\n got: %s", want.MaxPriorityFeePerGas, got.MaxPriorityFeePerGas)
	}
}

func assertJSONEqual(t *testing.T, got any, wantRaw json.RawMessage, field string) {
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
