// Shared by rt_core.c and the rt_sys_*.c layer: the boxed value model and
// the core helpers a system layer builds on.
#ifndef KIGUMI_RT_H
#define KIGUMI_RT_H
#include <stddef.h>
#include <stdint.h>
typedef struct rt_type {
    const char* name;
    int kind;
    int nfields;
    const char** fields;
    int nvariants;
    struct rt_vdesc** variants;
    void* drop;
    // borrow_fields bits the fields holding a borrow (`&'a T`);
    // rt_release/rt_copy skip them, since the record never owns them.
    uint64_t borrow_fields;
} rt_type;
typedef struct rt_vdesc {
    const char* name;
    rt_type* adt;
    int index;
    int npayload;
} rt_vdesc;
enum { K_INT = 1,
       K_FLOAT,
       K_BOOL,
       K_STR,
       K_BYTES,
       K_CHAR,
       K_UNIT,
       K_RECORD,
       K_VARIANT,
       K_ARRAY,
       K_CLOSURE,
       K_BOX,
       K_CELL,
       K_OPAQUE,
       K_PTR };

typedef struct obj* val;
typedef val (*adapter_fn)(val env, val args);
struct obj {
    int kind;
    int nk;
    int rc;
    int owned;
    struct rt_allocator* al;
    union rt_value {
        int64_t i;
        double f;
        int b;
        uint32_t c;
        struct {
            const char* data;
            int64_t len;
        } s;
        struct {
            rt_type* type;
            int64_t n;
            val* fields;
        } rec;
        struct {
            rt_vdesc* v;
            int64_t n;
            val* payload;
        } var;
        struct {
            val* items;
            int64_t len, cap;
        } arr;
        struct {
            adapter_fn fn;
            val env;
            const char* key;
            int lent;
            int async;
        } clo;
        struct {
            rt_type* dyn;
            val v;
            // vt holds one adapter per requirement of the interface the
            // value was boxed as.
            void** vt;
        } box;
        struct {
            val v;
        } cell;
        struct {
            const char* kind;
            void* data;
        } op;
        void* p;
    } u;
};

typedef struct {
    char* p;
    size_t len, cap;
} buf;

// ---- system layer (rt_sys.c; hosted std overrides the weak symbols) ----
void rt_sys_write(int fd, const char* p, size_t n);
void rt_sys_exit(int code);
int rt_sys_close(int fd);
val rt_sys_std(const char* k, int n, val* a, val a0, val a1, val a2);

// ---- core helpers ----
void rt_panic(const char* msg);
void panicf(const char* fmt, const char* a);
val rt_retain(val v);
void rt_release(val v);
val deref(val v);
val rt_unit(void);
val rt_int(int64_t i, int nk);
val rt_float(double f, int nk);
val rt_bool(int b);
val rt_str(const char* data, int64_t len);
val rt_bytes(const char* data, int64_t len);
val rt_str_take(char* data, int64_t len);
val strdup_val(const char* s, int64_t len);
val bytes_val(const char* s, int64_t len);
val rt_from_cstr(const char* s);
val rt_ptr(void* p);
val rt_array(int64_t n, val* items);
void push(val a, val x);
val rt_record(rt_type* t, int64_t n, val* fields);
val rt_copy(val v);
val opaque(const char* kind, void* data);
val some(val v);
val none(void);
val ok(val v);
val err_val(val e);
val err_msg(const char* msg);
val make_record(const char* tname, int n, val* fields);
val display(val v);
void display_into(buf* b, val v, int canon);
const char* rt_cstr(val s);
const char* cstr(val s);
void bput(buf* b, const char* s, size_t n);
void bputs(buf* b, const char* s);
int utf8_valid(const char* s, int64_t n);
rt_type* find_type(const char* name);
int equal(val a, val b);

// ---- rt_float.c: shortest round-trip formatting, correctly rounded parsing ----
void rt_format_float(double x, int f32, char* out, size_t n);
int rt_parse_f64(const char* s, const char** end, double* out, int* erange);
int rt_parse_f32(const char* s, const char** end, float* out, int* erange);
#endif
