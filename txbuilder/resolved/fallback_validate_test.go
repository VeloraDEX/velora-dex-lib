package resolved

import (
	"strings"
	"testing"
)

func validFallbackExchangeParam() DexExchangeBuildParam {
	return DexExchangeBuildParam{
		NeedWrapNative:      RawBool{Value: true, Valid: true, Present: true},
		ExchangeData:        "0x12345678",
		TargetExchange:      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DexFuncHasRecipient: true,
	}
}

func TestValidateExchangeParamValidatesFallback(t *testing.T) {
	param := validFallbackExchangeParam()
	fallback := validFallbackExchangeParam()
	fallback.TargetExchange = "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB" // not lowercase
	param.FallbackParam = &fallback

	err := validateExchangeParam(param, "legs[0]")
	if err == nil || !strings.Contains(err.Error(), "fallbackParam") {
		t.Fatalf("expected a fallbackParam validation error, got %v", err)
	}
}

func TestValidateExchangeParamRejectsNestedFallback(t *testing.T) {
	param := validFallbackExchangeParam()
	fallback := validFallbackExchangeParam()
	nested := validFallbackExchangeParam()
	fallback.FallbackParam = &nested
	param.FallbackParam = &fallback

	err := validateExchangeParam(param, "legs[0]")
	if err == nil || !strings.Contains(err.Error(), "must not nest") {
		t.Fatalf("expected a nesting rejection, got %v", err)
	}
}

func TestValidateExchangeParamAcceptsValidFallback(t *testing.T) {
	param := validFallbackExchangeParam()
	fallback := validFallbackExchangeParam()
	param.FallbackParam = &fallback

	if err := validateExchangeParam(param, "legs[0]"); err != nil {
		t.Fatalf("valid fallback rejected: %v", err)
	}
}
