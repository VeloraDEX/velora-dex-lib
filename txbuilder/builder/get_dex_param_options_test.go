package builder

import (
	"context"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

type recordingDexRegistry struct {
	encoder *recordingDexEncoder
}

func (r recordingDexRegistry) GetDexEncoder(_ context.Context, _ int, _ string) (DexEncoder, error) {
	return r.encoder, nil
}

type recordingDexEncoder struct {
	got DexParamInput
}

func (r *recordingDexEncoder) NeedWrapNative(context.Context, NeedWrapNativeInput) (bool, error) {
	return true, nil
}

func (r *recordingDexEncoder) GetDexParam(_ context.Context, input DexParamInput) (DexExchangeParam, error) {
	r.got = input
	return DexExchangeParam{
		NeedWrapNative:      true,
		ExchangeData:        "0x12345678",
		TargetExchange:      testApprovalTargetExchange,
		DexFuncHasRecipient: true,
	}, nil
}

func TestBuildResolvedLegs_ForwardsGetDexParamOptions(t *testing.T) {
	priceRoute, routePlan, _ := testApprovalRouteAndLeg()
	nowTimestampMs := uint64(1_700_000_123_456)
	options := &GetDexParamOptions{NowTimestampMs: &nowTimestampMs}
	recorder := &recordingDexEncoder{}

	_, err := buildResolvedLegs(
		context.Background(),
		BuildRequest{
			PriceRoute:         priceRoute,
			MinMaxAmount:       "1",
			GetDexParamOptions: options,
		},
		Deps{
			DexRegistry: recordingDexRegistry{encoder: recorder},
		},
		resolved.EncodingContext{
			AugustusV6Address:         resolved.NullAddress,
			WrappedNativeTokenAddress: testApprovalWETH,
		},
		routePlan,
		testApprovalTargetExchange,
	)
	if err != nil {
		t.Fatalf("buildResolvedLegs() error = %v", err)
	}
	if recorder.got.Options != options {
		t.Fatalf("DexParamInput.Options = %p, want %p", recorder.got.Options, options)
	}
	if recorder.got.Options.NowTimestampMs == nil || *recorder.got.Options.NowTimestampMs != nowTimestampMs {
		t.Fatalf("NowTimestampMs not preserved: %+v", recorder.got.Options)
	}
}
