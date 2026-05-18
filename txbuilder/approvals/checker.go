package approvals

import (
	"context"
	"fmt"
	"strings"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/builder"
	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

const cachePrefix = "dl"

type Checker struct {
	network int
	cache   ApprovalCache
	reader  AllowanceReader
}

type Config struct {
	Network int
	Cache   ApprovalCache
	Reader  AllowanceReader
}

type ApprovalCache interface {
	GetApproved(ctx context.Context, hashKey string, keys []string) (map[string]bool, error)
	SetApproved(ctx context.Context, hashKey string, keys []string) error
}

type AllowanceReader interface {
	HasAllowances(ctx context.Context, requests []AllowanceRequest) ([]bool, error)
}

type AllowanceRequest struct {
	Spender resolved.Address
	Token   resolved.Address
	Target  resolved.Address
	Permit2 bool
}

var _ builder.ApprovalChecker = (*Checker)(nil)

func NewChecker(config Config) (*Checker, error) {
	if config.Cache == nil {
		return nil, fmt.Errorf("approval cache is required")
	}
	if config.Reader == nil {
		return nil, fmt.Errorf("allowance reader is required")
	}

	return &Checker{
		network: config.Network,
		cache:   config.Cache,
		reader:  config.Reader,
	}, nil
}

func CacheHashKey(network int) string {
	return fmt.Sprintf("%s_%d_generic_approves", cachePrefix, network)
}

func CacheFieldKey(spender, token, target resolved.Address, permit2 bool) string {
	key := fmt.Sprintf("%s_%s_%s", spender, token, target)
	if permit2 {
		key += "_permit2"
	}
	return strings.ToLower(key)
}

func (c *Checker) Check(
	ctx context.Context,
	spender resolved.Address,
	requests []builder.ApprovalRequest,
) ([]bool, error) {
	if len(requests) == 0 {
		return []bool{}, nil
	}

	hashKey := CacheHashKey(c.network)
	outputKeys := make([]string, len(requests))
	approvedByKey := make(map[string]bool, len(requests))
	requestByKey := make(map[string]AllowanceRequest, len(requests))
	uniqueKeys := make([]string, 0, len(requests))

	for index, request := range requests {
		key := CacheFieldKey(spender, request.Token, request.Target, request.Permit2)
		outputKeys[index] = key
		if _, exists := requestByKey[key]; !exists {
			uniqueKeys = append(uniqueKeys, key)
			requestByKey[key] = AllowanceRequest{
				Spender: normalizeAddress(spender),
				Token:   normalizeAddress(request.Token),
				Target:  normalizeAddress(request.Target),
				Permit2: request.Permit2,
			}
		}
		if isNativeToken(request.Token) {
			approvedByKey[key] = true
			continue
		}
		if _, exists := approvedByKey[key]; !exists {
			approvedByKey[key] = false
		}
	}

	cacheKeys := filterKeys(uniqueKeys, approvedByKey, false)
	if len(cacheKeys) > 0 {
		cached, err := c.cache.GetApproved(ctx, hashKey, cacheKeys)
		if err != nil {
			return nil, err
		}
		for key := range cached {
			approvedByKey[key] = true
		}
	}

	missKeys := filterKeys(uniqueKeys, approvedByKey, false)
	if len(missKeys) > 0 {
		readerRequests := make([]AllowanceRequest, len(missKeys))
		for index, key := range missKeys {
			readerRequests[index] = requestByKey[key]
		}

		onChainApprovals, err := c.reader.HasAllowances(ctx, readerRequests)
		if err != nil {
			return nil, err
		}
		if len(onChainApprovals) != len(readerRequests) {
			return nil, fmt.Errorf("allowance reader result length must match request count")
		}

		approvedKeys := make([]string, 0, len(missKeys))
		for index, approved := range onChainApprovals {
			if !approved {
				continue
			}
			key := missKeys[index]
			approvedByKey[key] = true
			approvedKeys = append(approvedKeys, key)
		}
		if len(approvedKeys) > 0 {
			if err := c.cache.SetApproved(ctx, hashKey, approvedKeys); err != nil {
				return nil, err
			}
		}
	}

	out := make([]bool, len(requests))
	for index, key := range outputKeys {
		out[index] = approvedByKey[key]
	}
	return out, nil
}

func filterKeys(keys []string, approvals map[string]bool, approved bool) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if approvals[key] == approved {
			out = append(out, key)
		}
	}
	return out
}

func normalizeAddress(address resolved.Address) resolved.Address {
	return resolved.Address(strings.ToLower(string(address)))
}

func isNativeToken(token resolved.Address) bool {
	return strings.EqualFold(string(token), string(resolved.NativeTokenAddress))
}
