package builder

import (
	"encoding/json"
	"testing"
)

// The persisted build record is a verbatim marshal of BuildRequest, so the
// fields pricing attaches to a firm-quote leg must survive a decode/encode
// round trip — an undeclared field is a field the record loses.
func TestPriceRouteSwapExchangeKeepsFallbackAndTarget(t *testing.T) {
	in := []byte(`{
		"exchange": "Metric",
		"percent": 100,
		"srcAmount": "2000",
		"destAmount": "1998",
		"poolIdentifiers": ["0xrfq"],
		"targetExchange": "0xswaprouter",
		"fallback": {
			"exchange": "UniswapV3",
			"percent": 100,
			"srcAmount": "2000",
			"destAmount": "1900",
			"poolIdentifiers": ["0xpool"],
			"targetExchange": "0xrouter",
			"data": {"path": []}
		}
	}`)

	var se PriceRouteSwapExchange
	if err := json.Unmarshal(in, &se); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if se.TargetExchange != "0xswaprouter" {
		t.Errorf("targetExchange: got %q", se.TargetExchange)
	}
	if se.Fallback == nil {
		t.Fatal("fallback was dropped at decode")
	}
	if se.Fallback.Exchange != "UniswapV3" || se.Fallback.DestAmount != "1900" {
		t.Errorf("fallback content: got %+v", se.Fallback)
	}
	if se.Fallback.TargetExchange != "0xrouter" {
		t.Errorf("fallback targetExchange: got %q", se.Fallback.TargetExchange)
	}

	out, err := json.Marshal(se)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(out, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["fallback"]; !ok {
		t.Error("fallback missing from the re-encoded record")
	}
	if fields["targetExchange"] != "0xswaprouter" {
		t.Errorf("re-encoded targetExchange: got %v", fields["targetExchange"])
	}

	bare, err := json.Marshal(PriceRouteSwapExchange{Exchange: "dexA"})
	if err != nil {
		t.Fatal(err)
	}
	fields = map[string]interface{}{}
	if err := json.Unmarshal(bare, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["fallback"]; ok {
		t.Error("fallback must be omitted when absent")
	}
	if _, ok := fields["targetExchange"]; ok {
		t.Error("targetExchange must be omitted when absent")
	}
}
