package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/builder"
	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

type Entry struct {
	Keys          []string
	Encoder       builder.DexEncoder
	DirectEncoder builder.DirectDexEncoder
}

type Registry struct {
	encoders       map[string]builder.DexEncoder
	directEncoders map[string]builder.DirectDexEncoder
}

func New(entries ...Entry) (*Registry, error) {
	encoders := make(map[string]builder.DexEncoder)
	directEncoders := make(map[string]builder.DirectDexEncoder)

	for entryIndex, entry := range entries {
		if len(entry.Keys) == 0 {
			return nil, fmt.Errorf("dex registry entry %d must have at least one key", entryIndex)
		}
		if entry.Encoder == nil && entry.DirectEncoder == nil {
			return nil, fmt.Errorf("dex registry entry %d encoder or direct encoder is required", entryIndex)
		}
		for _, key := range entry.Keys {
			if key == "" {
				return nil, fmt.Errorf("dex registry entry %d key must be non-empty", entryIndex)
			}
			if entry.Encoder != nil {
				if _, exists := encoders[key]; exists {
					return nil, fmt.Errorf("duplicate dex registry key %q", key)
				}
				encoders[key] = entry.Encoder
			}
			if entry.DirectEncoder != nil {
				if _, exists := directEncoders[key]; exists {
					return nil, fmt.Errorf("duplicate direct dex registry key %q", key)
				}
				directEncoders[key] = entry.DirectEncoder
			}
		}
	}

	return &Registry{encoders: encoders, directEncoders: directEncoders}, nil
}

func MustNew(entries ...Entry) *Registry {
	registry, err := New(entries...)
	if err != nil {
		panic(err)
	}
	return registry
}

func (r *Registry) GetDexEncoder(_ context.Context, network int, dexKey string) (builder.DexEncoder, error) {
	if r == nil {
		return nil, fmt.Errorf("dex encoder registry is nil for %s on network %d", dexKey, network)
	}
	encoder, ok := r.encoders[dexKey]
	if !ok {
		return nil, fmt.Errorf("dex encoder not found for %s on network %d", dexKey, network)
	}
	return encoder, nil
}

func (r *Registry) GetDirectDexEncoder(
	_ context.Context,
	network int,
	dexKey string,
	contractMethod string,
) (builder.DirectDexEncoder, error) {
	if r == nil {
		return nil, fmt.Errorf("direct dex encoder registry is nil for %s on network %d", dexKey, network)
	}
	if !resolved.IsDirectContractMethod(contractMethod) {
		return nil, fmt.Errorf("unsupported V6 direct method %s", contractMethod)
	}
	encoder, ok := r.directEncoders[dexKey]
	if !ok {
		return nil, fmt.Errorf("direct dex encoder not found for %s on network %d", dexKey, network)
	}
	for _, method := range encoder.DirectContractMethodsV6() {
		if strings.EqualFold(method, contractMethod) {
			return encoder, nil
		}
	}
	return nil, fmt.Errorf("direct dex encoder %s does not support method %s", dexKey, contractMethod)
}
