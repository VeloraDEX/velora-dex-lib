package builder

import (
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

func TestResolveBeneficiaryForwardsCallerValue(t *testing.T) {
	userAddress := resolved.Address("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	explicit := resolved.Address("0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")
	sameAsUser := resolved.Address("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	null := resolved.NullAddress
	empty := resolved.Address("")

	testCases := []struct {
		name        string
		beneficiary *resolved.Address
		want        resolved.Address
	}{
		{"absent defaults to null address", nil, resolved.NullAddress},
		{"empty defaults to null address", &empty, resolved.NullAddress},
		{"explicit null address is kept", &null, resolved.NullAddress},
		{"explicit beneficiary is lowercased", &explicit, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		// The pre-fc9f9b92 builder collapsed this case to the null address.
		{"beneficiary equal to user address survives", &sameAsUser, userAddress},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertEqual(t, resolveBeneficiary(testCase.beneficiary), testCase.want, "beneficiary")
		})
	}
}
