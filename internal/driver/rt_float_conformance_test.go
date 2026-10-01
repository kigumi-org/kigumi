package driver_test

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// rtFloatHarness drives internal/llgen/runtime/rt_float.c directly (it
// depends on nothing else in the runtime, per its own header comment), so
// this compiles just that one file plus a thin line-protocol wrapper
// instead of building a whole kigumi program. Protocol, one request per
// line: "F <0|1> <hex bits>" formats (0=f64, 1=f32) and replies with the
// text; "P <0|1> <string>" parses and replies "<ok> <hex bits> <consumed>
// <erange>".
const rtFloatHarnessSrc = `#include <stdint.h>
#include <stdio.h>
#include <string.h>
void rt_format_float(double x, int f32, char* out, size_t n);
int rt_parse_f64(const char* s, const char** end, double* out, int* erange);
int rt_parse_f32(const char* s, const char** end, float* out, int* erange);
int main(void) {
    char line[8192];
    while (fgets(line, sizeof line, stdin)) {
        size_t len = strlen(line);
        while (len > 0 && (line[len - 1] == '\n' || line[len - 1] == '\r')) line[--len] = 0;
        if (line[0] == 'F') {
            int f32;
            unsigned long long bits;
            sscanf(line + 2, "%d %llx", &f32, &bits);
            char out[128];
            if (f32) {
                uint32_t b32 = (uint32_t)bits;
                float f;
                memcpy(&f, &b32, 4);
                rt_format_float((double)f, 1, out, sizeof out);
            } else {
                double d;
                memcpy(&d, &bits, 8);
                rt_format_float(d, 0, out, sizeof out);
            }
            printf("%s\n", out);
        } else {
            int f32 = line[2] - '0';
            const char* s = line + 4;
            const char* end;
            int erange = 0, ok;
            uint64_t bits;
            if (f32) {
                float f = 0;
                ok = rt_parse_f32(s, &end, &f, &erange);
                uint32_t b32;
                memcpy(&b32, &f, 4);
                bits = b32;
            } else {
                double d = 0;
                ok = rt_parse_f64(s, &end, &d, &erange);
                memcpy(&bits, &d, 8);
            }
            printf("%d %llx %d %d\n", ok, (unsigned long long)bits, (int)(end - s), erange);
        }
    }
    return 0;
}
`

func compileRtFloatHarness(t *testing.T) string {
	t.Helper()
	cc := driver.CCompiler()
	if cc == nil {
		t.Skip("no C compiler")
	}
	dir := t.TempDir()
	harness := filepath.Join(dir, "harness.c")
	if err := os.WriteFile(harness, []byte(rtFloatHarnessSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	rtFloat, err := filepath.Abs("../llgen/runtime/rt_float.c")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "rt_float_harness")
	args := append(append([]string{}, cc[1:]...), "-O2", "-o", out, harness, rtFloat)
	cmd := exec.Command(cc[0], args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("compile rt_float harness: %v\n%s", err, stderr.String())
	}
	return out
}

// runRtFloatHarness feeds every request in one process (fork+exec per case
// would dominate runtime for a corpus this size).
func runRtFloatHarness(t *testing.T, bin string, requests []string) []string {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(strings.Join(requests, "\n") + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("run rt_float harness: %v\n%s", err, errb.String())
	}
	sc := bufio.NewScanner(&out)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) != len(requests) {
		t.Fatalf("harness returned %d lines for %d requests", len(lines), len(requests))
	}
	return lines
}

// TestRtFloatFormatConformance covers handoff 3.7 item 12: rt_float.c's
// Dragon4 shortest-digit formatter must produce exactly what Go's
// strconv.FormatFloat(f, 'g', -1, bits) does, since internal/interp and
// internal/vm format floats that way (internal/vm/display.go,
// internal/interp/display.go) and native must agree byte for byte.
func TestRtFloatFormatConformance(t *testing.T) {
	t.Parallel()
	bin := compileRtFloatHarness(t)

	var reqs []string
	f64s := []float64{
		0, math.Copysign(0, -1), 1, -1, 100000, 999999, 1000000, 1234567, 123456,
		0.0001, 0.00001, 1.5, 2.5, 0.5, 100.5, 1234567.5, 123456.5,
		math.MaxFloat64, -math.MaxFloat64, math.SmallestNonzeroFloat64,
		2.2250738585072014e-308, 9007199254740992, 9007199254740993, 9007199254740994,
		1e300, 1e-300, 1e21, 1e20, 1e-4, 1e-5, 5e-324, 1.7976931348623157e308,
	}
	for _, v := range f64s {
		reqs = append(reqs, fmt.Sprintf("F 0 %x", math.Float64bits(v)))
	}
	f32s := []float32{
		0, float32(math.Copysign(0, -1)), 1, -1, 100000, 999999, 1000000,
		16777216, 16777217, 0.1, 0.2, 1.0 / 3.0, math.MaxFloat32,
		1.401298464324817e-45, 1.1754944e-38, 1.1754942e-38, 3.4028235e+38, 1e-45,
	}
	for _, v := range f32s {
		reqs = append(reqs, fmt.Sprintf("F 1 %x", math.Float32bits(v)))
	}

	// Exhaustive over every exact power of two (not sampled): this is the
	// shape that caught f32 2^-12 needing a pinned tie-break exception
	// (see rt_float.c).
	for exp := 1; exp <= 254; exp++ {
		reqs = append(reqs, fmt.Sprintf("F 1 %x", uint32(exp)<<23))
	}
	for exp := 1; exp <= 2046; exp++ {
		reqs = append(reqs, fmt.Sprintf("F 0 %x", uint64(exp)<<52))
	}

	// Random sample across every bit-pattern category (normal, subnormal,
	// near-Inf, and exact powers of two), the shape of the Ryu/Grisu test
	// suites: shortest-digit bugs cluster at these boundaries.
	n := 200000
	if testing.Short() {
		n = 5000
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < n; i++ {
		var bits uint32
		switch i % 4 {
		case 0:
			bits = rng.Uint32()
		case 1:
			bits = rng.Uint32() & 0x807FFFFF
		case 2:
			bits = (rng.Uint32() & 0x807FFFFF) | 0x7F000000
		case 3:
			bits = rng.Uint32() &^ 0x7FFFFF
		}
		reqs = append(reqs, fmt.Sprintf("F 1 %x", bits))
	}
	n64 := 80000
	if testing.Short() {
		n64 = 3000
	}
	for i := 0; i < n64; i++ {
		var bits uint64
		switch i % 4 {
		case 0:
			bits = rng.Uint64()
		case 1:
			bits = rng.Uint64() & 0x800FFFFFFFFFFFFF
		case 2:
			bits = (rng.Uint64() & 0x800FFFFFFFFFFFFF) | 0x7FE0000000000000
		case 3:
			bits = rng.Uint64() &^ 0xFFFFFFFFFFFFF
		}
		reqs = append(reqs, fmt.Sprintf("F 0 %x", bits))
	}

	got := runRtFloatHarness(t, bin, reqs)
	fails := 0
	for i, req := range reqs {
		var f32 int
		var bits uint64
		fmt.Sscanf(req, "F %d %x", &f32, &bits)
		var want string
		if f32 == 1 {
			want = strconv.FormatFloat(float64(math.Float32frombits(uint32(bits))), 'g', -1, 32)
		} else {
			want = strconv.FormatFloat(math.Float64frombits(bits), 'g', -1, 64)
		}
		if got[i] != want {
			fails++
			if fails <= 20 {
				t.Errorf("format mismatch: f32=%d bits=%x want=%q got=%q", f32, bits, want, got[i])
			}
		}
	}
	if fails > 0 {
		t.Fatalf("%d/%d formatting cases mismatched", fails, len(reqs))
	}
}

// TestRtFloatParseConformance covers the parsing half of handoff 3.7 item
// 12: rt_parse_f64/rt_parse_f32's big-integer "Algorithm M" must round
// exactly like a correctly rounded strtod/strtof (Go's strconv.ParseFloat
// is also correctly rounded, so it is a valid oracle for plain decimal
// text, which is all rt_float.c's reduced grammar accepts — see the
// no-hex-floats, no-NaN-payload note on rt_parse_generic in rt_float.c).
func TestRtFloatParseConformance(t *testing.T) {
	t.Parallel()
	bin := compileRtFloatHarness(t)

	special := []string{
		"0", "-0", "0.0", "-0.0", "00000", "0.000", "000.000e10",
		"1", "-1", "1.0", "1e0", "1e+0", "1e-0",
		"1e400", "-1e400", "1e-400", "-1e-400", "1e309", "1e-324", "1e-325", "5e-324", "4.9e-324",
		"1.7976931348623157e308", "1.7976931348623159e308", "1.7976931348623157e+308",
		"9007199254740993", "9007199254740992", "9007199254740994.5",
		"18446744073709551615", "18446744073709551616",
		"3.14159265358979323846264338327950288419716939937510",
		"1.00000000000000000000000000000000000000000000000001",
		"123456789012345678901234567890.123456789012345678901234567890e-20",
		".5", "5.", "1.", ".1", "  42", "  -42", "42  ", "+", "-", "", ".", "e5", "1e", "1e+", "1ee5",
		"340282346638528859811704183484516925440",
		"1.401298464324817e-45", "1.1754943508222875e-38",
		"16777216", "16777217", "16777218",
		"0." + strings.Repeat("0", 400) + "1",
		"1" + strings.Repeat("0", 400),
	}

	var reqs []string
	var modes []int // 0 = f64, 1 = f32
	add := func(f32 int, s string) {
		reqs = append(reqs, fmt.Sprintf("P %d %s", f32, s))
		modes = append(modes, f32)
	}
	for _, s := range special {
		add(0, s)
		add(1, s)
	}

	rng := rand.New(rand.NewSource(7))
	randDigits := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte('0' + rng.Intn(10))
		}
		return string(b)
	}

	// Round-trip through Go's own shortest formatter: this is the property
	// that matters most in practice (format then parse recovers exactly).
	n := 60000
	if testing.Short() {
		n = 2000
	}
	for i := 0; i < n; i++ {
		bits := rng.Uint64()
		x := math.Float64frombits(bits)
		if math.IsNaN(x) {
			continue
		}
		add(0, strconv.FormatFloat(x, 'g', -1, 64))
	}
	for i := 0; i < n; i++ {
		bits := uint32(rng.Uint32())
		x := math.Float32frombits(bits)
		if math.IsNaN(float64(x)) {
			continue
		}
		add(1, strconv.FormatFloat(float64(x), 'g', -1, 32))
	}

	n2 := 60000
	if testing.Short() {
		n2 = 2000
	}
	for i := 0; i < n2; i++ {
		intLen, fracLen := rng.Intn(20), rng.Intn(25)
		s := ""
		if rng.Intn(2) == 0 {
			s += "-"
		}
		if intLen == 0 {
			s += "0"
		} else {
			s += randDigits(intLen)
		}
		if fracLen > 0 {
			s += "." + randDigits(fracLen)
		}
		if rng.Intn(3) == 0 {
			s += fmt.Sprintf("e%+d", rng.Intn(700)-350)
		}
		add(rng.Intn(2), s)
	}

	// Long digit strings exercise MAX_SIG_DIGITS truncation + the sticky
	// tie-break bit.
	for i := 0; i < 2000; i++ {
		nd := 700 + rng.Intn(400)
		s := randDigits(1+rng.Intn(9)) + "." + randDigits(nd)
		f32 := rng.Intn(2)
		add(f32, s)
		add(f32, s+strings.Repeat("0", 24))
		add(f32, s+strings.Repeat("9", 24))
	}

	got := runRtFloatHarness(t, bin, reqs)
	fails := 0
	for i, req := range reqs {
		s := req[4:]
		f32 := modes[i]

		var ok, consumed, erange int
		var bitsHex string
		fmt.Sscanf(got[i], "%d %s %d %d", &ok, &bitsHex, &consumed, &erange)
		bits, err := strconv.ParseUint(bitsHex, 16, 64)
		if err != nil {
			t.Fatalf("bad harness output %q for %q", got[i], req)
		}

		ws := 0
		for ws < len(s) && strings.ContainsRune(" \t\n\v\f\r", rune(s[ws])) {
			ws++
		}
		coreLen := consumed - ws
		var core string
		if ok == 1 && coreLen >= 0 && ws+coreLen <= len(s) {
			core = s[ws : ws+coreLen]
		}

		bitSize, expBits, mantBits := 64, 11, 52
		if f32 == 1 {
			bitSize, expBits, mantBits = 32, 8, 23
		}
		isNaN := (bits>>uint(mantBits))&((1<<uint(expBits))-1) == (1<<uint(expBits))-1 &&
			bits&((uint64(1)<<uint(mantBits))-1) != 0
		if isNaN || ok == 0 {
			continue
		}

		want, perr := strconv.ParseFloat(core, bitSize)
		if perr != nil {
			ne, isNum := perr.(*strconv.NumError)
			if !isNum || ne.Err != strconv.ErrRange {
				fails++
				if fails <= 20 {
					t.Errorf("parse mismatch: req=%q core=%q rt-ok but go-err=%v", req, core, perr)
				}
				continue
			}
			// ErrRange: Go still returns the correctly rounded ±Inf/±0.
		}
		var wantBits uint64
		if f32 == 1 {
			wantBits = uint64(math.Float32bits(float32(want)))
		} else {
			wantBits = math.Float64bits(want)
		}
		if wantBits != bits {
			fails++
			if fails <= 20 {
				t.Errorf("parse mismatch: req=%q core=%q got=%x want=%x", req, core, bits, wantBits)
			}
		}
	}
	t.Logf("checked %d parse cases", len(reqs))
	if fails > 0 {
		t.Fatalf("%d/%d parse cases mismatched", fails, len(reqs))
	}
}
