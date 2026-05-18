package approvals

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/VeloraDEX/velora-dex-lib/txbuilder/builder"
	"github.com/VeloraDEX/velora-dex-lib/txbuilder/resolved"
)

func TestCacheKeysMatchTypeScriptShape(t *testing.T) {
	spender := resolved.Address("0x1111111111111111111111111111111111111111")
	token := resolved.Address("0xAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAa")
	target := resolved.Address("0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")

	if got, want := CacheHashKey(1), "dl_1_generic_approves"; got != want {
		t.Fatalf("CacheHashKey() = %q, want %q", got, want)
	}

	want := "0x1111111111111111111111111111111111111111_0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb_permit2"
	if got := CacheFieldKey(spender, token, target, true); got != want {
		t.Fatalf("CacheFieldKey() = %q, want %q", got, want)
	}
}

func TestCheckerEmptyInputDoesNotTouchDeps(t *testing.T) {
	checker := newTestChecker(t, 1, &fakeApprovalCache{}, &fakeAllowanceReader{})

	got, err := checker.Check(context.Background(), testSpender, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("len(result) = %d, want 0", len(got))
	}
}

func TestCheckerNativeTokenDoesNotTouchCacheOrReader(t *testing.T) {
	cache := &fakeApprovalCache{}
	reader := &fakeAllowanceReader{}
	checker := newTestChecker(t, 1, cache, reader)

	got, err := checker.Check(context.Background(), testSpender, []builder.ApprovalRequest{
		{Token: resolved.Address("0xEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE"), Target: testTarget},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []bool{true}) {
		t.Fatalf("result = %v, want [true]", got)
	}
	if cache.getCalls != 0 || cache.setCalls != 0 {
		t.Fatalf("cache calls = get:%d set:%d, want none", cache.getCalls, cache.setCalls)
	}
	if reader.calls != 0 {
		t.Fatalf("reader calls = %d, want 0", reader.calls)
	}
}

func TestCheckerZeroAddressIsNotNative(t *testing.T) {
	cache := &fakeApprovalCache{}
	reader := &fakeAllowanceReader{results: []bool{false}}
	checker := newTestChecker(t, 1, cache, reader)

	got, err := checker.Check(context.Background(), testSpender, []builder.ApprovalRequest{
		{Token: resolved.NullAddress, Target: testTarget},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []bool{false}) {
		t.Fatalf("result = %v, want [false]", got)
	}
	if reader.calls != 1 {
		t.Fatalf("reader calls = %d, want 1", reader.calls)
	}
}

func TestCheckerCacheHitAvoidsReaderAndPreservesDuplicateOutput(t *testing.T) {
	keyA := CacheFieldKey(testSpender, testTokenA, testTarget, false)
	keyB := CacheFieldKey(testSpender, testTokenB, testTarget, true)
	cache := &fakeApprovalCache{approved: map[string]bool{
		keyA: false,
		keyB: true,
	}}
	reader := &fakeAllowanceReader{}
	checker := newTestChecker(t, 1, cache, reader)

	got, err := checker.Check(context.Background(), testSpender, []builder.ApprovalRequest{
		{RoutePositionKey: "0-0-0", Token: testTokenA, Target: testTarget},
		{RoutePositionKey: "0-0-1", Token: testTokenA, Target: testTarget},
		{RoutePositionKey: "0-1-0", Token: testTokenB, Target: testTarget, Permit2: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []bool{true, true, true}) {
		t.Fatalf("result = %v, want all true with duplicate preserved", got)
	}
	if reader.calls != 0 {
		t.Fatalf("reader calls = %d, want 0", reader.calls)
	}
	if !reflect.DeepEqual(cache.getKeys, []string{keyA, keyB}) {
		t.Fatalf("cache get keys = %v, want %v", cache.getKeys, []string{keyA, keyB})
	}
}

func TestCheckerCacheMissCallsReaderAndCachesOnlyTrue(t *testing.T) {
	keyA := CacheFieldKey(testSpender, testTokenA, testTarget, false)
	keyB := CacheFieldKey(testSpender, testTokenB, testTarget, true)
	cache := &fakeApprovalCache{}
	reader := &fakeAllowanceReader{results: []bool{true, false}}
	checker := newTestChecker(t, 137, cache, reader)

	got, err := checker.Check(context.Background(), testSpender, []builder.ApprovalRequest{
		{Token: testTokenA, Target: testTarget},
		{Token: testTokenB, Target: testTarget, Permit2: true},
		{Token: testTokenA, Target: testTarget},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []bool{true, false, true}) {
		t.Fatalf("result = %v, want [true false true]", got)
	}
	if reader.calls != 1 {
		t.Fatalf("reader calls = %d, want 1", reader.calls)
	}
	if len(reader.requests) != 2 {
		t.Fatalf("reader request count = %d, want 2", len(reader.requests))
	}
	if !reflect.DeepEqual(cache.setKeys, []string{keyA}) {
		t.Fatalf("cache set keys = %v, want [%s]", cache.setKeys, keyA)
	}
	if cache.setHashKey != "dl_137_generic_approves" {
		t.Fatalf("set hash key = %q, want dl_137_generic_approves", cache.setHashKey)
	}
	if !reflect.DeepEqual(cache.getKeys, []string{keyA, keyB}) {
		t.Fatalf("cache get keys = %v, want %v", cache.getKeys, []string{keyA, keyB})
	}
}

func TestCheckerReturnsReaderLengthMismatch(t *testing.T) {
	checker := newTestChecker(t, 1, &fakeApprovalCache{}, &fakeAllowanceReader{
		results: []bool{true, false},
	})

	_, err := checker.Check(context.Background(), testSpender, []builder.ApprovalRequest{
		{Token: testTokenA, Target: testTarget},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCheckerPropagatesDependencyErrors(t *testing.T) {
	cacheErr := errors.New("cache down")
	checker := newTestChecker(t, 1, &fakeApprovalCache{err: cacheErr}, &fakeAllowanceReader{})
	_, err := checker.Check(context.Background(), testSpender, []builder.ApprovalRequest{
		{Token: testTokenA, Target: testTarget},
	})
	if !errors.Is(err, cacheErr) {
		t.Fatalf("error = %v, want %v", err, cacheErr)
	}

	readerErr := errors.New("rpc down")
	checker = newTestChecker(t, 1, &fakeApprovalCache{}, &fakeAllowanceReader{err: readerErr})
	_, err = checker.Check(context.Background(), testSpender, []builder.ApprovalRequest{
		{Token: testTokenA, Target: testTarget},
	})
	if !errors.Is(err, readerErr) {
		t.Fatalf("error = %v, want %v", err, readerErr)
	}
}

func newTestChecker(
	t *testing.T,
	network int,
	cache ApprovalCache,
	reader AllowanceReader,
) *Checker {
	t.Helper()
	checker, err := NewChecker(Config{
		Network: network,
		Cache:   cache,
		Reader:  reader,
	})
	if err != nil {
		t.Fatal(err)
	}
	return checker
}

type fakeApprovalCache struct {
	approved map[string]bool
	err      error

	getCalls   int
	getHashKey string
	getKeys    []string
	setCalls   int
	setHashKey string
	setKeys    []string
}

func (c *fakeApprovalCache) GetApproved(
	ctx context.Context,
	hashKey string,
	keys []string,
) (map[string]bool, error) {
	c.getCalls++
	c.getHashKey = hashKey
	c.getKeys = append([]string(nil), keys...)
	if c.err != nil {
		return nil, c.err
	}

	out := make(map[string]bool)
	for _, key := range keys {
		if approved, exists := c.approved[key]; exists {
			out[key] = approved
		}
	}
	return out, nil
}

func (c *fakeApprovalCache) SetApproved(ctx context.Context, hashKey string, keys []string) error {
	c.setCalls++
	c.setHashKey = hashKey
	c.setKeys = append([]string(nil), keys...)
	return c.err
}

type fakeAllowanceReader struct {
	results  []bool
	err      error
	calls    int
	requests []AllowanceRequest
}

func (r *fakeAllowanceReader) HasAllowances(
	ctx context.Context,
	requests []AllowanceRequest,
) ([]bool, error) {
	r.calls++
	r.requests = append([]AllowanceRequest(nil), requests...)
	if r.err != nil {
		return nil, r.err
	}
	return append([]bool(nil), r.results...), nil
}

var (
	testSpender = resolved.Address("0x1111111111111111111111111111111111111111")
	testTokenA  = resolved.Address("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	testTokenB  = resolved.Address("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	testTarget  = resolved.Address("0x2222222222222222222222222222222222222222")
)
