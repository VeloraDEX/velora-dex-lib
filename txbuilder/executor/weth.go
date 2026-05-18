package executor

import "github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"

type WETHBuilder struct{}

func NewWETHBuilder() WETHBuilder {
	return WETHBuilder{}
}

func (WETHBuilder) BuildBytecode(resolved.ExecutorBytecodeBuildInput) (resolved.HexBytes, error) {
	return resolved.HexBytes("0x"), nil
}
