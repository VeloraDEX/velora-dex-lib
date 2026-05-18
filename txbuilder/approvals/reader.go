package approvals

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const Permit2Address resolved.Address = "0x000000000022d473030f116ddee9f6b43ac78ba3"

type ContractCaller interface {
	CallContract(ctx context.Context, call ContractCall, blockNumber *big.Int) ([]byte, error)
}

type ContractCall struct {
	To   *common.Address
	Data []byte
}

type MulticallAllowanceReader struct {
	caller           ContractCaller
	multicallAddress common.Address
}

type multicall3Call struct {
	Target       common.Address
	AllowFailure bool
	CallData     []byte
}

type multicall3Result struct {
	Success    bool
	ReturnData []byte
}

func NewMulticallAllowanceReader(
	caller ContractCaller,
	multicallAddress resolved.Address,
) (*MulticallAllowanceReader, error) {
	if caller == nil {
		return nil, fmt.Errorf("contract caller is required")
	}
	if !common.IsHexAddress(string(multicallAddress)) {
		return nil, fmt.Errorf("multicall address must be a hex address: %s", multicallAddress)
	}

	return &MulticallAllowanceReader{
		caller:           caller,
		multicallAddress: common.HexToAddress(string(multicallAddress)),
	}, nil
}

func (r *MulticallAllowanceReader) HasAllowances(
	ctx context.Context,
	requests []AllowanceRequest,
) ([]bool, error) {
	if len(requests) == 0 {
		return []bool{}, nil
	}

	calls := make([]multicall3Call, len(requests))
	for index, request := range requests {
		call, err := buildAllowanceCall(request)
		if err != nil {
			return nil, err
		}
		calls[index] = call
	}

	callData, err := multicall3ABI.Pack("aggregate3", calls)
	if err != nil {
		return nil, err
	}

	returnData, err := r.caller.CallContract(ctx, ContractCall{
		To:   &r.multicallAddress,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, err
	}

	results, err := decodeAggregate3Result(returnData)
	if err != nil {
		return nil, err
	}
	if len(results) != len(requests) {
		return nil, fmt.Errorf("multicall result length must match request count")
	}

	out := make([]bool, len(requests))
	for index, result := range results {
		if !result.Success {
			out[index] = false
			continue
		}

		amount, err := decodeAllowanceAmount(requests[index], result.ReturnData)
		if err != nil {
			return nil, err
		}
		out[index] = amount.Sign() != 0
	}
	return out, nil
}

func buildAllowanceCall(request AllowanceRequest) (multicall3Call, error) {
	if !common.IsHexAddress(string(request.Spender)) {
		return multicall3Call{}, fmt.Errorf("spender must be a hex address: %s", request.Spender)
	}
	if !common.IsHexAddress(string(request.Token)) {
		return multicall3Call{}, fmt.Errorf("token must be a hex address: %s", request.Token)
	}
	if !common.IsHexAddress(string(request.Target)) {
		return multicall3Call{}, fmt.Errorf("target must be a hex address: %s", request.Target)
	}

	spender := common.HexToAddress(string(request.Spender))
	token := common.HexToAddress(string(request.Token))
	target := common.HexToAddress(string(request.Target))

	if request.Permit2 {
		callData, err := permit2AllowanceABI.Pack("allowance", spender, token, target)
		if err != nil {
			return multicall3Call{}, err
		}
		return multicall3Call{
			Target:       common.HexToAddress(string(Permit2Address)),
			AllowFailure: true,
			CallData:     callData,
		}, nil
	}

	callData, err := erc20AllowanceABI.Pack("allowance", spender, target)
	if err != nil {
		return multicall3Call{}, err
	}
	return multicall3Call{
		Target:       token,
		AllowFailure: true,
		CallData:     callData,
	}, nil
}

func decodeAggregate3Result(data []byte) ([]multicall3Result, error) {
	unpacked, err := multicall3ABI.Unpack("aggregate3", data)
	if err != nil {
		return nil, err
	}
	if len(unpacked) != 1 {
		return nil, fmt.Errorf("unexpected aggregate3 output count: %d", len(unpacked))
	}

	results, ok := unpacked[0].([]struct {
		Success    bool   `json:"success"`
		ReturnData []byte `json:"returnData"`
	})
	if ok {
		out := make([]multicall3Result, len(results))
		for index, result := range results {
			out[index] = multicall3Result{
				Success:    result.Success,
				ReturnData: result.ReturnData,
			}
		}
		return out, nil
	}

	typedResults, ok := unpacked[0].([]multicall3Result)
	if ok {
		return typedResults, nil
	}

	return nil, fmt.Errorf("unexpected aggregate3 output type: %T", unpacked[0])
}

func decodeAllowanceAmount(request AllowanceRequest, data []byte) (*big.Int, error) {
	if request.Permit2 {
		unpacked, err := permit2AllowanceABI.Unpack("allowance", data)
		if err != nil {
			return nil, err
		}
		return unpackBigInt(unpacked, "permit2 allowance amount")
	}

	unpacked, err := erc20AllowanceABI.Unpack("allowance", data)
	if err != nil {
		return nil, err
	}
	return unpackBigInt(unpacked, "erc20 allowance amount")
}

func unpackBigInt(unpacked []any, label string) (*big.Int, error) {
	if len(unpacked) == 0 {
		return nil, fmt.Errorf("%s missing", label)
	}
	amount, ok := unpacked[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("%s has unexpected type %T", label, unpacked[0])
	}
	return amount, nil
}

var erc20AllowanceABI = mustParseABI(`[
	{"type":"function","name":"allowance","inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"outputs":[{"name":"","type":"uint256"}]}
]`)

var permit2AllowanceABI = mustParseABI(`[
	{"type":"function","name":"allowance","inputs":[{"name":"owner","type":"address"},{"name":"token","type":"address"},{"name":"spender","type":"address"}],"outputs":[{"name":"amount","type":"uint160"},{"name":"expiration","type":"uint48"},{"name":"nonce","type":"uint48"}]}
]`)

var multicall3ABI = mustParseABI(`[
	{"type":"function","name":"aggregate3","inputs":[{"name":"calls","type":"tuple[]","components":[{"name":"target","type":"address"},{"name":"allowFailure","type":"bool"},{"name":"callData","type":"bytes"}]}],"outputs":[{"name":"returnData","type":"tuple[]","components":[{"name":"success","type":"bool"},{"name":"returnData","type":"bytes"}]}]}
]`)

func mustParseABI(raw string) ethabi.ABI {
	parsed, err := ethabi.JSON(strings.NewReader(raw))
	if err != nil {
		panic(err)
	}
	return parsed
}
