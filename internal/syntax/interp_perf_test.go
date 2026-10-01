package syntax

import (
	"strings"
	"testing"

	"kigumi/internal/token"
)

// nestedInterpSrc builds a string literal nested depth levels deep, each
// level wrapping padPerLevel bytes of filler before its `${...}`.
func nestedInterpSrc(depth, padPerLevel int) string {
	pad := strings.Repeat("a", padPerLevel)
	var b strings.Builder
	for range depth {
		b.WriteString(`"`)
		b.WriteString(pad)
		b.WriteString(`${`)
	}
	b.WriteString("x")
	for range depth {
		b.WriteString(`}`)
		b.WriteString(pad)
		b.WriteString(`"`)
	}
	return b.String()
}

func measureScanOps(depth, padPerLevel int) (ops, srcLen int) {
	src := []byte("let s = " + nestedInterpSrc(depth, padPerLevel) + "\n")
	scanOps = 0
	Parse(&token.File{Src: src})
	return scanOps, len(src)
}

func measureUTF8ValidBytes(depth, padPerLevel int) (bytes, srcLen int) {
	src := []byte("let s = " + nestedInterpSrc(depth, padPerLevel) + "\n")
	utf8ValidBytes = 0
	Parse(&token.File{Src: src})
	return utf8ValidBytes, len(src)
}

// stringLitNodes returns every StringLit node id in the tree.
func stringLitNodes(t *Tree) []NodeID {
	var ids []NodeID
	for id := NodeID(1); int(id) < len(t.Nodes); id++ {
		if t.Kind(id) == StringLit {
			ids = append(ids, id)
		}
	}
	return ids
}

// visitAllStringParts replays what sem, hir and interp each do to every
// string node after parsing: read its StringParts and its Span.
func visitAllStringParts(t *Tree, ids []NodeID) {
	for _, id := range ids {
		for _, p := range t.StringParts(id) {
			_ = p
		}
		t.Span(id)
	}
}

// measurePostParseConsumers runs visitAllStringParts `consumers` times to
// simulate that many independent passes over the same tree, and returns the
// scanOps and spanComputeOps spent doing so (parse-time cost is excluded by
// zeroing both counters after Parse returns).
func measurePostParseConsumers(depth, padPerLevel, consumers int) (scanOpsDelta, spanOpsDelta, srcLen int) {
	src := []byte("let s = " + nestedInterpSrc(depth, padPerLevel) + "\n")
	tr := Parse(&token.File{Src: src})
	ids := stringLitNodes(tr)
	scanOps, spanComputeOps = 0, 0
	for range consumers {
		visitAllStringParts(tr, ids)
	}
	return scanOps, spanComputeOps, len(src)
}

// Counts scan operations instead of timing wall-clock, since wall-clock
// assertions are flaky under race detection or CI contention.
func TestDeepInterpolationScansLinearly(t *testing.T) {
	opsShallow, lenShallow := measureScanOps(10, 100)
	opsDeep, lenDeep := measureScanOps(50, 100)

	if lenDeep <= lenShallow {
		t.Fatalf("test setup: want lenDeep > lenShallow, got %d, %d", lenDeep, lenShallow)
	}
	if lenDeep < 9000 || lenDeep > 11000 {
		t.Fatalf("test setup: want the deep case around 10KB, got %d bytes", lenDeep)
	}

	sizeRatio := float64(lenDeep) / float64(lenShallow)
	opsRatio := float64(opsDeep) / float64(opsShallow)
	if opsRatio > sizeRatio*3 {
		t.Fatalf("scanOps grew %.1fx for a %.1fx growth in source size (depth 10 -> 50); want close to linear, not O(depth²)", opsRatio, sizeRatio)
	}
	if opsDeep > lenDeep*10 {
		t.Fatalf("scanOps = %d for %d bytes of source at depth 50; want roughly linear in size", opsDeep, lenDeep)
	}
}

func TestDeepInterpolationValidatesUTF8Linearly(t *testing.T) {
	bytesShallow, lenShallow := measureUTF8ValidBytes(10, 100)
	bytesDeep, lenDeep := measureUTF8ValidBytes(50, 100)

	sizeRatio := float64(lenDeep) / float64(lenShallow)
	bytesRatio := float64(bytesDeep) / float64(bytesShallow)
	if bytesRatio > sizeRatio*3 {
		t.Fatalf("utf8ValidBytes grew %.1fx for a %.1fx growth in source size (depth 10 -> 50); want close to linear, not O(depth²)", bytesRatio, sizeRatio)
	}
	if bytesDeep > lenDeep*10 {
		t.Fatalf("utf8ValidBytes = %d for %d bytes of source at depth 50; want roughly linear in size", bytesDeep, lenDeep)
	}
}

// scanOps spent after parsing must stay zero across any number of consumer passes.
func TestDeepInterpolationStringPartsIsCachedAcrossConsumers(t *testing.T) {
	for _, depth := range []int{50, 100, 200} {
		ops, _, _ := measurePostParseConsumers(depth, 100, 3)
		if ops != 0 {
			t.Fatalf("depth %d: StringParts across 3 post-parse consumers cost %d scanOps; want 0 (all cache hits)", depth, ops)
		}
	}
}

// Span is lazy, so the first post-parse pass may cost O(depth) cache misses;
// further passes over the same nodes must cost zero.
func TestDeepInterpolationSpanIsMemoized(t *testing.T) {
	for _, depth := range []int{50, 100, 200} {
		src := []byte("let s = " + nestedInterpSrc(depth, 100) + "\n")
		tr := Parse(&token.File{Src: src})
		ids := stringLitNodes(tr)

		spanComputeOps = 0
		visitAllStringParts(tr, ids)
		firstPass := spanComputeOps
		if firstPass == 0 || firstPass > 3*depth+10 {
			t.Fatalf("depth %d: first Span pass did %d cache misses; want roughly linear in depth (>0, <= %d)", depth, firstPass, 3*depth+10)
		}

		spanComputeOps = 0
		visitAllStringParts(tr, ids)
		visitAllStringParts(tr, ids)
		if spanComputeOps != 0 {
			t.Fatalf("depth %d: 2 more Span passes over the same nodes cost %d cache misses; want 0 (all cache hits)", depth, spanComputeOps)
		}
	}
}

// Compares depth 50 against depth 200 (4x): even the first pass, which pays
// for every cache miss once, must cost O(depth), not O(depth²).
func TestDeepInterpolationPostParseConsumersScaleLinearly(t *testing.T) {
	opsShallow, spanShallow, lenShallow := measurePostParseConsumers(50, 100, 1)
	opsDeep, spanDeep, lenDeep := measurePostParseConsumers(200, 100, 1)

	sizeRatio := float64(lenDeep) / float64(lenShallow)
	if opsShallow > 0 {
		if r := float64(opsDeep) / float64(opsShallow); r > sizeRatio*3 {
			t.Fatalf("first-pass StringParts scanOps grew %.1fx for a %.1fx growth in depth (50 -> 200); want close to linear", r, sizeRatio)
		}
	}
	if spanShallow > 0 {
		if r := float64(spanDeep) / float64(spanShallow); r > sizeRatio*3 {
			t.Fatalf("first-pass Span cache misses grew %.1fx for a %.1fx growth in depth (50 -> 200); want close to linear", r, sizeRatio)
		}
	}
}
