// Shortest round-trip float formatting and correctly rounded decimal
// parsing, libc-free (no snprintf/strtod/strtof/libm). Formatting is
// Dragon4 (Steele & White); parsing is "Algorithm M" big-integer division,
// so one bignum engine serves both.
#include <stdint.h>
#include <string.h>

#include "rt.h"

// ---- minimal big integer: base 2^32 limbs, little-endian, no sign ----
// 192 limbs (6144 bits, ~1849 decimal digits) covers every bignum this
// file builds, with headroom for the shifts Dragon4/Algorithm M apply to
// align binary and decimal exponents.
#define BN_LIMBS 192
typedef struct {
    uint32_t d[BN_LIMBS];
    int n; // used limbs; n==0 means the value 0
} bn;

static void bn_zero(bn* a) {
    a->n = 0;
}
static void bn_trim(bn* a) {
    while (a->n > 0 && a->d[a->n - 1] == 0) a->n--;
}
static void bn_from_u64(bn* a, uint64_t v) {
    a->n = 0;
    while (v) {
        a->d[a->n++] = (uint32_t)v;
        v >>= 32;
    }
}
static int bn_is_zero(const bn* a) {
    return a->n == 0;
}
static void bn_copy(bn* dst, const bn* src) {
    dst->n = src->n;
    memcpy(dst->d, src->d, (size_t)src->n * sizeof src->d[0]);
}
static int bn_cmp(const bn* a, const bn* b) {
    if (a->n != b->n) return a->n < b->n ? -1 : 1;
    for (int i = a->n - 1; i >= 0; i--) {
        if (a->d[i] != b->d[i]) return a->d[i] < b->d[i] ? -1 : 1;
    }
    return 0;
}
static int bn_bitlen(const bn* a) {
    if (a->n == 0) return 0;
    uint32_t top = a->d[a->n - 1];
    int bits = (a->n - 1) * 32;
    while (top) {
        bits++;
        top >>= 1;
    }
    return bits;
}
// a += b
static void bn_add(bn* a, const bn* b) {
    int n = a->n > b->n ? a->n : b->n;
    uint64_t carry = 0;
    for (int i = 0; i < n; i++) {
        uint64_t x = (i < a->n ? a->d[i] : 0) + (uint64_t)(i < b->n ? b->d[i] : 0) + carry;
        a->d[i] = (uint32_t)x;
        carry = x >> 32;
    }
    a->n = n;
    if (carry) a->d[a->n++] = (uint32_t)carry;
}
// a -= b, requires a >= b
static void bn_sub(bn* a, const bn* b) {
    int64_t borrow = 0;
    for (int i = 0; i < a->n; i++) {
        int64_t x = (int64_t)a->d[i] - (int64_t)(i < b->n ? b->d[i] : 0) - borrow;
        if (x < 0) {
            x += (int64_t)1 << 32;
            borrow = 1;
        } else {
            borrow = 0;
        }
        a->d[i] = (uint32_t)x;
    }
    bn_trim(a);
}
// a *= m, m a small (<2^32) multiplier
static void bn_mul_u32(bn* a, uint32_t m) {
    uint64_t carry = 0;
    for (int i = 0; i < a->n; i++) {
        uint64_t x = (uint64_t)a->d[i] * m + carry;
        a->d[i] = (uint32_t)x;
        carry = x >> 32;
    }
    while (carry) {
        a->d[a->n++] = (uint32_t)carry;
        carry >>= 32;
    }
}
// a += m (small)
static void bn_add_u32(bn* a, uint32_t m) {
    uint64_t carry = m;
    for (int i = 0; i < a->n && carry; i++) {
        uint64_t x = (uint64_t)a->d[i] + carry;
        a->d[i] = (uint32_t)x;
        carry = x >> 32;
    }
    while (carry) {
        a->d[a->n++] = (uint32_t)carry;
        carry >>= 32;
    }
}
// a <<= bits (bits >= 0)
static void bn_shl(bn* a, int bits) {
    if (bits <= 0 || a->n == 0) return;
    int limbShift = bits / 32, bitShift = bits % 32;
    int newn = a->n + limbShift + (bitShift ? 1 : 0);
    for (int i = newn - 1; i >= 0; i--) {
        uint32_t lo = 0, hi = 0;
        int src = i - limbShift;
        if (src >= 0 && src < a->n) lo = a->d[src];
        if (bitShift && src - 1 >= 0 && src - 1 < a->n) hi = a->d[src - 1];
        uint32_t v = bitShift ? (lo << bitShift) | (uint32_t)((uint64_t)hi >> (32 - bitShift)) : lo;
        a->d[i] = v;
    }
    a->n = newn;
    bn_trim(a);
}
static void bn_mul_pow10(bn* a, int k) {
    for (int i = 0; i < k; i++) bn_mul_u32(a, 10);
}
// Full binary long division: q = a / b, r = a % b. q's buffer may alias a.
static void bn_divmod(const bn* a, const bn* b, bn* q, bn* r) {
    bn_zero(q);
    bn_zero(r);
    int bits = bn_bitlen(a);
    for (int i = bits - 1; i >= 0; i--) {
        bn_shl(r, 1);
        int limb = i / 32, off = i % 32;
        if (limb < a->n && (a->d[limb] >> off & 1)) {
            if (r->n == 0) {
                r->d[0] = 1;
                r->n = 1;
            } else {
                r->d[0] |= 1;
            }
        }
        int bit = bn_cmp(r, b) >= 0;
        if (bit) bn_sub(r, b);
        int qlimb = i / 32, qoff = i % 32;
        if (bit) {
            while (q->n <= qlimb) q->d[q->n++] = 0;
            q->d[qlimb] |= (uint32_t)1 << qoff;
        }
    }
    bn_trim(q);
}
// 2*r compared against b, without doubling into a possibly-overflowing bignum.
static int bn_cmp_2x(const bn* r, const bn* b) {
    bn t;
    bn_copy(&t, r);
    bn_shl(&t, 1);
    return bn_cmp(&t, b);
}

// ---- IEEE754 bit decomposition (no math.h; memcpy avoids strict-aliasing UB) ----
static uint64_t f64_bits(double x) {
    uint64_t u;
    memcpy(&u, &x, 8);
    return u;
}
static double f64_from_bits(uint64_t u) {
    double x;
    memcpy(&x, &u, 8);
    return x;
}
static uint32_t f32_bits(float x) {
    uint32_t u;
    memcpy(&u, &x, 4);
    return u;
}
static float f32_from_bits(uint32_t u) {
    float x;
    memcpy(&x, &u, 4);
    return x;
}

// ---- formatting: Dragon4 (Steele & White) shortest round-trip digits ----
// A finite nonzero magnitude as value = m * 2^e (m the full p-bit
// significand). minExp distinguishes a genuine power-of-two boundary
// (asymmetric neighbor spacing) from the smallest normal, which has none.
typedef struct {
    uint64_t m;
    int e, p, minExp;
} fdecomp;
static fdecomp decompose_f64(double x) {
    uint64_t bits = f64_bits(x);
    int biased = (int)((bits >> 52) & 0x7FF);
    uint64_t frac = bits & 0xFFFFFFFFFFFFFULL;
    fdecomp r = {.p = 53, .minExp = -1074};
    if (biased == 0) {
        r.m = frac;
        r.e = -1074;
    } else {
        r.m = frac | (1ULL << 52);
        r.e = biased - 1023 - 52;
    }
    return r;
}
static fdecomp decompose_f32(float x) {
    uint32_t bits = f32_bits(x);
    int biased = (int)((bits >> 23) & 0xFF);
    uint32_t frac = bits & 0x7FFFFF;
    fdecomp r = {.p = 24, .minExp = -149};
    if (biased == 0) {
        r.m = frac;
        r.e = -149;
    } else {
        r.m = (uint64_t)(frac | (1u << 23));
        r.e = biased - 127 - 23;
    }
    return r;
}
// Shortest decimal digits such that the value equals 0.<digits> * 10^dp;
// digits must hold fd.p+1 bytes (the +1 covers a carry-out inserting a
// leading "1", e.g. 9.99...9 rounding up to 10.0...0).
static void dragon4_shortest(fdecomp fd, char* digits, int* ndigits, int* dp) {
    int pow2 = fd.m == (1ULL << (fd.p - 1));
    int asymmetric = pow2 && fd.e > fd.minExp;

    bn R, S, mP, mM;
    bn_from_u64(&R, fd.m);
    bn_mul_u32(&R, 4);
    bn_from_u64(&S, 4);
    bn_from_u64(&mP, 2);
    bn_from_u64(&mM, asymmetric ? 1 : 2);
    if (fd.e >= 0) {
        bn_shl(&R, fd.e);
        bn_shl(&mP, fd.e);
        bn_shl(&mM, fd.e);
    } else {
        bn_shl(&S, -fd.e);
    }

    // Decimal-point estimate from bit length (within ~1 of exact), fixed
    // up below by exact bignum comparison.
    int bits = bn_bitlen(&R) - bn_bitlen(&S);
    int k = (int)(((int64_t)bits * 30103 + 99999) / 100000) + 1;
    if (k >= 0) bn_mul_pow10(&S, k);
    else {
        bn_mul_pow10(&R, -k);
        bn_mul_pow10(&mP, -k);
        bn_mul_pow10(&mM, -k);
    }
    for (;;) {
        bn t;
        bn_copy(&t, &R);
        bn_add(&t, &mP);
        if (bn_cmp(&t, &S) > 0) {
            bn_mul_u32(&S, 10);
            k++;
            continue;
        }
        bn t10;
        bn_copy(&t10, &t);
        bn_mul_u32(&t10, 10);
        if (bn_cmp(&t10, &S) <= 0) {
            bn_mul_u32(&R, 10);
            bn_mul_u32(&mP, 10);
            bn_mul_u32(&mM, 10);
            k--;
            continue;
        }
        break;
    }

    int roundEven = (fd.m & 1) == 0;
    int n = 0;
    for (;;) {
        bn_mul_u32(&R, 10);
        bn_mul_u32(&mP, 10);
        bn_mul_u32(&mM, 10);
        // R < S*10 is the loop invariant (R<S held before scaling by 10),
        // so the digit is a single decimal digit found by subtraction
        // rather than a full division.
        int d = 0;
        while (bn_cmp(&R, &S) >= 0) {
            bn_sub(&R, &S);
            d++;
        }
        int low = roundEven ? bn_cmp(&R, &mM) <= 0 : bn_cmp(&R, &mM) < 0;
        bn t;
        bn_copy(&t, &R);
        bn_add(&t, &mP);
        int high = roundEven ? bn_cmp(&t, &S) >= 0 : bn_cmp(&t, &S) > 0;
        if (!low && !high) {
            digits[n++] = (char)('0' + d);
            continue;
        }
        if (low && !high) {
            digits[n++] = (char)('0' + d);
        } else if (high && !low) {
            digits[n++] = (char)('0' + d + 1);
        } else {
            // 2R==S is an exact decimal tie (only possible when x's binary
            // expansion terminates in decimal). Round-half-to-even on d
            // matches Go's strconv except for f32's 2^-12 (fd.p==24,
            // fd.e==-35): Go's Dragonbox rounds up there instead, a pinned
            // hardcoded exception, not a general rule (see `exp == -77`
            // in Go's internal/strconv/ftoadbox.go).
            int cmp2 = bn_cmp_2x(&R, &S);
            int up = cmp2 > 0 || (cmp2 == 0 && ((fd.p == 24 && fd.e == -35) ? 1 : (d & 1)));
            digits[n++] = (char)('0' + d + (up ? 1 : 0));
        }
        break;
    }
    // A rounded-up final digit of value 10 carries into earlier digits
    // (e.g. 9.9995 rounding its last kept digit up becomes 10.000).
    int i = n - 1;
    while (digits[i] == '0' + 10) {
        digits[i] = '0';
        if (i == 0) {
            for (int j = n; j > 0; j--) digits[j] = digits[j - 1];
            digits[0] = '1';
            n++;
            k++;
            break;
        }
        digits[--i]++;
    }
    *ndigits = n;
    *dp = k;
}

static void copy_lit(char* out, size_t n, const char* s) {
    size_t i = 0;
    for (; s[i] && i + 1 < n; i++) out[i] = s[i];
    out[i] = 0;
}
// Places digits[0..nd) at decimal point dp into out, in Go strconv's %g
// layout: scientific when the leading digit's power is below -4 or at
// least 6, plain digits otherwise.
static void place_digits(char* out, size_t n, int neg, const char* digits, int nd, int dp) {
    char* p = out;
    char* end = out + n - 1;
    if (neg && p < end) *p++ = '-';
    int exp = dp - 1;
    if (exp < -4 || exp >= 6) {
        if (p < end) *p++ = digits[0];
        if (nd > 1) {
            if (p < end) *p++ = '.';
            for (int i = 1; i < nd && p < end; i++) *p++ = digits[i];
        }
        if (p < end) *p++ = 'e';
        if (p < end) *p++ = exp < 0 ? '-' : '+';
        int ae = exp < 0 ? -exp : exp;
        char ebuf[8];
        int en = 0;
        do {
            ebuf[en++] = (char)('0' + ae % 10);
            ae /= 10;
        } while (ae);
        while (en < 2) ebuf[en++] = '0';
        for (int i = en - 1; i >= 0 && p < end; i--) *p++ = ebuf[i];
    } else if (dp <= 0) {
        if (p < end) *p++ = '0';
        if (p < end) *p++ = '.';
        for (int i = 0; i < -dp && p < end; i++) *p++ = '0';
        for (int i = 0; i < nd && p < end; i++) *p++ = digits[i];
    } else if (dp >= nd) {
        for (int i = 0; i < nd && p < end; i++) *p++ = digits[i];
        for (int i = 0; i < dp - nd && p < end; i++) *p++ = '0';
    } else {
        for (int i = 0; i < dp && p < end; i++) *p++ = digits[i];
        if (p < end) *p++ = '.';
        for (int i = dp; i < nd && p < end; i++) *p++ = digits[i];
    }
    *p = 0;
}
// rt_format_float needs no parse-back check: Dragon4 generates the
// shortest digits directly.
void rt_format_float(double x, int f32, char* out, size_t n) {
    uint64_t bits = f64_bits(x);
    int neg = (bits >> 63) != 0;
    if (((bits >> 52) & 0x7FF) == 0x7FF) {
        copy_lit(out, n, (bits & 0xFFFFFFFFFFFFFULL) ? "NaN" : (neg ? "-Inf" : "+Inf"));
        return;
    }
    if ((bits << 1) == 0) {
        copy_lit(out, n, neg ? "-0" : "0");
        return;
    }
    fdecomp fd = f32 ? decompose_f32((float)x) : decompose_f64(x);
    char digits[64];
    int nd, dp;
    dragon4_shortest(fd, digits, &nd, &dp);
    place_digits(out, n, neg, digits, nd, dp);
}

// ---- parsing: correctly rounded decimal -> binary ("Algorithm M": exact
// rational num/den, rounded to nearest-even) ----
// More than this many significant digits can't change a double's correctly
// rounded result (the excess is captured by `sticky` for tie-breaking).
#define MAX_SIG_DIGITS 768
// abs(decExp10 + digit count) past this is unambiguously overflow/underflow
// for both f32 and f64 (real boundaries sit under 330).
#define EXP10_SHORTCUT 350

// Scans an unsigned decimal literal at *sp, filling D (significant digits,
// capped at MAX_SIG_DIGITS), decExp (value == D * 10^decExp), sticky (a
// nonzero digit was dropped past the cap) and *sigCount.
static int scan_unsigned_decimal(const char** sp, bn* D, int* decExp, int* sticky, int* sigCountOut) {
    const char* p = *sp;
    bn_zero(D);
    int seenDigit = 0, seenNonzero = 0, sigCount = 0, totalFrac = 0, dropped = 0;
    int afterPoint = 0;
    *sticky = 0;
    for (;;) {
        char c = *p;
        if (c == '.' && !afterPoint) {
            afterPoint = 1;
            p++;
            continue;
        }
        if (c < '0' || c > '9') break;
        seenDigit = 1;
        int dgt = c - '0';
        if (afterPoint) totalFrac++;
        if (!seenNonzero && dgt == 0) {
            p++;
            continue;
        }
        seenNonzero = 1;
        if (sigCount < MAX_SIG_DIGITS) {
            bn_mul_u32(D, 10);
            bn_add_u32(D, (uint32_t)dgt);
            sigCount++;
        } else {
            dropped++;
            if (dgt) *sticky = 1;
        }
        p++;
    }
    if (!seenDigit) return 0;
    int explicitExp = 0;
    if (*p == 'e' || *p == 'E') {
        const char* q = p + 1;
        int esign = 1;
        if (*q == '+' || *q == '-') {
            esign = *q == '-' ? -1 : 1;
            q++;
        }
        if (*q >= '0' && *q <= '9') {
            int64_t val = 0;
            while (*q >= '0' && *q <= '9') {
                if (val < 2000000000) val = val * 10 + (*q - '0');
                q++;
            }
            p = q;
            val *= esign;
            explicitExp = (int)(val > 1000000000 ? 1000000000 : (val < -1000000000 ? -1000000000 : val));
        }
    }
    *decExp = -totalFrac + dropped + explicitExp;
    *sigCountOut = sigCount;
    *sp = p;
    return 1;
}
static int ci_lit(const char* s, const char* lit) {
    int i = 0;
    for (; lit[i]; i++) {
        char a = s[i];
        if (a >= 'A' && a <= 'Z') a = (char)(a + 32);
        if (a != lit[i]) return 0;
    }
    return i;
}
// Converts the exact value D * 10^decExp to the nearest p-bit significand.
// *outE uses value == mant * 2^(*outE - (p-1)); the caller compares *outE
// to the format's max exponent for overflow and *outMant to 2^(p-1) for
// the subnormal/normal split.
static void decimal_to_binary(const bn* D, int decExp, int sticky, int p, int minExpNormal, uint64_t* outMant, int* outE) {
    bn num, den;
    if (decExp >= 0) {
        bn_copy(&num, D);
        bn_mul_pow10(&num, decExp);
        bn_from_u64(&den, 1);
    } else {
        bn_copy(&num, D);
        bn_from_u64(&den, 1);
        bn_mul_pow10(&den, -decExp);
    }
    int sCap = (p - 1) - minExpNormal;
    int s = (p - 1) - (bn_bitlen(&num) - bn_bitlen(&den));
    if (s > sCap) s = sCap;

    bn q, r, dUsed;
    for (;;) {
        bn n2, d2;
        if (s >= 0) {
            bn_copy(&n2, &num);
            bn_shl(&n2, s);
            bn_copy(&d2, &den);
        } else {
            bn_copy(&n2, &num);
            bn_copy(&d2, &den);
            bn_shl(&d2, -s);
        }
        bn_divmod(&n2, &d2, &q, &r);
        int qbits = bn_bitlen(&q);
        if (qbits > p) {
            s -= qbits - p;
            continue;
        }
        if (qbits < p && s < sCap) {
            int want = s + (p - qbits);
            s = want > sCap ? sCap : want;
            continue;
        }
        bn_copy(&dUsed, &d2);
        break;
    }
    uint64_t qval = q.n > 0 ? q.d[0] : 0;
    if (q.n > 1) qval |= (uint64_t)q.d[1] << 32;
    int cmp = bn_cmp_2x(&r, &dUsed);
    int roundup = cmp > 0 || (cmp == 0 && (sticky || (qval & 1)));
    if (roundup) qval++;
    int E = (p - 1) - s;
    if (qval == (1ULL << p)) {
        qval >>= 1;
        E++;
    }
    *outMant = qval;
    *outE = E;
}
// rt_parse_generic is the shared core for rt_parse_f64/rt_parse_f32:
// strtod/strtof grammar minus hex floats and the NaN(n-char-seq) payload.
static int rt_parse_generic(const char* s, const char** end, int p, int minExpNormal, int bias, int maxBiasedExp, int signPos, uint64_t* bitsOut, int* erange) {
    *erange = 0;
    const char* p0 = s;
    while (*s == ' ' || *s == '\t' || *s == '\n' || *s == '\v' || *s == '\f' || *s == '\r') s++;
    int neg = 0;
    if (*s == '+' || *s == '-') {
        neg = *s == '-';
        s++;
    }
    int n;
    if ((n = ci_lit(s, "infinity")) || (n = ci_lit(s, "inf"))) {
        s += n;
        *bitsOut = ((uint64_t)neg << signPos) | ((uint64_t)maxBiasedExp << (p - 1));
        *end = s;
        return 1;
    }
    if ((n = ci_lit(s, "nan"))) {
        s += n;
        if (*s == '(') {
            const char* q = s + 1;
            while (*q && *q != ')') q++;
            if (*q == ')') s = q + 1;
        }
        *bitsOut = ((uint64_t)neg << signPos) | ((uint64_t)maxBiasedExp << (p - 1)) | (1ULL << (p - 2));
        *end = s;
        return 1;
    }
    bn D;
    int decExp, sticky, sigCount;
    if (!scan_unsigned_decimal(&s, &D, &decExp, &sticky, &sigCount)) {
        *end = p0;
        return 0;
    }
    *end = s;
    if (bn_is_zero(&D)) {
        *bitsOut = (uint64_t)neg << signPos;
        return 1;
    }
    int pointExp = decExp + sigCount;
    uint64_t mant;
    int E;
    if (pointExp > EXP10_SHORTCUT) {
        mant = 1ULL << (p - 1);
        E = maxBiasedExp - bias + 1; // force the overflow branch below
    } else if (pointExp < -EXP10_SHORTCUT) {
        mant = 0;
        E = minExpNormal;
    } else {
        decimal_to_binary(&D, decExp, sticky, p, minExpNormal, &mant, &E);
    }
    if (E + bias >= maxBiasedExp) { // overflow to infinity, incl. round-to-even at the largest finite value
        *bitsOut = ((uint64_t)neg << signPos) | ((uint64_t)maxBiasedExp << (p - 1));
        *erange = 1; // matches glibc: HUGE_VAL result sets errno to ERANGE
        return 1;
    }
    uint64_t biasedExp, field;
    if (mant >> (p - 1)) { // normal: mant has the implicit top bit set
        biasedExp = (uint64_t)(E + bias);
        field = mant & ((1ULL << (p - 1)) - 1);
    } else {
        biasedExp = 0;
        field = mant;
        *erange = 1; // matches glibc: a nonzero D underflowing to subnormal-or-0 sets ERANGE too
    }
    *bitsOut = ((uint64_t)neg << signPos) | (biasedExp << (p - 1)) | field;
    return 1;
}
// *erange is set the way glibc sets errno==ERANGE: whenever the exact
// decimal value over- or underflows the target width, even though *out
// still holds the correctly rounded result. Pass NULL to ignore it.
int rt_parse_f64(const char* s, const char** end, double* out, int* erange) {
    uint64_t bits;
    int localErange;
    if (!rt_parse_generic(s, end, 53, -1022, 1023, 2047, 63, &bits, &localErange)) return 0;
    if (erange) *erange = localErange;
    *out = f64_from_bits(bits);
    return 1;
}
int rt_parse_f32(const char* s, const char** end, float* out, int* erange) {
    uint64_t bits;
    int localErange;
    if (!rt_parse_generic(s, end, 24, -126, 127, 255, 31, &bits, &localErange)) return 0;
    if (erange) *erange = localErange;
    *out = f32_from_bits((uint32_t)bits);
    return 1;
}
