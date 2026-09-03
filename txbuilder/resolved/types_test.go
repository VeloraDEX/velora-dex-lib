package resolved

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTxObject_EmitsLegacyKeyOrder pins the JSON key order on the
// wire. Some legacy clients read the response as an ordered map and
// would break if a struct-field reorder shuffled keys. encoding/json
// emits struct fields in declaration order, so we capture the order
// here and assert it directly.
func TestTxObject_EmitsLegacyKeyOrder(t *testing.T) {
	tx := TxObject{
		From:                 "0xaaaa",
		To:                   "0xbbbb",
		Value:                "0",
		Data:                 "0xdead",
		ChainID:              1,
		GasPrice:             "30000000000",
		MaxFeePerGas:         "50000000000",
		MaxPriorityFeePerGas: "1000000000",
		Gas:                  "210000",
	}
	b, err := json.Marshal(tx)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)

	expectedOrder := []string{
		`"from":`,
		`"to":`,
		`"value":`,
		`"data":`,
		`"chainId":`,
		`"gasPrice":`,
		`"maxFeePerGas":`,
		`"maxPriorityFeePerGas":`,
		`"gas":`,
	}
	prev := -1
	for _, key := range expectedOrder {
		idx := strings.Index(got, key)
		if idx < 0 {
			t.Fatalf("expected key %q in JSON: %s", key, got)
		}
		if idx <= prev {
			t.Fatalf("key %q appeared out of legacy order at offset %d (previous %d) in JSON: %s",
				key, idx, prev, got)
		}
		prev = idx
	}
}

// TestTxObject_EmptyGasFieldsOmitted pins that the optional gas
// fields are omitted from the JSON output when empty, while the
// required fields stay (legacy clients expect from/to/value/data/
// chainId present even at zero value).
func TestTxObject_EmptyGasFieldsOmitted(t *testing.T) {
	tx := TxObject{
		From:    "0xaaaa",
		To:      "0xbbbb",
		Value:   "0",
		Data:    "0x",
		ChainID: 1,
	}
	b, err := json.Marshal(tx)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"gasPrice", "maxFeePerGas", "maxPriorityFeePerGas"} {
		if _, ok := raw[key]; ok {
			t.Fatalf("empty %s must be omitted from JSON", key)
		}
	}
	for _, key := range []string{"from", "to", "value", "data", "chainId"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("required field %q must be present", key)
		}
	}
}
