package approvals

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"reflect"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
	"github.com/ethereum/go-ethereum/common"
)

func TestBuildAllowanceCallERC20(t *testing.T) {
	call, err := buildAllowanceCall(AllowanceRequest{
		Spender: testSpender,
		Token:   testTokenA,
		Target:  testTarget,
	})
	if err != nil {
		t.Fatal(err)
	}

	if call.Target != common.HexToAddress(string(testTokenA)) {
		t.Fatalf("target = %s, want token %s", call.Target.Hex(), testTokenA)
	}
	if !call.AllowFailure {
		t.Fatal("AllowFailure = false, want true")
	}

	want := "dd62ed3e" +
		"0000000000000000000000001111111111111111111111111111111111111111" +
		"0000000000000000000000002222222222222222222222222222222222222222"
	if got := hex.EncodeToString(call.CallData); got != want {
		t.Fatalf("call data = %s, want %s", got, want)
	}
}

func TestBuildAllowanceCallPermit2ArgumentOrder(t *testing.T) {
	call, err := buildAllowanceCall(AllowanceRequest{
		Spender: testSpender,
		Token:   testTokenA,
		Target:  testTarget,
		Permit2: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if call.Target != common.HexToAddress(string(Permit2Address)) {
		t.Fatalf("target = %s, want Permit2 %s", call.Target.Hex(), Permit2Address)
	}
	if !call.AllowFailure {
		t.Fatal("AllowFailure = false, want true")
	}

	method := permit2AllowanceABI.Methods["allowance"]
	values, err := method.Inputs.Unpack(call.CallData[4:])
	if err != nil {
		t.Fatal(err)
	}
	want := []any{
		common.HexToAddress(string(testSpender)),
		common.HexToAddress(string(testTokenA)),
		common.HexToAddress(string(testTarget)),
	}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("permit2 args = %#v, want %#v", values, want)
	}
}

func TestMulticallAllowanceReaderHasAllowances(t *testing.T) {
	erc20Allowance, err := erc20AllowanceABI.Methods["allowance"].Outputs.Pack(big.NewInt(7))
	if err != nil {
		t.Fatal(err)
	}
	permit2ZeroAllowance, err := permit2AllowanceABI.Methods["allowance"].Outputs.Pack(
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(2),
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregateOutput := packAggregate3Results(t, []multicall3Result{
		{Success: true, ReturnData: erc20Allowance},
		{Success: false, ReturnData: nil},
		{Success: true, ReturnData: permit2ZeroAllowance},
	})

	caller := &fakeContractCaller{returnData: aggregateOutput}
	reader := newTestMulticallReader(t, caller)

	got, err := reader.HasAllowances(context.Background(), []AllowanceRequest{
		{Spender: testSpender, Token: testTokenA, Target: testTarget},
		{Spender: testSpender, Token: testTokenB, Target: testTarget},
		{Spender: testSpender, Token: testTokenA, Target: testTarget, Permit2: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []bool{true, false, false}) {
		t.Fatalf("result = %v, want [true false false]", got)
	}
	if caller.calls != 1 {
		t.Fatalf("caller calls = %d, want 1", caller.calls)
	}
	if caller.call.To == nil || *caller.call.To != common.HexToAddress(string(testMulticall)) {
		t.Fatalf("call target = %v, want %s", caller.call.To, testMulticall)
	}
}

func TestMulticallAllowanceReaderErrors(t *testing.T) {
	transportErr := errors.New("transport")
	reader := newTestMulticallReader(t, &fakeContractCaller{err: transportErr})
	_, err := reader.HasAllowances(context.Background(), []AllowanceRequest{
		{Spender: testSpender, Token: testTokenA, Target: testTarget},
	})
	if !errors.Is(err, transportErr) {
		t.Fatalf("error = %v, want %v", err, transportErr)
	}

	reader = newTestMulticallReader(t, &fakeContractCaller{returnData: []byte{0x01}})
	_, err = reader.HasAllowances(context.Background(), []AllowanceRequest{
		{Spender: testSpender, Token: testTokenA, Target: testTarget},
	})
	if err == nil {
		t.Fatal("expected aggregate decode error")
	}

	badSubcallOutput := packAggregate3Results(t, []multicall3Result{
		{Success: true, ReturnData: []byte{0x01}},
	})
	reader = newTestMulticallReader(t, &fakeContractCaller{returnData: badSubcallOutput})
	_, err = reader.HasAllowances(context.Background(), []AllowanceRequest{
		{Spender: testSpender, Token: testTokenA, Target: testTarget},
	})
	if err == nil {
		t.Fatal("expected subcall decode error")
	}
}

func packAggregate3Results(t *testing.T, results []multicall3Result) []byte {
	t.Helper()
	data, err := multicall3ABI.Methods["aggregate3"].Outputs.Pack(results)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newTestMulticallReader(t *testing.T, caller ContractCaller) *MulticallAllowanceReader {
	t.Helper()
	reader, err := NewMulticallAllowanceReader(caller, testMulticall)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

type fakeContractCaller struct {
	returnData []byte
	err        error
	calls      int
	call       ContractCall
}

func (c *fakeContractCaller) CallContract(
	ctx context.Context,
	call ContractCall,
	blockNumber *big.Int,
) ([]byte, error) {
	c.calls++
	c.call = call
	if c.err != nil {
		return nil, c.err
	}
	return append([]byte(nil), c.returnData...), nil
}

const testMulticall resolved.Address = "0x3333333333333333333333333333333333333333"
