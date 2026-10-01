// kigumi runtime core: boxed values, reference counting, allocators, the
// std primitives that need no operating system. System access goes through
// rt_sys_*.c.
#define _GNU_SOURCE
#include <math.h>
#include <stdlib.h>
#include <string.h>

#include "rt.h"
static int g_argc;
static char** g_argv;
static int g_ntypes;
static rt_type** g_types;
static rt_vdesc *V_Some, *V_None, *V_Ok, *V_Err, *V_Less, *V_Equal, *V_Greater;
static rt_type T_Error = {.name = "Error"};
// The runtime's own errors are boxed as Error with `message` in slot 0.
// The retain offsets invoke's release of the argument array, as rt_at does.
static val error_message_adapter(val env, val args) {
    (void)env;
    val e = rt_retain(args->u.arr.items[0]);
    return rt_str((const char*)e->u.op.data, strlen((const char*)e->u.op.data));
}
static void* T_Error_vt[1] = {(void*)error_message_adapter};

// Allocators: objects and their slot buffers come from the current
// allocator and remember it, so escaping values free through their origin.
// String bytes stay on libc because every producer of owned text uses malloc.
typedef struct rt_allocator {
    void* (*alloc)(struct rt_allocator*, size_t);
    void (*free)(struct rt_allocator*, void*);
    int64_t live, bytes;
    int closed;
} rt_allocator;
static void* gen_alloc(rt_allocator* a, size_t n) {
    void* p = calloc(1, n);
    if (p) a->live++;
    return p;
}
static void gen_free(rt_allocator* a, void* p) {
    a->live--;
    free(p);
}
static rt_allocator g_general = {.alloc = gen_alloc, .free = gen_free};
static rt_allocator* g_alloc_stack[64] = {&g_general};
static int g_alloc_sp = 0;
static rt_allocator* cur_alloc(void) {
    return g_alloc_stack[g_alloc_sp];
}
static void* amalloc(rt_allocator* a, size_t n) {
    a->bytes += n;
    return a->alloc(a, n);
}
static void afree(rt_allocator* a, void* p) {
    if (p) a->free(a, p);
}
static void* arealloc(rt_allocator* a, void* p, size_t oldn, size_t newn) {
    void* q = amalloc(a, newn);
    if (!q) return NULL;
    if (p) {
        memcpy(q, p, oldn);
        afree(a, p);
    }
    return q;
}
typedef struct chunk {
    struct chunk* next;
    size_t cap, used;
    char data[];
} chunk;
typedef struct arena {
    rt_allocator a;
    chunk* head;
} arena;
static void* arena_alloc(rt_allocator* a, size_t n) {
    arena* ar = (arena*)a;
    n = (n + 15) & ~(size_t)15;
    chunk* c = ar->head;
    if (!c || c->used + n > c->cap) {
        size_t cap = n > 65536 ? n : 65536;
        c = malloc(sizeof(chunk) + cap);
        if (!c) return NULL;
        c->next = ar->head;
        c->cap = cap;
        c->used = 0;
        ar->head = c;
    }
    void* p = c->data + c->used;
    c->used += n;
    memset(p, 0, n);
    a->live++;
    return p;
}
static void arena_drop_chunks(arena* ar) {
    for (chunk* c = ar->head; c;) {
        chunk* nx = c->next;
        free(c);
        c = nx;
    }
    free(ar);
}
// Live objects keep the chunks; a closed arena is freed by its last object.
static void arena_free(rt_allocator* a, void* p) {
    (void)p;
    if (--a->live == 0 && a->closed) arena_drop_chunks((arena*)a);
}
static void arena_close(arena* ar) {
    ar->a.closed = 1;
    if (ar->a.live == 0) arena_drop_chunks(ar);
}
static val alloc_in(rt_allocator* a, int kind) {
    val v = amalloc(a, sizeof(struct obj));
    if (!v) rt_panic("out of memory");
    v->kind = kind;
    v->rc = 1;
    v->al = a;
    return v;
}
static val alloc(int kind) {
    return alloc_in(cur_alloc(), kind);
}
// An allocator's own bookkeeping objects come from that allocator, so an
// `allocator` block needs no ambient allocator (`allocator = none`).
static val opaque_in(rt_allocator* a, const char* kind, void* data) {
    val v = alloc_in(a, K_OPAQUE);
    v->u.op.kind = kind;
    v->u.op.data = data;
    return v;
}
// Reference counts: every heap object starts at 1; static objects use -1
// and are never freed. Moves keep the count, copies/hand-outs retain, MIR
// drops release; over-retaining only leaks.
// -DKIGUMI_RC_CHECK quarantines freed objects instead of freeing them and
// panics on later use, standing in for a sanitizer.
void rt_panic(const char* msg);
#ifdef KIGUMI_RC_CHECK
#define CHK(v)                                           \
    do {                                                 \
        if ((v) && (v)->kind == 0) {                     \
            rt_sys_write(2, "use after free in ", 18);   \
            rt_sys_write(2, __func__, strlen(__func__)); \
            rt_sys_write(2, "\n", 1);                    \
            rt_sys_exit(2);                              \
        }                                                \
    } while (0)
#else
#define CHK(v) \
    do {       \
    } while (0)
#endif
val rt_retain(val v) {
    CHK(v);
    if (v && v->rc > 0) v->rc++;
    return v;
}
static void free_obj(val v);
void rt_release(val v) {
    CHK(v);
    if (!v || v->rc <= 0) return;
    if (--v->rc == 0) free_obj(v);
}
void rt_panic(const char* msg) {
    rt_sys_write(2, "panic: ", 7);
    rt_sys_write(2, msg, strlen(msg));
    rt_sys_write(2, "\n", 1);
    rt_sys_exit(2);
}
// panicf's every caller passes a fixed message with exactly one "%s"; this
// avoids a full printf just for that one substitution.
void panicf(const char* fmt, const char* a) {
    char buf[512];
    size_t n = 0;
    for (const char* f = fmt; *f && n + 1 < sizeof buf; f++) {
        if (f[0] == '%' && f[1] == 's') {
            for (const char* s = a; *s && n + 1 < sizeof buf; s++) buf[n++] = *s;
            f++;
            continue;
        }
        buf[n++] = *f;
    }
    buf[n] = 0;
    rt_panic(buf);
}

val rt_unit(void) {
    static struct obj u = {.kind = K_UNIT, .rc = -1};
    return &u;
}
val rt_int(int64_t i, int nk) {
    val v = alloc(K_INT);
    v->u.i = i;
    v->nk = nk ? nk : (64 | 256);
    return v;
}
// An f32 value is stored as the double nearest to its single-precision
// rounding, so every operation on it rounds the way IEEE single does.
val rt_float(double f, int nk) {
    val v = alloc(K_FLOAT);
    v->u.f = (nk & 255) == 32 ? (double)(float)f : f;
    v->nk = nk;
    return v;
}
val rt_bool(int b) {
    static struct obj t = {K_BOOL, 0, -1, 0, NULL, {.b = 1}}, f = {K_BOOL, 0, -1, 0, NULL, {.b = 0}};
    return b ? &t : &f;
}
val rt_char(uint32_t c) {
    val v = alloc(K_CHAR);
    v->u.c = c;
    return v;
}
val rt_str(const char* data, int64_t len) {
    val v = alloc(K_STR);
    v->u.s.data = data;
    v->u.s.len = len;
    return v;
}
val rt_bytes(const char* data, int64_t len) {
    val v = alloc(K_BYTES);
    v->u.s.data = data;
    v->u.s.len = len;
    return v;
}
val rt_str_take(char* data, int64_t len) {
    val v = rt_str(data, len);
    v->owned = 1;
    return v;
}
val strdup_val(const char* s, int64_t len) {
    if (len < 0) rt_panic("out of memory");
    char* d = malloc((uint64_t)len + 1);
    if (!d) rt_panic("out of memory");
    memcpy(d, s, len);
    d[len] = 0;
    return rt_str_take(d, len);
}
// NULL on refusal instead of strdup_val's panic, for the fallible primitives.
static char* strdup_or_null(const char* s, int64_t len) {
    if (len < 0) return NULL;
    char* d = malloc((uint64_t)len + 1);
    if (!d) return NULL;
    memcpy(d, s, len);
    d[len] = 0;
    return d;
}
val bytes_val(const char* s, int64_t len) {
    val v = strdup_val(s, len);
    v->kind = K_BYTES;
    return v;
}
val rt_cell(val x) {
    val v = alloc(K_CELL);
    v->u.cell.v = x;
    return v;
}
val rt_cellget(val c) {
    return rt_retain(c->u.cell.v);
}
void rt_cellset(val c, val x) {
    val old = c->u.cell.v;
    c->u.cell.v = x;
    rt_release(old);
}
val deref(val v);
val rt_ptr(void* p) {
    val v = alloc(K_PTR);
    v->u.p = p;
    return v;
}
void* rt_ptr_val(val v) {
    v = deref(v);
    return v->kind == K_PTR ? v->u.p : NULL;
}
val opaque(const char* kind, void* data) {
    val v = alloc(K_OPAQUE);
    v->u.op.kind = kind;
    v->u.op.data = data;
    return v;
}

val rt_array(int64_t n, val* items) {
    val v = alloc(K_ARRAY);
    v->u.arr.cap = n < 4 ? 4 : n;
    v->u.arr.items = amalloc(v->al, v->u.arr.cap * sizeof(val));
    for (int64_t i = 0; i < n; i++) v->u.arr.items[i] = items[i];
    v->u.arr.len = n;
    return v;
}
// try_grow makes room for extra more items; 0 when the allocator refuses.
static int try_grow(val a, uint64_t extra) {
    uint64_t len = (uint64_t)a->u.arr.len;
    if (extra > UINT64_MAX - len) return 0;
    uint64_t need = len + extra;
    if (need <= (uint64_t)a->u.arr.cap) return 1;
    if (need > SIZE_MAX / sizeof(val) / 2) return 0;
    uint64_t cap = (uint64_t)a->u.arr.cap * 2 > need ? (uint64_t)a->u.arr.cap * 2 : need;
    val* items = arealloc(a->al, a->u.arr.items, a->u.arr.len * sizeof(val), cap * sizeof(val));
    if (!items) return 0;
    a->u.arr.items = items;
    a->u.arr.cap = (int64_t)cap;
    return 1;
}
void push(val a, val x) {
    if (!try_grow(a, 1)) rt_panic("out of memory");
    a->u.arr.items[a->u.arr.len++] = x;
}
val rt_at(val a, int64_t i) {
    if (!a || i < 0 || i >= a->u.arr.len) return rt_unit();
    return rt_retain(a->u.arr.items[i]);
}
val rt_record(rt_type* t, int64_t n, val* fields) {
    val v = alloc(K_RECORD);
    v->u.rec.type = t;
    v->u.rec.n = n;
    v->u.rec.fields = amalloc(v->al, (n ? n : 1) * sizeof(val));
    for (int64_t i = 0; i < n; i++) {
        v->u.rec.fields[i] = fields[i];
    }
    return v;
}
val rt_variant(rt_vdesc* d, int64_t n, val* payload) {
    val v = alloc(K_VARIANT);
    v->u.var.v = d;
    v->u.var.n = n;
    v->u.var.payload = amalloc(v->al, (n ? n : 1) * sizeof(val));
    for (int64_t i = 0; i < n; i++) {
        v->u.var.payload[i] = payload[i];
    }
    return v;
}
val deref(val v) {
    CHK(v);
    while (v && v->kind == K_CELL) {
        v = v->u.cell.v;
        CHK(v);
    }
    return v;
}
// rt_box_replace swaps dst's content with v's in place, keeping dst's
// identity (rc/al stay put) so every other holder of dst sees the new
// value; v ends up holding dst's old content, released normally through
// its own drop(). Used for `self = newValue` on a receiver too wide a
// shape for field-by-field (a multi-variant ADT, or a resource).
void rt_box_replace(val dst, val v) {
    dst = deref(dst);
    v = deref(v);
    int dst_rc = dst->rc, v_rc = v->rc;
    rt_allocator *dst_al = dst->al, *v_al = v->al;
    struct obj tmp = *dst;
    *dst = *v;
    *v = tmp;
    dst->rc = dst_rc;
    dst->al = dst_al;
    v->rc = v_rc;
    v->al = v_al;
    rt_release(v);
}
val rt_field(val r, int64_t i) {
    r = deref(r);
    if (r->kind == K_BOX) r = r->u.box.v;
    return rt_retain(r->u.rec.fields[i]);
}
// Takes field i out of r without a retain: r's slot goes NULL, which
// rt_release already treats as a no-op, so the field drops once through
// whichever of r or the returned value reaches a drop first.
val rt_field_move(val r, int64_t i) {
    r = deref(r);
    if (r->kind == K_BOX) r = r->u.box.v;
    val v = r->u.rec.fields[i];
    r->u.rec.fields[i] = NULL;
    return v;
}
// Storing into a place releases what it held: the caller hands over an owned
// reference, so without this the overwritten value is never dropped.
void rt_setfield(val r, int64_t i, val x) {
    r = deref(r);
    val old = r->u.rec.fields[i];
    r->u.rec.fields[i] = x;
    rt_release(old);
}
val rt_payload(val v, int64_t i) {
    v = deref(v);
    return rt_retain(v->u.var.payload[i]);
}
val rt_isvariant(val v, rt_vdesc* d) {
    v = deref(v);
    return rt_bool(v->kind == K_VARIANT && v->u.var.v == d);
}
val rt_box(val v, rt_type* dyn, void** vt) {
    if (v->kind == K_BOX) return v;
    val b = alloc(K_BOX);
    b->u.box.dyn = dyn;
    b->u.box.v = v;
    b->u.box.vt = vt;
    return b;
}
val rt_unbox(val b) {
    return b->kind == K_BOX ? rt_retain(b->u.box.v) : b;
}
val rt_istype(val b, rt_type* t) {
    b = deref(b);
    return rt_bool(b->kind == K_BOX && b->u.box.dyn == t);
}
// std/error.as[E]: e is E(x) doesn't type-check over a type parameter
// (E101), so this runtime downcast runs here instead. target
// and ctx are compile-time descriptors; wrapped is Context's "wrapped"
// field index, so a field reorder in error.kg still walks the right slot.
// sem requires E: Copy, so the match is duplicated with rt_copy, never
// aliased into the caller's borrow.
val rt_error_as(val e, rt_type* target, rt_type* ctx, int64_t wrapped) {
    val cur = deref(e);
    for (;;) {
        if (cur->kind != K_BOX) return none();
        if (cur->u.box.dyn == target) return some(rt_copy(cur->u.box.v));
        if (!ctx || cur->u.box.dyn != ctx) return none();
        cur = deref(cur->u.box.v->u.rec.fields[wrapped]);
    }
}
static int boundary(const char* s, int64_t n, int64_t i) {
    return i == n || (s[i] & 0xC0) != 0x80;
}
val rt_index(val base, val idx) {
    base = deref(base);
    idx = deref(idx);
    if (idx->kind == K_RECORD) {
        int64_t lo = idx->u.rec.fields[0]->u.i, hi = idx->u.rec.fields[1]->u.i;
        if (idx->u.rec.fields[2]->u.b) hi++;
        if (base->kind == K_STR || base->kind == K_BYTES) {
            if (lo < 0 || hi > base->u.s.len || lo > hi || (base->kind == K_STR && (!boundary(base->u.s.data, base->u.s.len, lo) || !boundary(base->u.s.data, base->u.s.len, hi)))) rt_panic("slice is not on character boundaries");
            val r = strdup_val(base->u.s.data + lo, hi - lo);
            r->kind = base->kind;
            return r;
        }
        if (lo < 0 || hi > base->u.arr.len || lo > hi) rt_panic("slice out of range");
        {
            val out = rt_array(hi - lo, base->u.arr.items + lo);
            for (int64_t j = 0; j < out->u.arr.len; j++) rt_retain(out->u.arr.items[j]);
            return out;
        }
    }
    int64_t i = idx->u.i;
    if (base->kind == K_STR || base->kind == K_BYTES) {
        if (i < 0 || i >= base->u.s.len) rt_panic("index out of range");
        return rt_int((unsigned char)base->u.s.data[i], 8);
    }
    if (base->kind != K_ARRAY || i < 0 || i >= base->u.arr.len) rt_panic("index out of range");
    return rt_retain(base->u.arr.items[i]);
}
void rt_setindex(val base, val idx, val v) {
    base = deref(base);
    int64_t i = deref(idx)->u.i;
    if (base->kind != K_ARRAY || i < 0 || i >= base->u.arr.len) rt_panic("index out of range");
    val old = base->u.arr.items[i];
    base->u.arr.items[i] = v;
    rt_release(old);
}
int64_t rt_int_val(val v) {
    v = deref(v);
    return v->kind == K_INT ? v->u.i : 0;
}
double rt_float_val(val v) {
    v = deref(v);
    return v->kind == K_FLOAT ? v->u.f : (double)v->u.i;
}
uint32_t rt_char_val(val v) {
    v = deref(v);
    return v->kind == K_CHAR ? v->u.c : 0;
}
val rt_from_cstr(const char* s) {
    return s ? rt_str(s, strlen(s)) : rt_str("", 0);
}
int rt_truth(val v) {
    v = deref(v);
    return v->kind == K_BOOL && v->u.b;
}
// Captures may alias live locals (method values, borrowed captures), so
// the environment retains them.
val rt_closure(adapter_fn fn, int n, val* caps) {
    val v = alloc(K_CLOSURE);
    v->u.clo.fn = fn;
    v->u.clo.env = rt_array(n, caps);
    for (int i = 0; i < n; i++) rt_retain(caps[i]);
    return v;
}

// rt_closure_async is rt_closure for an async fn/method used as a value
// (A25-b): rt_callv on the result builds a Future instead of running fn.
val rt_closure_async(adapter_fn fn, int n, val* caps) {
    val v = rt_closure(fn, n, caps);
    v->u.clo.async = 1;
    return v;
}

val rt_copy(val v) {
    if (!v) return v;
    switch (v->kind) {
    case K_RECORD: {
        if (v->u.rec.type->kind == 1) return rt_retain(v);
        val c = rt_record(v->u.rec.type, v->u.rec.n, v->u.rec.fields);
        uint64_t bf = v->u.rec.type->borrow_fields;
        for (int64_t i = 0; i < c->u.rec.n; i++) {
            if ((bf >> i) & 1) continue;
            c->u.rec.fields[i] = rt_copy(c->u.rec.fields[i]);
        }
        return c;
    }
    case K_VARIANT: {
        val c = rt_variant(v->u.var.v, v->u.var.n, v->u.var.payload);
        for (int64_t i = 0; i < c->u.var.n; i++) c->u.var.payload[i] = rt_copy(c->u.var.payload[i]);
        return c;
    }
    case K_ARRAY: {
        val c = rt_array(v->u.arr.len, v->u.arr.items);
        for (int64_t i = 0; i < c->u.arr.len; i++) c->u.arr.items[i] = rt_copy(c->u.arr.items[i]);
        return c;
    }
    case K_BOX: {
        val c = alloc(K_BOX);
        c->u.box.dyn = v->u.box.dyn;
        c->u.box.vt = v->u.box.vt;
        c->u.box.v = rt_copy(v->u.box.v);
        return c;
    }
    }
    return rt_retain(v);
}

int equal(val a, val b) {
    a = deref(a);
    b = deref(b);
    if (a->kind != b->kind) return 0;
    switch (a->kind) {
    case K_INT:
        return a->u.i == b->u.i;
    case K_FLOAT:
        return a->u.f == b->u.f;
    case K_BOOL:
        return a->u.b == b->u.b;
    case K_CHAR:
        return a->u.c == b->u.c;
    case K_UNIT:
        return 1;
    case K_PTR:
        return a->u.p == b->u.p;
    case K_STR:
    case K_BYTES:
        return a->u.s.len == b->u.s.len && memcmp(a->u.s.data, b->u.s.data, a->u.s.len) == 0;
    }
    // Records, ADTs and arrays compare through their `equals`.
    return a == b;
}

// ---- strings and display ----
void bput(buf* b, const char* s, size_t n) {
    if (b->len + n + 1 > b->cap) {
        if (n > SIZE_MAX - 1 || b->len > SIZE_MAX - 1 - n) rt_panic("out of memory");
        size_t need = b->len + n + 1;
        if (need > (SIZE_MAX - 16) / 2) rt_panic("out of memory");
        size_t cap = need * 2 + 16;
        char* p = realloc(b->p, cap);
        if (!p) rt_panic("out of memory");
        b->p = p;
        b->cap = cap;
    }
    memcpy(b->p + b->len, s, n);
    b->len += n;
    b->p[b->len] = 0;
}
void bputs(buf* b, const char* s) {
    bput(b, s, strlen(s));
}
static const char* short_name(const char* name) {
    const char* d = strrchr(name, '.');
    return d ? d + 1 : name;
}
// Base-10 text of v into out (NUL-terminated); out must hold 21 bytes.
static void u64_dec(uint64_t v, char* out) {
    char tmp[20];
    int n = 0;
    do {
        tmp[n++] = (char)('0' + v % 10);
        v /= 10;
    } while (v);
    for (int i = 0; i < n; i++) out[i] = tmp[n - 1 - i];
    out[n] = 0;
}
static void i64_dec(int64_t v, char* out) {
    if (v >= 0) {
        u64_dec((uint64_t)v, out);
        return;
    }
    *out++ = '-';
    // INT64_MIN negated overflows int64_t; take the magnitude via uint64_t.
    u64_dec(v == INT64_MIN ? (uint64_t)INT64_MAX + 1 : (uint64_t)(-v), out);
}
// "0x" + lowercase hex of p, no leading zeros (out must hold 2+16+1 bytes).
static void ptr_hex(const void* p, char* out) {
    uintptr_t v = (uintptr_t)p;
    *out++ = '0';
    *out++ = 'x';
    char tmp[16];
    int n = 0;
    do {
        tmp[n++] = "0123456789abcdef"[v & 0xf];
        v >>= 4;
    } while (v);
    for (int i = 0; i < n; i++) out[i] = tmp[n - 1 - i];
    out[n] = 0;
}
// canon collapses -0.0 to 0.0 when rendering K_FLOAT, so hash_of's digest
// agrees with equal()'s 0.0 == -0.0; display()
// and rt_interp() render normally with canon 0.
void display_into(buf* b, val v, int canon) {
    char tmp[64];
    v = deref(v);
    switch (v->kind) {
    case K_INT:
        if (v->nk & 256) i64_dec(v->u.i, tmp);
        else u64_dec((uint64_t)v->u.i, tmp);
        bputs(b, tmp);
        break;
    case K_FLOAT: {
        char text[64];
        double f = v->u.f;
        if (canon && f == 0.0) f = 0.0;
        rt_format_float(f, (v->nk & 255) == 32, text, sizeof text);
        bputs(b, text);
        break;
    }
    case K_BOOL:
        bputs(b, v->u.b ? "true" : "false");
        break;
    case K_CHAR: {
        uint32_t c = v->u.c;
        char u[5] = {0};
        if (c < 0x80) u[0] = c;
        else if (c < 0x800) {
            u[0] = 0xC0 | (c >> 6);
            u[1] = 0x80 | (c & 0x3F);
        } else if (c < 0x10000) {
            u[0] = 0xE0 | (c >> 12);
            u[1] = 0x80 | ((c >> 6) & 0x3F);
            u[2] = 0x80 | (c & 0x3F);
        } else {
            u[0] = 0xF0 | (c >> 18);
            u[1] = 0x80 | ((c >> 12) & 0x3F);
            u[2] = 0x80 | ((c >> 6) & 0x3F);
            u[3] = 0x80 | (c & 0x3F);
        }
        bputs(b, u);
        break;
    }
    case K_STR:
    case K_BYTES:
        bput(b, v->u.s.data, v->u.s.len);
        break;
    case K_UNIT:
        bputs(b, "()");
        break;
    case K_PTR:
        bputs(b, "<ptr ");
        ptr_hex(v->u.p, tmp);
        bputs(b, tmp);
        bputs(b, ">");
        break;
    case K_VARIANT:
        bputs(b, v->u.var.v->name);
        if (v->u.var.n) {
            bputs(b, "(");
            for (int64_t i = 0; i < v->u.var.n; i++) {
                if (i) bputs(b, ", ");
                display_into(b, v->u.var.payload[i], canon);
            }
            bputs(b, ")");
        }
        break;
    case K_RECORD:
        bputs(b, short_name(v->u.rec.type->name));
        bputs(b, " {");
        for (int64_t i = 0; i < v->u.rec.n; i++) {
            if (i) bputs(b, ",");
            bputs(b, " ");
            display_into(b, v->u.rec.fields[i], canon);
        }
        bputs(b, " }");
        break;
    case K_ARRAY:
        bputs(b, "[");
        for (int64_t i = 0; i < v->u.arr.len; i++) {
            if (i) bputs(b, ", ");
            display_into(b, v->u.arr.items[i], canon);
        }
        bputs(b, "]");
        break;
    case K_BOX:
        display_into(b, v->u.box.v, canon);
        break;
    case K_OPAQUE:
        if (strcmp(v->u.op.kind, "Error") == 0 || strcmp(v->u.op.kind, "Path") == 0) bputs(b, (const char*)v->u.op.data);
        else {
            bputs(b, "<");
            bputs(b, v->u.op.kind);
            bputs(b, ">");
        }
        break;
    default:
        bputs(b, "<value>");
    }
}
val display(val v) {
    buf b = {0};
    bputs(&b, "");
    display_into(&b, v, 0);
    return rt_str_take(b.p, b.len);
}
val rt_interp(int n, val* items) {
    buf b = {0};
    bputs(&b, "");
    for (int i = 0; i < n; i++) display_into(&b, items[i], 0);
    return rt_str_take(b.p, b.len);
}
const char* rt_cstr(val s);
const char* cstr(val s) {
    return rt_cstr(s);
}
const char* rt_cstr(val s) {
    s = deref(s);
    if (s->kind == K_BOX) s = s->u.box.v;
    if (s->kind == K_OPAQUE) return (const char*)s->u.op.data;
    if (s->u.s.len < 0) rt_panic("out of memory");
    char* d = malloc((uint64_t)s->u.s.len + 1);
    if (!d) rt_panic("out of memory");
    memcpy(d, s->u.s.data, s->u.s.len);
    d[s->u.s.len] = 0;
    return d;
}

// ---- numbers ----
static int bits_of(int nk) {
    return nk & 255;
}
static int is_signed(int nk) {
    return (nk >> 8) & 1;
}
// A width-64 shift into the sign bit is signed-shift UB; INT64_MIN sidesteps it.
static int64_t min_of(int nk) {
    if (!is_signed(nk)) return 0;
    int b = bits_of(nk);
    return b >= 64 ? INT64_MIN : -((int64_t)1 << (b - 1));
}
static uint64_t max_of(int nk) {
    int b = bits_of(nk);
    if (is_signed(nk)) return b >= 64 ? (uint64_t)INT64_MAX : ((uint64_t)1 << (b - 1)) - 1;
    if (b >= 64) return ~(uint64_t)0;
    return ((uint64_t)1 << b) - 1;
}
static int64_t truncate_to(int64_t v, int nk) {
    int b = bits_of(nk);
    if (b >= 64) return v;
    int64_t mask = ((int64_t)1 << b) - 1;
    v &= mask;
    if (is_signed(nk) && (v & ((int64_t)1 << (b - 1)))) v -= (int64_t)1 << b;
    return v;
}
static int64_t checked_i64(int64_t v, int nk, int over) {
    if (!over) {
        if (is_signed(nk)) over = v < min_of(nk) || v > (int64_t)max_of(nk);
        else if (bits_of(nk) < 64) over = v < 0 || (uint64_t)v > max_of(nk);
    }
    if (over) rt_panic("integer overflow");
    return v;
}
static val checked(int64_t v, int nk, int over) {
    return rt_int(checked_i64(v, nk, over), nk);
}
// rt_i*: the scalar (unboxed) core of rt_binop/rt_unop's integer cases,
// shared so llgen's native-register codegen computes exactly what the
// boxed path does, minus the box.
int64_t rt_iadd(int64_t x, int64_t y, int nk) {
    int sg = is_signed(nk);
    int64_t r;
    int over = sg ? __builtin_add_overflow(x, y, &r) : (bits_of(nk) == 64 && (uint64_t)(x + y) < (uint64_t)x);
    if (!sg) r = x + y;
    return checked_i64(r, nk, over);
}
int64_t rt_isub(int64_t x, int64_t y, int nk) {
    int sg = is_signed(nk);
    int64_t r;
    int over = sg ? __builtin_sub_overflow(x, y, &r) : ((uint64_t)x < (uint64_t)y);
    if (!sg) r = x - y;
    return checked_i64(r, nk, over);
}
int64_t rt_imul(int64_t x, int64_t y, int nk) {
    int sg = is_signed(nk);
    int64_t r;
    int over;
    if (sg) {
        over = __builtin_mul_overflow(x, y, &r);
    } else {
        uint64_t p;
        over = __builtin_mul_overflow((uint64_t)x, (uint64_t)y, &p);
        r = (int64_t)p;
    }
    return checked_i64(r, nk, over);
}
int64_t rt_idiv(int64_t x, int64_t y, int nk) {
    if (y == 0) rt_panic("division by zero");
    if (is_signed(nk) && y == -1 && x == min_of(nk)) rt_panic("integer overflow");
    if (is_signed(nk)) return x / y;
    return (int64_t)((uint64_t)x / (uint64_t)y);
}
int64_t rt_imod(int64_t x, int64_t y, int nk) {
    if (y == 0) rt_panic("division by zero");
    if (is_signed(nk) && y == -1 && x == min_of(nk)) rt_panic("integer overflow");
    if (is_signed(nk)) return x % y;
    return (int64_t)((uint64_t)x % (uint64_t)y);
}
int64_t rt_ixor(int64_t x, int64_t y, int nk) {
    return checked_i64(x ^ y, nk, 0);
}
int64_t rt_ishl(int64_t x, int64_t y, int nk) {
    if (y < 0 || y >= bits_of(nk)) rt_panic("shift count out of range");
    return checked_i64((int64_t)((uint64_t)x << y), nk, 0);
}
int64_t rt_ishr(int64_t x, int64_t y, int nk) {
    if (y < 0 || y >= bits_of(nk)) rt_panic("shift count out of range");
    return is_signed(nk) ? x >> y : (int64_t)((uint64_t)x >> y);
}
int64_t rt_ineg(int64_t x, int nk) {
    return checked_i64((int64_t)(0 - (uint64_t)x), nk, x == min_of(nk));
}
int64_t rt_inot(int64_t x, int nk) {
    return truncate_to(~x, nk);
}
// prelude.<T>.wrapping* / rotate*: modular arithmetic and shifts masked to the width.
static val int_fn(const char* k, val a0, val a1) {
    const char* op = strrchr(k, '.');
    if (!op++) return NULL;
    int nk = a0->nk;
    int b = bits_of(nk);
    uint64_t x = (uint64_t)a0->u.i, y = (uint64_t)a1->u.i;
    if (!strcmp(op, "wrappingAdd")) return rt_int(truncate_to((int64_t)(x + y), nk), nk);
    if (!strcmp(op, "wrappingSub")) return rt_int(truncate_to((int64_t)(x - y), nk), nk);
    if (!strcmp(op, "wrappingMul")) return rt_int(truncate_to((int64_t)(x * y), nk), nk);
    unsigned c = (unsigned)(y & (uint64_t)(b - 1));
    uint64_t ux = b == 64 ? x : x & (((uint64_t)1 << b) - 1);
    if (!strcmp(op, "wrappingShiftLeft")) return rt_int(truncate_to((int64_t)(ux << c), nk), nk);
    if (!strcmp(op, "wrappingShiftRight")) return rt_int(truncate_to(is_signed(nk) ? truncate_to((int64_t)ux, nk) >> c : (int64_t)(ux >> c), nk), nk);
    if (!strcmp(op, "rotateLeft")) return rt_int(truncate_to((int64_t)(c ? (ux << c) | (ux >> (b - c)) : ux), nk), nk);
    if (!strcmp(op, "rotateRight")) return rt_int(truncate_to((int64_t)(c ? (ux >> c) | (ux << (b - c)) : ux), nk), nk);
    return NULL;
}
static int cmp_vals(val a, val b) {
    a = deref(a);
    b = deref(b);
    switch (a->kind) {
    case K_INT:
        if (is_signed(a->nk)) return (a->u.i > b->u.i) - (a->u.i < b->u.i);
        return ((uint64_t)a->u.i > (uint64_t)b->u.i) - ((uint64_t)a->u.i < (uint64_t)b->u.i);
    case K_FLOAT:
        return (a->u.f > b->u.f) - (a->u.f < b->u.f);
    case K_CHAR:
        return (a->u.c > b->u.c) - (a->u.c < b->u.c);
    case K_BOOL:
        return a->u.b - b->u.b;
    case K_STR:
    case K_BYTES: {
        int64_t n = a->u.s.len < b->u.s.len ? a->u.s.len : b->u.s.len;
        int c = memcmp(a->u.s.data, b->u.s.data, n);
        if (c) return c < 0 ? -1 : 1;
        return (a->u.s.len > b->u.s.len) - (a->u.s.len < b->u.s.len);
    }
    }
    return 0;
}
// float_total_equal/float_total_compare are the Eq/Ord protocol's `f64`/
// `f32` total order: every NaN
// equals every other NaN and sorts above every non-NaN value; -0.0 equals
// 0.0. `==`/`<`/... in rt_binop stay IEEE and do not call these.
static int float_total_equal(double a, double b) {
    int an = a != a, bn = b != b;
    if (an || bn) return an && bn;
    if (a == 0.0) a = 0.0;
    if (b == 0.0) b = 0.0;
    return a == b;
}
static int float_total_compare(double a, double b) {
    int an = a != a, bn = b != b;
    if (an && bn) return 0;
    if (an) return 1;
    if (bn) return -1;
    if (a == 0.0) a = 0.0;
    if (b == 0.0) b = 0.0;
    return (a > b) - (a < b);
}
static int in_range(int64_t v, int s, int d) {
    if (!is_signed(s) && v < 0) return 0;
    if (is_signed(d)) return v >= min_of(d) && v <= (int64_t)max_of(d);
    return v >= 0 && (bits_of(d) == 64 || (uint64_t)v <= max_of(d));
}
// rt_convert is the prelude's numeric `toX`: dnk is the target kind (bit 10
// for Char), checked says the declaration returns an Option.
val rt_convert(val a0, int dnk, int checked) {
    a0 = deref(a0);
    if (a0->kind == K_BOX) a0 = a0->u.box.v;
    if (dnk & 1024) return rt_char((uint32_t)a0->u.i);
    if (a0->kind == K_CHAR) return rt_int(a0->u.c, dnk);
    if (dnk & 512) {
        if (a0->kind == K_FLOAT) return rt_float(a0->u.f, dnk);
        return rt_float(is_signed(a0->nk) ? (double)a0->u.i : (double)(uint64_t)a0->u.i, dnk);
    }
    if (a0->kind == K_FLOAT) {
        double x = trunc(a0->u.f);
        if (x != x || x < -9223372036854775808.0 || x >= 9223372036854775808.0) return none();
        return some(rt_int((int64_t)x, dnk));
    }
    if (!checked) return rt_int(a0->u.i, dnk);
    if (!in_range(a0->u.i, a0->nk, dnk)) return none();
    return some(rt_int(a0->u.i, dnk));
}
static val range_new(val lo, val hi, int inclusive);
val rt_binop(const char* op, val a, val b) {
    a = deref(a);
    b = deref(b);
    if (!strcmp(op, "==")) return rt_bool(equal(a, b));
    if (!strcmp(op, "!=")) return rt_bool(!equal(a, b));
    if (!strcmp(op, "..")) return range_new(a, b, 0);
    if (!strcmp(op, "..=")) return range_new(a, b, 1);
    if (a->kind != K_FLOAT) {
        if (!strcmp(op, "<")) return rt_bool(cmp_vals(a, b) < 0);
        if (!strcmp(op, "<=")) return rt_bool(cmp_vals(a, b) <= 0);
        if (!strcmp(op, ">")) return rt_bool(cmp_vals(a, b) > 0);
        if (!strcmp(op, ">=")) return rt_bool(cmp_vals(a, b) >= 0);
    }
    if (a->kind == K_FLOAT) {
        double x = a->u.f, y = b->u.f;
        /* IEEE ordering: every comparison with NaN is false, which a three-way cmp_vals cannot say. */
        if (!strcmp(op, "<")) return rt_bool(x < y);
        if (!strcmp(op, "<=")) return rt_bool(x <= y);
        if (!strcmp(op, ">")) return rt_bool(x > y);
        if (!strcmp(op, ">=")) return rt_bool(x >= y);
        switch (op[0]) {
        case '+':
            return rt_float(x + y, a->nk);
        case '-':
            return rt_float(x - y, a->nk);
        case '*':
            return rt_float(x * y, a->nk);
        case '/':
            return rt_float(x / y, a->nk);
        }
    }
    if (a->kind != K_INT) rt_panic("unsupported operands");
    int64_t x = a->u.i, y = b->u.i;
    int nk = a->nk;
    switch (op[0]) {
    case '+':
        return rt_int(rt_iadd(x, y, nk), nk);
    case '-':
        return rt_int(rt_isub(x, y, nk), nk);
    case '*':
        return rt_int(rt_imul(x, y, nk), nk);
    case '/':
        return rt_int(rt_idiv(x, y, nk), nk);
    case '%':
        return rt_int(rt_imod(x, y, nk), nk);
    case '&':
        return rt_int(x & y, nk);
    case '|':
        return rt_int(x | y, nk);
    case '^':
        return rt_int(rt_ixor(x, y, nk), nk);
    case '<':
        return rt_int(rt_ishl(x, y, nk), nk);
    case '>':
        return rt_int(rt_ishr(x, y, nk), nk);
    }
    panicf("unsupported operator %s", op);
    return NULL;
}
val rt_unop(const char* op, val a) {
    a = deref(a);
    if (op[0] == '!') return rt_bool(!a->u.b);
    if (op[0] == '-') {
        if (a->kind == K_FLOAT) return rt_float(-a->u.f, a->nk);
        return rt_int(rt_ineg(a->u.i, a->nk), a->nk);
    }
    if (op[0] == '~') return rt_int(rt_inot(a->u.i, a->nk), a->nk);
    panicf("unsupported unary %s", op);
    return NULL;
}
val rt_cast(val a, int nk) {
    a = deref(a);
    if (nk & 512) return rt_float(a->kind == K_FLOAT ? a->u.f : (double)a->u.i, nk);
    return rt_int(a->u.i, nk);
}

// ---- ADT helpers and init ----
rt_type* find_type(const char* name) {
    for (int i = 0; i < g_ntypes; i++)
        if (!strcmp(short_name(g_types[i]->name), name) || !strcmp(g_types[i]->name, name)) return g_types[i];
    return NULL;
}
static rt_vdesc* find_variant(rt_type* t, const char* name) {
    if (!t) return NULL;
    for (int i = 0; i < t->nvariants; i++)
        if (!strcmp(t->variants[i]->name, name)) return t->variants[i];
    return NULL;
}
void rt_init(int argc, char** argv, int ntypes, rt_type** types) {
    g_argc = argc;
    g_argv = argv;
    g_ntypes = ntypes;
    g_types = types;
    rt_type *opt = find_type("Option"), *res = find_type("Result"), *ord = find_type("Ordering");
    V_Some = find_variant(opt, "Some");
    V_None = find_variant(opt, "None");
    V_Ok = find_variant(res, "Ok");
    V_Err = find_variant(res, "Err");
    V_Less = find_variant(ord, "Less");
    V_Equal = find_variant(ord, "Equal");
    V_Greater = find_variant(ord, "Greater");
}
val some(val v) {
    return rt_variant(V_Some, 1, &v);
}
val none(void) {
    return rt_variant(V_None, 0, NULL);
}
val ok(val v) {
    return rt_variant(V_Ok, 1, &v);
}
val err_val(val e) {
    return rt_variant(V_Err, 1, &e);
}
val err_msg(const char* msg) {
    val e = rt_box(opaque("Error", (void*)msg), &T_Error, T_Error_vt);
    return err_val(e);
}
// Err(AllocError.OutOfMemory); looked up on demand since it is rare, unlike
// the V_* variants cached at init.
static val alloc_error(void) {
    rt_vdesc* d = find_variant(find_type("AllocError"), "OutOfMemory");
    return d ? err_val(rt_variant(d, 0, NULL)) : err_msg("out of memory");
}
static val ordering(int c) {
    return rt_variant(c < 0 ? V_Less : c > 0 ? V_Greater
                                             : V_Equal,
                      0,
                      NULL);
}
// `..`/`..=` are binary operators, so their operands are not consumed by
// the emitter and the range must retain them.
static val range_new(val lo, val hi, int inclusive) {
    rt_type* t = find_type("Range");
    val f[3] = {rt_retain(lo), rt_retain(hi), rt_bool(inclusive)};
    return rt_record(t, 3, f);
}
val make_record(const char* tname, int n, val* fields) {
    rt_type* t = find_type(tname);
    if (!t) panicf("runtime type %s missing", tname);
    return rt_record(t, n, fields);
}

val rt_call_slot(val recv, int slot, int n, val* args);
val rt_std(const char* key, int n, val* args);
static val error_message(val e) {
    return rt_call_slot(e, 0, 0, NULL);
}
int rt_exit_code(val r) {
    r = deref(r);
    if (r && r->kind == K_VARIANT && r->u.var.v == V_Err) {
        val m = display(error_message(r->u.var.payload[0]));
        rt_sys_write(2, "error: ", 7);
        rt_sys_write(2, m->u.s.data, m->u.s.len);
        rt_sys_write(2, "\n", 1);
        return 1;
    }
    return 0;
}
// invoke runs an adapter on a fresh argument array and releases the array
// afterwards: the adapter retained what it bound, so the caller's references
// in the array are consumed here.
val invoke(adapter_fn fn, val env, int n, val* args) {
    val arr = rt_array(n, args);
    val r = fn(env, arr);
    rt_release(arr);
    return r;
}
// A call's argument count has no compile-time cap (interface dispatch, std
// closures, awaited futures), so anything past this stack capacity spills
// to the heap instead of writing past a fixed buffer.
#define STACK_ARGS 16
// callv_async builds a lazy Future instead of running the
// body: f is lent, so the Future keeps an independent closure of its own;
// args move into the Future unchanged.
static val callv_async(val f, int n, val* args) {
    val raw = alloc(K_CLOSURE);
    raw->u.clo.fn = f->u.clo.fn;
    raw->u.clo.env = rt_retain(f->u.clo.env);
    raw->u.clo.key = f->u.clo.key;
    raw->u.clo.lent = f->u.clo.lent;
    val stackbuf[STACK_ARGS];
    val* all = n + 1 <= STACK_ARGS ? stackbuf : malloc((n + 1) * sizeof(val));
    all[0] = raw;
    for (int i = 0; i < n; i++) all[i + 1] = args[i];
    val arr = rt_array(n + 1, all);
    if (all != stackbuf) free(all);
    return opaque("Future", arr);
}
val rt_callv(val f, int n, val* args) {
    f = deref(f);
    if (f->kind != K_CLOSURE) rt_panic("value is not callable");
    if (f->u.clo.async) return callv_async(f, n, args);
    if (f->u.clo.key) {
        int64_t envlen = f->u.clo.env->u.arr.len;
        int64_t total = envlen + n;
        val stackbuf[STACK_ARGS];
        val* all = total <= STACK_ARGS ? stackbuf : malloc(total * sizeof(val));
        int m = 0;
        for (int64_t i = 0; i < envlen; i++) all[m++] = f->u.clo.env->u.arr.items[i];
        for (int i = 0; i < n; i++) all[m++] = args[i];
        val r = rt_std(f->u.clo.key, m, all);
        for (int i = 0; i < n; i++)
            if (!((f->u.clo.lent >> i) & 1)) rt_release(args[i]);
        if (all != stackbuf) free(all);
        return r;
    }
    return invoke(f->u.clo.fn, f->u.clo.env, n, args);
}
// rt_call_slot calls requirement slot of the interface a boxed value was
// boxed as. invoke's rt_at-based unpacking retains the receiver once and
// invoke's own release balances it, matching a direct call's borrow.
val rt_call_slot(val recv, int slot, int n, val* args) {
    val target = deref(recv);
    if (target->kind != K_BOX || !target->u.box.vt) rt_panic("dynamic call on a value without a witness table");
    adapter_fn fn = (adapter_fn)target->u.box.vt[slot];
    int64_t total = n + 1;
    val stackbuf[STACK_ARGS];
    val* all = total <= STACK_ARGS ? stackbuf : malloc(total * sizeof(val));
    all[0] = target->u.box.v;
    for (int i = 0; i < n; i++) all[i + 1] = args[i];
    val r = invoke(fn, NULL, (int)total, all);
    if (all != stackbuf) free(all);
    return r;
}
val rt_std_closure(const char* key, int n, val* caps) {
    val v = rt_closure(NULL, n, caps);
    v->u.clo.key = key;
    return v;
}
// rt_std_bind is a primitive used as a witness: lent (bit i for argument
// i) marks the receiver and the borrowed parameters, which rt_callv must
// leave alone as a direct call would.
val rt_std_bind(const char* key, int lent, int n, val* caps) {
    val v = rt_std_closure(key, n, caps);
    v->u.clo.lent = lent;
    return v;
}
void rt_drop(val v) {
    rt_release(v);
}
// A Future is the function value plus its arguments; awaiting runs the call.
static val await_future(val fut) {
    val arr = (val)fut->u.op.data;
    int n = (int)arr->u.arr.len - 1;
    val stackbuf[STACK_ARGS];
    val* a = n <= STACK_ARGS ? stackbuf : malloc(n * sizeof(val));
    for (int i = 0; i < n; i++) a[i] = rt_retain(arr->u.arr.items[i + 1]);
    val r = rt_callv(arr->u.arr.items[0], n, a);
    if (a != stackbuf) free(a);
    return r;
}
// A Shared cell's control block outlives the content for weak handles.
// borrow is a RefCell-style dynamic borrow flag (0 free, -1 exclusive, >0
// shared count), since Shared's receiver is a freely-copyable handle the
// static borrow checker does not track.
typedef struct sctrl {
    val cell, shared;
    int weak, alive, borrow;
} sctrl;
// Opaque values that own runtime state: an arena handle closes its arena, a
// scope keeps its handle alive, a guard pops the allocator it pushed, and a
// Shared cell releases its content.
static void free_opaque(val v) {
    const char* kind = v->u.op.kind;
    void* d = v->u.op.data;
    if (!strcmp(kind, "AllocatorHandle")) arena_close((arena*)d);
    else if (!strcmp(kind, "Shared")) {
        sctrl* c = d;
        c->alive = 0;
        rt_release(c->cell);
        if (c->weak == 0) free(c);
    } else if (!strcmp(kind, "Weak")) {
        sctrl* c = d;
        if (--c->weak == 0 && !c->alive) free(c);
    } else if (!strcmp(kind, "AllocatorScope") || !strcmp(kind, "Future") || !strcmp(kind, "Task")) rt_release((val)d);
    else if (!strcmp(kind, "AllocGuard")) {
        g_alloc_sp--;
        rt_release((val)d);
    }
}
static void free_obj(val v) {
    switch (v->kind) {
    case K_STR:
    case K_BYTES:
        if (v->owned) free((char*)v->u.s.data);
        break;
    case K_RECORD: {
        rt_type* t = v->u.rec.type;
        if (t->kind == 1 && t->drop) {
            v->rc = -1;
            val a[1] = {v};
            invoke((adapter_fn)t->drop, NULL, 1, a);
            v->rc = 0;
        }
        for (int64_t i = v->u.rec.n - 1; i >= 0; i--) {
            if ((t->borrow_fields >> i) & 1) continue;
            rt_release(v->u.rec.fields[i]);
        }
        afree(v->al, v->u.rec.fields);
        break;
    }
    case K_VARIANT:
        for (int64_t i = 0; i < v->u.var.n; i++) rt_release(v->u.var.payload[i]);
        afree(v->al, v->u.var.payload);
        break;
    case K_ARRAY:
        for (int64_t i = v->u.arr.len - 1; i >= 0; i--) rt_release(v->u.arr.items[i]);
        afree(v->al, v->u.arr.items);
        break;
    case K_BOX:
        rt_release(v->u.box.v);
        break;
    case K_CLOSURE:
        rt_release(v->u.clo.env);
        break;
    case K_CELL:
        rt_release(v->u.cell.v);
        break;
    case K_OPAQUE:
        free_opaque(v);
        break;
    }
#ifdef KIGUMI_RC_CHECK
    v->kind = 0;
    return;
#endif
    afree(v->al, v);
}
static rt_type* map_type(void) {
    static rt_type* t;
    static int looked;
    if (!looked) {
        t = find_type("Map");
        looked = 1;
    }
    return t;
}
static val iter_len(val it) {
    it = deref(it);
    if (it->kind == K_ARRAY) return rt_int(it->u.arr.len, 64);
    if (it->kind == K_BYTES) return rt_int(it->u.s.len, 64);
    if (it->kind == K_RECORD && it->u.rec.type == map_type()) {
        return rt_int(it->u.rec.fields[0]->u.arr.len, 64);
    }
    int64_t n = it->u.rec.fields[1]->u.i - it->u.rec.fields[0]->u.i;
    if (it->u.rec.fields[2]->u.b) n++;
    return rt_int(n, 64);
}
/* A trivial scalar has no drop hook of its own: sharing it needs no
 * ownership transfer, only a retain. */
static int is_trivial_scalar(val v) {
    switch (v->kind) {
    case K_INT: case K_FLOAT: case K_BOOL: case K_STR: case K_BYTES:
    case K_CHAR: case K_UNIT:
        return 1;
    }
    return 0;
}
/* Takes ownership of *slot for the owned non-Copy case: a
 * trivial scalar (no drop of its own, and a generic loop body compiles
 * once for every T, so this also runs for a Copy instantiation such as
 * Array[T].fold with no `T: Copy` bound) is retained in place instead, so
 * a std loop re-reading the container mid-loop by index (Array.reverse)
 * still sees it. */
static val take_or_retain(val* slot) {
    if (is_trivial_scalar(*slot)) return rt_retain(*slot);
    val v = *slot;
    *slot = rt_unit();
    return v;
}
/* mode: 0 = iter.at (retains in place), 1 = iter.at_ref (borrowed, no
 * retain), 2 = iter.at_move (owned, transfers non-Copy elements). */
static val iter_at(val it, val i, int mode) {
    it = deref(it);
    if (it->kind == K_ARRAY) {
        if (mode == 1) return it->u.arr.items[i->u.i];
        if (mode == 2) return take_or_retain(&it->u.arr.items[i->u.i]);
        return rt_retain(it->u.arr.items[i->u.i]);
    }
    if (it->kind == K_BYTES) return rt_int((unsigned char)it->u.s.data[i->u.i], 8);
    if (it->kind == K_RECORD && it->u.rec.type == map_type()) {
        val keys = it->u.rec.fields[0], vals = it->u.rec.fields[1];
        rt_type* tup = find_type("Tuple2");
        if (mode == 1) {
            val f[2] = {keys->u.arr.items[i->u.i], vals->u.arr.items[i->u.i]};
            return rt_record(tup, 2, f);
        }
        if (mode == 2) {
            val f[2] = {take_or_retain(&keys->u.arr.items[i->u.i]), take_or_retain(&vals->u.arr.items[i->u.i])};
            return rt_record(tup, 2, f);
        }
        val f[2] = {rt_retain(keys->u.arr.items[i->u.i]), rt_retain(vals->u.arr.items[i->u.i])};
        return rt_record(tup, 2, f);
    }
    val lo = it->u.rec.fields[0];
    return rt_int(lo->u.i + i->u.i, lo->nk);
}
val rt_builtin(const char* name, int n, val* args) {
    if (!strcmp(name, "print")) {
        val s = display(args[0]);
        rt_sys_write(1, s->u.s.data, s->u.s.len);
        rt_sys_write(1, "\n", 1);
        rt_release(s);
        return rt_unit();
    }
    if (!strcmp(name, "eprint")) {
        val s = display(args[0]);
        rt_sys_write(2, s->u.s.data, s->u.s.len);
        rt_sys_write(2, "\n", 1);
        rt_release(s);
        return rt_unit();
    }
    if (!strcmp(name, "panic")) rt_panic(n > 0 ? cstr(args[0]) : "explicit panic");
    if (!strcmp(name, "host")) return opaque("Host", NULL);
    if (!strcmp(name, "iter.len")) return iter_len(args[0]);
    if (!strcmp(name, "iter.at")) return iter_at(args[0], deref(args[1]), 0);
    if (!strcmp(name, "iter.at_ref")) return iter_at(args[0], deref(args[1]), 1);
    if (!strcmp(name, "iter.at_move")) return iter_at(args[0], deref(args[1]), 2);
    if (!strcmp(name, "future")) {
        val arr = rt_array(n, args);
        for (int i = 0; i < n; i++) rt_retain(args[i]);
        return opaque("Future", arr);
    }
    if (!strcmp(name, "await")) return await_future(deref(args[0]));
    if (!strcmp(name, "alloc.push")) {
        val scope = deref(args[0]);
        if (g_alloc_sp + 1 >= 64) rt_panic("allocator blocks nested too deeply");
        arena* ar = (arena*)deref((val)scope->u.op.data)->u.op.data;
        g_alloc_stack[++g_alloc_sp] = &ar->a;
        return opaque_in(&((arena*)((val)scope->u.op.data)->u.op.data)->a, "AllocGuard", rt_retain(scope));
    }
    panicf("unknown builtin %s", name);
    return NULL;
}

// RFC 3629 / Unicode Table 3-7: rejects overlong encodings, surrogates
// (D800-DFFF) and codepoints above U+10FFFF, matching Go's unicode/utf8.Valid.
int utf8_valid(const char* s, int64_t n) {
    int64_t i = 0;
    while (i < n) {
        unsigned char c = s[i];
        if (c < 0x80) {
            i++;
            continue;
        }
        int len;
        uint32_t min, cp;
        if ((c & 0xE0) == 0xC0) {
            len = 2;
            min = 0x80;
            cp = c & 0x1F;
        } else if ((c & 0xF0) == 0xE0) {
            len = 3;
            min = 0x800;
            cp = c & 0x0F;
        } else if ((c & 0xF8) == 0xF0) {
            len = 4;
            min = 0x10000;
            cp = c & 0x07;
        } else {
            return 0;
        }
        if (i + len > n) return 0;
        for (int k = 1; k < len; k++) {
            unsigned char cc = s[i + k];
            if ((cc & 0xC0) != 0x80) return 0;
            cp = (cp << 6) | (cc & 0x3F);
        }
        if (cp < min || cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF)) return 0;
        i += len;
    }
    return 1;
}
// Decodes one codepoint at s[i], the same rules utf8_valid checks; returns
// the sequence length on success or 0 on any violation, so a caller can
// tell exactly where a bad sequence starts.
static int utf8_decode_one(const char* s, int64_t n, int64_t i, uint32_t* cp_out) {
    unsigned char c = s[i];
    if (c < 0x80) {
        *cp_out = c;
        return 1;
    }
    int len;
    uint32_t min, cp;
    if ((c & 0xE0) == 0xC0) {
        len = 2;
        min = 0x80;
        cp = c & 0x1F;
    } else if ((c & 0xF0) == 0xE0) {
        len = 3;
        min = 0x800;
        cp = c & 0x0F;
    } else if ((c & 0xF8) == 0xF0) {
        len = 4;
        min = 0x10000;
        cp = c & 0x07;
    } else {
        return 0;
    }
    if (i + len > n) return 0;
    for (int k = 1; k < len; k++) {
        unsigned char cc = s[i + k];
        if ((cc & 0xC0) != 0x80) return 0;
        cp = (cp << 6) | (cc & 0x3F);
    }
    if (cp < min || cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF)) return 0;
    *cp_out = cp;
    return len;
}
// Lossy UTF-8 conversion for ffi.stringLossy: each byte that
// doesn't start a valid sequence becomes its own U+FFFD, matching Go's
// utf8.DecodeRuneInString fallback, so native agrees byte for byte with
// the VM/interp Go paths.
static val utf8_lossy_val(const char* s, int64_t n) {
    buf b = {0};
    int64_t i = 0;
    while (i < n) {
        uint32_t cp;
        int len = utf8_decode_one(s, n, i, &cp);
        if (len == 0) {
            bput(&b, "\xEF\xBF\xBD", 3);
            i++;
        } else {
            bput(&b, s + i, (size_t)len);
            i += len;
        }
    }
    if (!b.p) b.p = calloc(1, 1);
    return rt_str_take(b.p, b.len);
}
// hash_of is FNV-1a-64 (standard offset basis 0xcbf29ce484222325 /
// 14695981039346656037) over the canon rendering of v, so this and
// internal/hashkey.Sum64 (interp, vm) agree byte for byte.
uint64_t hash_of(val v) {
    buf b = {0};
    bputs(&b, "");
    display_into(&b, v, 1);
    uint64_t h = 14695981039346656037ULL;
    for (size_t i = 0; i < b.len; i++) {
        h ^= (unsigned char)b.p[i];
        h *= 1099511628211ULL;
    }
    free(b.p);
    return h;
}
// Constant-time compare: time
// depends only on the lengths (an early return there is fine, lengths
// aren't secret), never on where the bytes differ. `diff` is volatile so
// the OR loop can't be short-circuited or vectorized away at any -O level.
static int subtle_constant_time_eq(val a, val b) {
    int64_t n = a->u.s.len;
    if (n != b->u.s.len) return 0;
    volatile uint8_t diff = 0;
    for (int64_t i = 0; i < n; i++) {
        diff |= (uint8_t)a->u.s.data[i] ^ (uint8_t)b->u.s.data[i];
    }
    return diff == 0;
}
// Stable small ids for std primitives: llgen resolves a call's std key to
// one of these at compile time and
// emits rt_std_id directly, skipping the string compares below. Order
// matches internal/llgen/std_ops.go's stdOpNames (TestStdOpsMatchC checks it).
enum std_op {
    OP_LEN,
    OP_STRING_TO_BYTES,
    OP_STRING_FROM_BYTES,
    OP_STRING_TRY_FROM_BYTES,
    OP_EQUALS,
    OP_COMPARE_TO,
    OP_HASH,
    OP_DISPLAY,
    OP_STRING_BYTES,
    OP_BYTES_SLICE,
    OP_BYTES_ZEROS,
    OP_BYTES_FILL,
    OP_BYTES_FROM_ARRAY,
    OP_BYTES_TRY_FROM_ARRAY,
    OP_BYTES_CONCAT,
    OP_BYTES_TRY_CONCAT,
    OP_ARRAY_EMPTY,
    OP_ARRAY_OF,
    OP_ARRAY_PUSH,
    OP_ARRAY_GROW_BY,
    OP_ARRAY_LEN,
    OP_ARRAY_GET,
    OP_ARRAY_WITH,
    OP_ARRAY_WITH_MUT,
    OP_ARRAY_TAKE_AT,
    OP_OPTION_TAKE,
    OP_TEXT_PARSE_FLOAT,
    OP_HOST_SHELL,
    OP_HOST_ARGS,
    OP_HOST_FILES,
    OP_HOST_NET,
    OP_HOST_DL,
    OP_HOST_ENTROPY,
    OP_HOST_SIGNALS,
    OP_ARGS_GET,
    OP_CSTRING_NEW,
    OP_CSTRING_PTR,
    OP_CSTRING_DROP,
    OP_FFI_BYTES_FROM,
    OP_FFI_BYTES_PTR,
    OP_BYTES_GET,
    OP_SUBTLE_CT_EQ,
    OP_DL_SYMBOL_CALL,
    OP_HOST_STDIN,
    OP_HOST_STDOUT,
    OP_PIN_NEW,
    OP_PIN_PTR,
    OP_PIN_WITH,
    OP_PIN_DROP,
    OP_PIN_RELEASE,
    OP_PIN_RECLAIM,
    OP_FFI_STRING,
    OP_FFI_STRING_CHECKED,
    OP_TASK_LOCAL,
    OP_EXECUTOR_RUN,
    OP_EXECUTOR_SPAWN,
    OP_TASK_JOIN,
    OP_ARENA_CREATE,
    OP_ALLOCATOR_SCOPE,
    OP_ALLOCATOR_ALLOCATED,
    OP_SHARED_NEW,
    OP_SHARED_CLONE,
    OP_SHARED_GET,
    OP_SHARED_SET,
    OP_SHARED_WITH,
    OP_SHARED_WITH_MUT,
    OP_SHARED_DOWNGRADE,
    OP_WEAK_UPGRADE,
    OP_NET_HTTP,
    OP_NET_CONN_DROP,
    OP_BUILD_ONLY,
    OP_ARGS_LEN,
    OP_MATH_SQRT,
    OP_MATH_FLOOR,
    OP_MATH_CEIL,
    OP_MATH_ROUND,
    OP_MATH_POW,
    OP_STRING_SLICE_BYTES,
    OP_ARRAY_CLONE,
    OP_WRAPPING_ADD,
    OP_WRAPPING_SUB,
    OP_WRAPPING_MUL,
    OP_WRAPPING_SHL,
    OP_WRAPPING_SHR,
    OP_ROTATE_LEFT,
    OP_ROTATE_RIGHT,
    OP_CHAR_FROM_INT,
    OP_FFI_STRING_LOSSY,
};
// rt_std_op_of mirrors std_ops.go's stdOpFor: an exact key first, then its
// trailing method name, then the std/build package prefix. Returns -1 for
// a key none of those resolve.
static int rt_std_op_of(const char* key) {
    const char* k = key;
    if (!strncmp(k, "std/", 4)) k += 4;
    if (!strcmp(k, "prelude.String.len") || !strcmp(k, "prelude.Bytes.len")) return OP_LEN;
    if (!strcmp(k, "prelude.String.toBytes")) return OP_STRING_TO_BYTES;
    if (!strcmp(k, "prelude.String.fromBytes")) return OP_STRING_FROM_BYTES;
    if (!strcmp(k, "prelude.String.tryFromBytes")) return OP_STRING_TRY_FROM_BYTES;
    if (!strcmp(k, "prelude.debug") || !strcmp(k, "text.fromChar")) return OP_DISPLAY;
    if (!strcmp(k, "prelude.String.bytes")) return OP_STRING_BYTES;
    if (!strcmp(k, "prelude.Bytes.slice")) return OP_BYTES_SLICE;
    if (!strcmp(k, "prelude.Bytes.zeros")) return OP_BYTES_ZEROS;
    if (!strcmp(k, "prelude.Bytes.fill")) return OP_BYTES_FILL;
    if (!strcmp(k, "prelude.Bytes.fromArray")) return OP_BYTES_FROM_ARRAY;
    if (!strcmp(k, "prelude.Bytes.tryFromArray")) return OP_BYTES_TRY_FROM_ARRAY;
    if (!strcmp(k, "prelude.Bytes.concat")) return OP_BYTES_CONCAT;
    if (!strcmp(k, "prelude.Bytes.tryConcat")) return OP_BYTES_TRY_CONCAT;
    if (!strcmp(k, "array.Array.empty")) return OP_ARRAY_EMPTY;
    if (!strcmp(k, "array.Array.of")) return OP_ARRAY_OF;
    if (!strcmp(k, "array.Array.push")) return OP_ARRAY_PUSH;
    if (!strcmp(k, "array.Array.growBy")) return OP_ARRAY_GROW_BY;
    if (!strcmp(k, "array.Array.len")) return OP_ARRAY_LEN;
    if (!strcmp(k, "array.Array.get")) return OP_ARRAY_GET;
    if (!strcmp(k, "array.Array.with")) return OP_ARRAY_WITH;
    if (!strcmp(k, "array.Array.withMut")) return OP_ARRAY_WITH_MUT;
    if (!strcmp(k, "array.Array.takeAt")) return OP_ARRAY_TAKE_AT;
    if (!strcmp(k, "prelude.Option.take")) return OP_OPTION_TAKE;
    if (!strcmp(k, "text.parseFloat")) return OP_TEXT_PARSE_FLOAT;
    if (!strcmp(k, "os.Host.shell")) return OP_HOST_SHELL;
    if (!strcmp(k, "os.Host.args")) return OP_HOST_ARGS;
    if (!strcmp(k, "os.Host.files")) return OP_HOST_FILES;
    if (!strcmp(k, "os.Host.net")) return OP_HOST_NET;
    if (!strcmp(k, "os.Host.dl")) return OP_HOST_DL;
    if (!strcmp(k, "os.Host.entropy")) return OP_HOST_ENTROPY;
    if (!strcmp(k, "os.Host.signals")) return OP_HOST_SIGNALS;
    if (!strcmp(k, "os.Args.get")) return OP_ARGS_GET;
    if (!strcmp(k, "ffi.CString.new")) return OP_CSTRING_NEW;
    if (!strcmp(k, "ffi.CString.ptr")) return OP_CSTRING_PTR;
    if (!strcmp(k, "ffi.CString.drop")) return OP_CSTRING_DROP;
    if (!strcmp(k, "ffi.bytesFrom")) return OP_FFI_BYTES_FROM;
    if (!strcmp(k, "ffi.bytesPtr")) return OP_FFI_BYTES_PTR;
    if (!strcmp(k, "prelude.Bytes.get")) return OP_BYTES_GET;
    if (!strcmp(k, "crypto/subtle.constantTimeEq")) return OP_SUBTLE_CT_EQ;
    if (!strcmp(k, "dl.Symbol.call")) return OP_DL_SYMBOL_CALL;
    if (!strcmp(k, "os.Host.stdin")) return OP_HOST_STDIN;
    if (!strcmp(k, "os.Host.stdout")) return OP_HOST_STDOUT;
    if (!strcmp(k, "ffi.Pin.new")) return OP_PIN_NEW;
    if (!strcmp(k, "ffi.Pin.ptr")) return OP_PIN_PTR;
    if (!strcmp(k, "ffi.Pin.with")) return OP_PIN_WITH;
    if (!strcmp(k, "ffi.Pin.drop")) return OP_PIN_DROP;
    if (!strcmp(k, "ffi.Pin.release")) return OP_PIN_RELEASE;
    if (!strcmp(k, "ffi.Pin.reclaim")) return OP_PIN_RECLAIM;
    if (!strcmp(k, "ffi.string")) return OP_FFI_STRING;
    if (!strcmp(k, "ffi.stringChecked")) return OP_FFI_STRING_CHECKED;
    if (!strcmp(k, "ffi.stringLossy")) return OP_FFI_STRING_LOSSY;
    if (!strcmp(k, "task.local")) return OP_TASK_LOCAL;
    if (!strcmp(k, "task.Executor.run")) return OP_EXECUTOR_RUN;
    if (!strcmp(k, "task.Executor.spawn")) return OP_EXECUTOR_SPAWN;
    if (!strcmp(k, "task.Task.join")) return OP_TASK_JOIN;
    if (!strcmp(k, "alloc.Arena.create")) return OP_ARENA_CREATE;
    if (!strcmp(k, "alloc.AllocatorHandle.scope")) return OP_ALLOCATOR_SCOPE;
    if (!strcmp(k, "alloc.AllocatorHandle.allocated")) return OP_ALLOCATOR_ALLOCATED;
    if (!strcmp(k, "alloc.Shared.new")) return OP_SHARED_NEW;
    if (!strcmp(k, "alloc.Shared.clone") || !strcmp(k, "alloc.Weak.clone")) return OP_SHARED_CLONE;
    if (!strcmp(k, "alloc.Shared.get")) return OP_SHARED_GET;
    if (!strcmp(k, "alloc.Shared.set")) return OP_SHARED_SET;
    if (!strcmp(k, "alloc.Shared.with")) return OP_SHARED_WITH;
    if (!strcmp(k, "alloc.Shared.withMut")) return OP_SHARED_WITH_MUT;
    if (!strcmp(k, "alloc.Shared.downgrade")) return OP_SHARED_DOWNGRADE;
    if (!strcmp(k, "alloc.Weak.upgrade")) return OP_WEAK_UPGRADE;
    if (!strcmp(k, "net.Net.http")) return OP_NET_HTTP;
    if (!strcmp(k, "net.Conn.drop") || !strcmp(k, "net.Listener.drop")) return OP_NET_CONN_DROP;
    if (!strcmp(k, "os.Args.len")) return OP_ARGS_LEN;
    if (!strcmp(k, "math.sqrt")) return OP_MATH_SQRT;
    if (!strcmp(k, "math.floor")) return OP_MATH_FLOOR;
    if (!strcmp(k, "math.ceil")) return OP_MATH_CEIL;
    if (!strcmp(k, "math.round")) return OP_MATH_ROUND;
    if (!strcmp(k, "math.pow")) return OP_MATH_POW;
    if (!strcmp(k, "prelude.String.sliceBytes")) return OP_STRING_SLICE_BYTES;
    if (!strcmp(k, "array.Array.clone")) return OP_ARRAY_CLONE;
    if (!strcmp(k, "prelude.Char.fromInt")) return OP_CHAR_FROM_INT;
    const char* dot = strrchr(k, '.');
    const char* m = dot ? dot + 1 : k;
    if (!strcmp(m, "equals")) return OP_EQUALS;
    if (!strcmp(m, "compareTo")) return OP_COMPARE_TO;
    if (!strcmp(m, "hash")) return OP_HASH;
    if (!strcmp(m, "wrappingAdd")) return OP_WRAPPING_ADD;
    if (!strcmp(m, "wrappingSub")) return OP_WRAPPING_SUB;
    if (!strcmp(m, "wrappingMul")) return OP_WRAPPING_MUL;
    if (!strcmp(m, "wrappingShiftLeft")) return OP_WRAPPING_SHL;
    if (!strcmp(m, "wrappingShiftRight")) return OP_WRAPPING_SHR;
    if (!strcmp(m, "rotateLeft")) return OP_ROTATE_LEFT;
    if (!strcmp(m, "rotateRight")) return OP_ROTATE_RIGHT;
    if (strstr(k, "build.")) return OP_BUILD_ONLY;
    return -1;
}
// rt_std_id never compares key strings: llgen resolves a call's std key to
// an enum std_op at compile time (rt_std_op_of does the same resolution
// for rt_std's few dynamic callers).
val rt_std_id(int op, int n, val* a) {
    val a0 = n > 0 ? deref(a[0]) : NULL, a1 = n > 1 ? deref(a[1]) : NULL, a2 = n > 2 ? deref(a[2]) : NULL;
    if (a0 && a0->kind == K_BOX) a0 = a0->u.box.v;
    switch ((enum std_op)op) {
    // prelude
    case OP_LEN: return rt_int(a0->u.s.len, 64);
    case OP_STRING_TO_BYTES: return bytes_val(a0->u.s.data, a0->u.s.len);
    case OP_STRING_FROM_BYTES:
        if (!utf8_valid(a0->u.s.data, a0->u.s.len)) return err_msg("invalid UTF-8");
        return ok(strdup_val(a0->u.s.data, a0->u.s.len));
    case OP_STRING_TRY_FROM_BYTES: {
        if (!utf8_valid(a0->u.s.data, a0->u.s.len)) return err_msg("invalid UTF-8");
        char* d = strdup_or_null(a0->u.s.data, a0->u.s.len);
        if (!d) return err_msg("out of memory");
        return ok(rt_str_take(d, a0->u.s.len));
    }
    case OP_EQUALS: return rt_bool(a0->kind == K_FLOAT ? float_total_equal(a0->u.f, a1->u.f) : equal(a0, a1));
    case OP_COMPARE_TO: return ordering(a0->kind == K_FLOAT ? float_total_compare(a0->u.f, a1->u.f) : cmp_vals(a0, a1));
    case OP_HASH: return rt_int((int64_t)hash_of(a0), 64);
    case OP_DISPLAY: return display(a0);
    case OP_STRING_BYTES: return bytes_val(a0->u.s.data, a0->u.s.len);
    case OP_BYTES_SLICE: {
        int64_t s = a1->u.i, e = a2->u.i, len = a0->u.s.len;
        if (s < 0 || s > e || e > len) return none();
        return some(bytes_val(a0->u.s.data + s, e - s));
    }
    case OP_BYTES_ZEROS: {
        int64_t n2 = a0->u.i;
        if (n2 < 0) rt_panic("out of memory");
        char* d = calloc((uint64_t)n2 + 1, 1);
        if (!d) rt_panic("out of memory");
        val v = rt_str_take(d, n2);
        v->kind = K_BYTES;
        return v;
    }
    case OP_BYTES_FILL: {
        int64_t n2 = a1->u.i;
        if (n2 < 0) rt_panic("out of memory");
        char* d = malloc((uint64_t)n2 + 1);
        if (!d) rt_panic("out of memory");
        memset(d, (unsigned char)a0->u.i, n2);
        d[n2] = 0;
        val v = rt_str_take(d, n2);
        v->kind = K_BYTES;
        return v;
    }
    case OP_BYTES_FROM_ARRAY: {
        int64_t n2 = a0->u.arr.len;
        char* d = malloc(n2 + 1);
        if (!d) rt_panic("out of memory");
        for (int64_t i = 0; i < n2; i++) d[i] = (char)deref(a0->u.arr.items[i])->u.i;
        d[n2] = 0;
        val v = rt_str_take(d, n2);
        v->kind = K_BYTES;
        return v;
    }
    case OP_BYTES_TRY_FROM_ARRAY: {
        int64_t n2 = a0->u.arr.len;
        char* d = malloc(n2 + 1);
        if (!d) return alloc_error();
        for (int64_t i = 0; i < n2; i++) d[i] = (char)deref(a0->u.arr.items[i])->u.i;
        d[n2] = 0;
        val v = rt_str_take(d, n2);
        v->kind = K_BYTES;
        return ok(v);
    }
    case OP_BYTES_CONCAT: {
        int64_t l0 = a0->u.s.len, l1 = a1->u.s.len;
        char* d = malloc(l0 + l1 + 1);
        if (!d) rt_panic("out of memory");
        memcpy(d, a0->u.s.data, l0);
        memcpy(d + l0, a1->u.s.data, l1);
        d[l0 + l1] = 0;
        val v = rt_str_take(d, l0 + l1);
        v->kind = K_BYTES;
        return v;
    }
    case OP_BYTES_TRY_CONCAT: {
        int64_t l0 = a0->u.s.len, l1 = a1->u.s.len;
        char* d = malloc(l0 + l1 + 1);
        if (!d) return alloc_error();
        memcpy(d, a0->u.s.data, l0);
        memcpy(d + l0, a1->u.s.data, l1);
        d[l0 + l1] = 0;
        val v = rt_str_take(d, l0 + l1);
        v->kind = K_BYTES;
        return ok(v);
    }
    // array
    case OP_ARRAY_EMPTY: return rt_array(0, NULL);
    case OP_ARRAY_OF: return rt_retain(a0);
    case OP_ARRAY_PUSH:
        push(a0, rt_retain(a[1]));
        return rt_unit();
    case OP_ARRAY_GROW_BY: return rt_bool(try_grow(a0, (uint64_t)a1->u.i));
    case OP_ARRAY_LEN: return rt_int(a0->u.arr.len, 64);
    case OP_ARRAY_GET: {
        int64_t i = a1->u.i;
        if (i < 0 || i >= a0->u.arr.len) return none();
        return some(rt_copy(a0->u.arr.items[i]));
    }
    case OP_ARRAY_WITH: {
        int64_t i = a1->u.i;
        if (i < 0 || i >= a0->u.arr.len) return none();
        val arg[1] = {a0->u.arr.items[i]};
        return some(rt_callv(a2, 1, arg));
    }
    case OP_ARRAY_WITH_MUT: {
        int64_t i = a1->u.i;
        if (i < 0 || i >= a0->u.arr.len) return none();
        // A scalar element aliases its obj on ordinary copy, so the
        // callback writes through a cell rather than the element itself;
        // the slot takes the cell's (possibly new) content back after.
        val cell = rt_cell(a0->u.arr.items[i]);
        val arg[1] = {cell};
        val r = rt_callv(a2, 1, arg);
        a0->u.arr.items[i] = cell->u.cell.v;
        // free_obj on a cell releases its `.v`, already handed to the
        // array slot above; detach it first so that release is a no-op.
        cell->u.cell.v = NULL;
        rt_release(cell);
        return some(r);
    }
    case OP_ARRAY_TAKE_AT: {
        int64_t i = a1->u.i;
        if (i < 0 || i >= a0->u.arr.len) return none();
        val x = a0->u.arr.items[i];
        for (int64_t j = i; j + 1 < a0->u.arr.len; j++) a0->u.arr.items[j] = a0->u.arr.items[j + 1];
        a0->u.arr.len--;
        return some(x);
    }
    case OP_OPTION_TAKE: {
        if (a0->u.var.v != V_Some) return none();
        val x = a0->u.var.payload[0];
        a0->u.var.v = V_None;
        a0->u.var.n = 0;
        return some(x);
    }
    // text
    case OP_TEXT_PARSE_FLOAT: {
        const char* s = cstr(a0);
        if (!*s || *s == ' ' || *s == '\t' || *s == '\n' || *s == '+') return none();
        const char* end;
        double d;
        // rt_parse_f64's erange also fires on underflow to 0/subnormal;
        // Go's strconv.ParseFloat only errors on overflow, so reject only
        // when the rounded result is actually infinite.
        if (!rt_parse_f64(s, &end, &d, NULL) || *end || isinf(d)) return none();
        return some(rt_float(d, 64 | 512));
    }
    // os / fs
    case OP_HOST_SHELL: return opaque("Shell", NULL);
    case OP_HOST_ARGS: return opaque("Args", NULL);
    case OP_HOST_FILES: return opaque("Files", NULL);
    case OP_HOST_NET: return opaque("Net", NULL);
    case OP_HOST_DL: return opaque("Loader", NULL);
    case OP_HOST_ENTROPY: return opaque("Entropy", NULL);
    case OP_HOST_SIGNALS: return opaque("Signals", NULL);
    case OP_ARGS_GET: {
        int64_t i = a1->u.i;
        if (i < 0 || i >= g_argc) return none();
        int64_t len = (int64_t)strlen(g_argv[i]);
        if (!utf8_valid(g_argv[i], len)) return none();
        return some(rt_str(g_argv[i], len));
    }
    // ffi / crypto / dl
    case OP_CSTRING_NEW: {
        // Every String/Bytes buffer already carries a guaranteed trailing
        // NUL at data[len] (see strdup_val); strchr for the first NUL byte
        // reaches that one unless an earlier, embedded NUL is hit first.
        if (strchr(a0->u.s.data, 0) != a0->u.s.data + a0->u.s.len) return err_msg("text contains a NUL byte");
        val h = rt_int((int64_t)(intptr_t)rt_cstr(a0), 64 | 256);
        return ok(make_record("CString", 1, &h));
    }
    case OP_CSTRING_PTR: return rt_ptr((void*)(intptr_t)a0->u.rec.fields[0]->u.i);
    case OP_CSTRING_DROP:
        free((void*)(intptr_t)a0->u.rec.fields[0]->u.i);
        return rt_unit();
    case OP_FFI_BYTES_FROM: return bytes_val((const char*)a0->u.p, a1->u.i);
    case OP_FFI_BYTES_PTR: return rt_ptr((void*)a0->u.s.data);
    case OP_BYTES_GET: {
        int64_t i = a1->u.i;
        if (i < 0 || i >= a0->u.s.len) return none();
        return some(rt_int((uint8_t)a0->u.s.data[i], 8));
    }
    case OP_SUBTLE_CT_EQ: return rt_bool(subtle_constant_time_eq(a0, a1));
    case OP_DL_SYMBOL_CALL:
        rt_panic("dl.Symbol.call is lowered by the compiler");
        return NULL;
    case OP_HOST_STDIN: return opaque("Stdin", NULL);
    case OP_HOST_STDOUT: return opaque("Stdout", NULL);
    case OP_PIN_NEW: {
        val v = rt_retain(deref(a[0]));
        val h = rt_int((int64_t)(intptr_t)v, 64 | 256);
        return make_record("Pin", 1, &h);
    }
    case OP_PIN_PTR: return rt_ptr((void*)(intptr_t)a0->u.rec.fields[0]->u.i);
    case OP_PIN_WITH: {
        val arg[1] = {(val)a0->u.p};
        return rt_callv(a1, 1, arg);
    }
    case OP_PIN_DROP: {
        // mir now consumes self for a type's own drop (isOwnDrop in
        // internal/mir/plan_args.go), so scope exit no longer re-enters
        // here; the zero-guard just stays as a defensive no-op on repeat.
        int64_t* hf = &a0->u.rec.fields[0]->u.i;
        if (*hf) {
            rt_release((val)(intptr_t)*hf);
            *hf = 0;
        }
        return rt_unit();
    }
    case OP_PIN_RELEASE:
        // release() (unlike drop) still lends self, so the caller's
        // scope-exit drop runs after this; retain here keeps the
        // extracted pointer alive through that drop.
        rt_retain((val)(intptr_t)a0->u.rec.fields[0]->u.i);
        return rt_ptr((void*)(intptr_t)a0->u.rec.fields[0]->u.i);
    case OP_PIN_RECLAIM: {
        val h = rt_int((int64_t)(intptr_t)a0->u.p, 64 | 256);
        return make_record("Pin", 1, &h);
    }
    case OP_FFI_STRING: {
        const char* s = a0->u.p ? (const char*)a0->u.p : "";
        return strdup_val(s, strlen(s));
    }
    case OP_FFI_STRING_LOSSY: {
        const char* s = a0->u.p ? (const char*)a0->u.p : "";
        return utf8_lossy_val(s, (int64_t)strlen(s));
    }
    case OP_FFI_STRING_CHECKED: {
        const char* s = a0->u.p ? (const char*)a0->u.p : "";
        int64_t len = (int64_t)strlen(s);
        if (!utf8_valid(s, len)) return none();
        return some(strdup_val(s, len));
    }
    // task / alloc / net
    case OP_TASK_LOCAL: return opaque("Executor", NULL);
    case OP_EXECUTOR_RUN: return await_future(a1);
    case OP_EXECUTOR_SPAWN: return opaque("Task", rt_retain(a1));
    case OP_TASK_JOIN: return await_future((val)a0->u.op.data);
    case OP_ARENA_CREATE: {
        arena* ar = calloc(1, sizeof *ar);
        ar->a.alloc = arena_alloc;
        ar->a.free = arena_free;
        return ok(opaque("AllocatorHandle", ar));
    }
    case OP_ALLOCATOR_SCOPE: return opaque_in(&((arena*)a0->u.op.data)->a, "AllocatorScope", rt_retain(a0));
    case OP_ALLOCATOR_ALLOCATED: return rt_int(((arena*)a0->u.op.data)->a.bytes, 64 | 256);
    case OP_SHARED_NEW: {
        sctrl* c = calloc(1, sizeof *c);
        c->cell = rt_cell(rt_retain(a0));
        c->alive = 1;
        c->shared = opaque("Shared", c);
        return c->shared;
    }
    case OP_SHARED_CLONE: return rt_retain(a0);
    case OP_SHARED_GET: return rt_copy(((sctrl*)a0->u.op.data)->cell->u.cell.v);
    case OP_SHARED_SET: {
        val cell = ((sctrl*)a0->u.op.data)->cell;
        val old = cell->u.cell.v;
        cell->u.cell.v = rt_retain(a1);
        rt_release(old);
        return rt_unit();
    }
    case OP_SHARED_WITH: {
        sctrl* c = a0->u.op.data;
        if (c->borrow < 0) rt_panic("Shared is exclusively borrowed");
        c->borrow++;
        val arg[1] = {c->cell->u.cell.v};
        val r = rt_callv(a1, 1, arg);
        c->borrow--;
        return r;
    }
    case OP_SHARED_WITH_MUT: {
        sctrl* c = a0->u.op.data;
        if (c->borrow != 0) rt_panic("Shared is already borrowed");
        c->borrow = -1;
        // The callback writes through c->cell itself, same as any
        // other &mut Cell store, instead of a snapshot of its content.
        val arg[1] = {c->cell};
        val r = rt_callv(a1, 1, arg);
        c->borrow = 0;
        return r;
    }
    case OP_SHARED_DOWNGRADE: {
        sctrl* c = a0->u.op.data;
        c->weak++;
        return opaque("Weak", c);
    }
    case OP_WEAK_UPGRADE: {
        sctrl* c = a0->u.op.data;
        return c->alive ? some(rt_retain(c->shared)) : none();
    }
    case OP_NET_HTTP: return opaque("Http", NULL);
    // Bodiless like Conn.drop/Listener.drop themselves (see net_hosted.kg):
    // an accelerator on a *bodied* drop only replaces normal calls to it,
    // not the implicit drop on a resource's last reference going, which
    // always runs the real (here nonexistent) lowered body instead.
    case OP_NET_CONN_DROP:
        rt_sys_close((int)a0->u.rec.fields[0]->u.i);
        return rt_unit();
    case OP_BUILD_ONLY:
        rt_panic("std/build only runs inside `kigumi build`");
        return NULL;
    // math / misc
    case OP_ARGS_LEN: return rt_int(g_argc, 64);
    case OP_MATH_SQRT: return rt_float(sqrt(a0->u.f), 64 | 512);
    case OP_MATH_FLOOR: return rt_float(floor(a0->u.f), 64 | 512);
    case OP_MATH_CEIL: return rt_float(ceil(a0->u.f), 64 | 512);
    case OP_MATH_ROUND: return rt_float(round(a0->u.f), 64 | 512);
    case OP_MATH_POW: return rt_float(pow(a0->u.f, a1->u.f), 64 | 512);
    case OP_STRING_SLICE_BYTES: {
        int64_t s = a1->u.i, e = a2->u.i, len = a0->u.s.len;
        if (s < 0 || s > e || e > len || !boundary(a0->u.s.data, len, s) || !boundary(a0->u.s.data, len, e)) return none();
        return some(strdup_val(a0->u.s.data + s, e - s));
    }
    case OP_ARRAY_CLONE: return rt_copy(a0);
    // int_fn reads the op name after the key's last '.', so a leading dot
    // is enough; reusing it keeps the wrapping/rotate math in one place.
    case OP_WRAPPING_ADD: return int_fn(".wrappingAdd", a0, a1);
    case OP_WRAPPING_SUB: return int_fn(".wrappingSub", a0, a1);
    case OP_WRAPPING_MUL: return int_fn(".wrappingMul", a0, a1);
    case OP_WRAPPING_SHL: return int_fn(".wrappingShiftLeft", a0, a1);
    case OP_WRAPPING_SHR: return int_fn(".wrappingShiftRight", a0, a1);
    case OP_ROTATE_LEFT: return int_fn(".rotateLeft", a0, a1);
    case OP_ROTATE_RIGHT: return int_fn(".rotateRight", a0, a1);
    case OP_CHAR_FROM_INT: {
        int64_t v = a0->u.i;
        if (v < 0 || v > 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF)) return none();
        return some(rt_char((uint32_t)v));
    }
    }
    rt_panic("rt_std_id: bad op");
    return NULL;
}
// rt_std is rt_callv's path for a std function value (rt_std_closure /
// rt_std_bind carry the key, resolved fresh on every call); a static call
// site instead calls rt_std_id directly at its own compile-time-known op.
val rt_std(const char* key, int n, val* a) {
    int op = rt_std_op_of(key);
    if (op >= 0) return rt_std_id(op, n, a);
    val a0 = n > 0 ? deref(a[0]) : NULL, a1 = n > 1 ? deref(a[1]) : NULL, a2 = n > 2 ? deref(a[2]) : NULL;
    if (a0 && a0->kind == K_BOX) a0 = a0->u.box.v;
    val r = rt_sys_std(key, n, a, a0, a1, a2);
    if (r) return r;
    panicf("no runtime for %s", key);
    return NULL;
}
