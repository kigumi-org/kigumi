package llgen

import "strings"

// stdOpNames names, in id order, every op rt_std_id dispatches on: a
// primitive's id is its index here, matching rt_core.c's `enum std_op`
// order (its C identifier is "OP_" + this name; TestStdOpsMatchC checks it).
var stdOpNames = []string{
	"LEN", "STRING_TO_BYTES", "STRING_FROM_BYTES", "STRING_TRY_FROM_BYTES",
	"EQUALS", "COMPARE_TO", "HASH", "DISPLAY", "STRING_BYTES",
	"BYTES_SLICE", "BYTES_ZEROS", "BYTES_FILL", "BYTES_FROM_ARRAY",
	"BYTES_TRY_FROM_ARRAY", "BYTES_CONCAT", "BYTES_TRY_CONCAT",
	"ARRAY_EMPTY", "ARRAY_OF", "ARRAY_PUSH", "ARRAY_GROW_BY", "ARRAY_LEN",
	"ARRAY_GET", "ARRAY_WITH", "ARRAY_WITH_MUT", "ARRAY_TAKE_AT",
	"OPTION_TAKE", "TEXT_PARSE_FLOAT",
	"HOST_SHELL", "HOST_ARGS", "HOST_FILES", "HOST_NET", "HOST_DL",
	"HOST_ENTROPY", "HOST_SIGNALS", "ARGS_GET",
	"CSTRING_NEW", "CSTRING_PTR", "CSTRING_DROP",
	"FFI_BYTES_FROM", "FFI_BYTES_PTR", "BYTES_GET",
	"SUBTLE_CT_EQ", "DL_SYMBOL_CALL", "HOST_STDIN", "HOST_STDOUT",
	"PIN_NEW", "PIN_PTR", "PIN_WITH", "PIN_DROP", "PIN_RELEASE", "PIN_RECLAIM",
	"FFI_STRING", "FFI_STRING_CHECKED",
	"TASK_LOCAL", "EXECUTOR_RUN", "EXECUTOR_SPAWN", "TASK_JOIN",
	"ARENA_CREATE", "ALLOCATOR_SCOPE", "ALLOCATOR_ALLOCATED",
	"SHARED_NEW", "SHARED_CLONE", "SHARED_GET", "SHARED_SET",
	"SHARED_WITH", "SHARED_WITH_MUT", "SHARED_DOWNGRADE", "WEAK_UPGRADE",
	"NET_HTTP", "NET_CONN_DROP", "BUILD_ONLY",
	"ARGS_LEN", "MATH_SQRT", "MATH_FLOOR", "MATH_CEIL", "MATH_ROUND", "MATH_POW",
	"STRING_SLICE_BYTES", "ARRAY_CLONE",
	"WRAPPING_ADD", "WRAPPING_SUB", "WRAPPING_MUL", "WRAPPING_SHL", "WRAPPING_SHR",
	"ROTATE_LEFT", "ROTATE_RIGHT", "CHAR_FROM_INT",
	"FFI_STRING_LOSSY",
}

// stdOpKeys maps a std key ("std/"-stripped) to the stdOpNames entry
// answering it; keys sharing a name share a runtime case.
var stdOpKeys = map[string]string{
	"prelude.String.len":              "LEN",
	"prelude.Bytes.len":               "LEN",
	"prelude.String.toBytes":          "STRING_TO_BYTES",
	"prelude.String.fromBytes":        "STRING_FROM_BYTES",
	"prelude.String.tryFromBytes":     "STRING_TRY_FROM_BYTES",
	"prelude.debug":                   "DISPLAY",
	"text.fromChar":                   "DISPLAY",
	"prelude.String.bytes":            "STRING_BYTES",
	"prelude.Bytes.slice":             "BYTES_SLICE",
	"prelude.Bytes.zeros":             "BYTES_ZEROS",
	"prelude.Bytes.fill":              "BYTES_FILL",
	"prelude.Bytes.fromArray":         "BYTES_FROM_ARRAY",
	"prelude.Bytes.tryFromArray":      "BYTES_TRY_FROM_ARRAY",
	"prelude.Bytes.concat":            "BYTES_CONCAT",
	"prelude.Bytes.tryConcat":         "BYTES_TRY_CONCAT",
	"array.Array.empty":               "ARRAY_EMPTY",
	"array.Array.of":                  "ARRAY_OF",
	"array.Array.push":                "ARRAY_PUSH",
	"array.Array.growBy":              "ARRAY_GROW_BY",
	"array.Array.len":                 "ARRAY_LEN",
	"array.Array.get":                 "ARRAY_GET",
	"array.Array.with":                "ARRAY_WITH",
	"array.Array.withMut":             "ARRAY_WITH_MUT",
	"array.Array.takeAt":              "ARRAY_TAKE_AT",
	"prelude.Option.take":             "OPTION_TAKE",
	"text.parseFloat":                 "TEXT_PARSE_FLOAT",
	"os.Host.shell":                   "HOST_SHELL",
	"os.Host.args":                    "HOST_ARGS",
	"os.Host.files":                   "HOST_FILES",
	"os.Host.net":                     "HOST_NET",
	"os.Host.dl":                      "HOST_DL",
	"os.Host.entropy":                 "HOST_ENTROPY",
	"os.Host.signals":                 "HOST_SIGNALS",
	"os.Args.get":                     "ARGS_GET",
	"ffi.CString.new":                 "CSTRING_NEW",
	"ffi.CString.ptr":                 "CSTRING_PTR",
	"ffi.CString.drop":                "CSTRING_DROP",
	"ffi.bytesFrom":                   "FFI_BYTES_FROM",
	"ffi.bytesPtr":                    "FFI_BYTES_PTR",
	"prelude.Bytes.get":               "BYTES_GET",
	"crypto/subtle.constantTimeEq":    "SUBTLE_CT_EQ",
	"dl.Symbol.call":                  "DL_SYMBOL_CALL",
	"os.Host.stdin":                   "HOST_STDIN",
	"os.Host.stdout":                  "HOST_STDOUT",
	"ffi.Pin.new":                     "PIN_NEW",
	"ffi.Pin.ptr":                     "PIN_PTR",
	"ffi.Pin.with":                    "PIN_WITH",
	"ffi.Pin.drop":                    "PIN_DROP",
	"ffi.Pin.release":                 "PIN_RELEASE",
	"ffi.Pin.reclaim":                 "PIN_RECLAIM",
	"ffi.string":                      "FFI_STRING",
	"ffi.stringChecked":               "FFI_STRING_CHECKED",
	"ffi.stringLossy":                 "FFI_STRING_LOSSY",
	"task.local":                      "TASK_LOCAL",
	"task.Executor.run":               "EXECUTOR_RUN",
	"task.Executor.spawn":             "EXECUTOR_SPAWN",
	"task.Task.join":                  "TASK_JOIN",
	"alloc.Arena.create":              "ARENA_CREATE",
	"alloc.AllocatorHandle.scope":     "ALLOCATOR_SCOPE",
	"alloc.AllocatorHandle.allocated": "ALLOCATOR_ALLOCATED",
	"alloc.Shared.new":                "SHARED_NEW",
	"alloc.Shared.clone":              "SHARED_CLONE",
	"alloc.Weak.clone":                "SHARED_CLONE",
	"alloc.Shared.get":                "SHARED_GET",
	"alloc.Shared.set":                "SHARED_SET",
	"alloc.Shared.with":               "SHARED_WITH",
	"alloc.Shared.withMut":            "SHARED_WITH_MUT",
	"alloc.Shared.downgrade":          "SHARED_DOWNGRADE",
	"alloc.Weak.upgrade":              "WEAK_UPGRADE",
	"net.Net.http":                    "NET_HTTP",
	"net.Conn.drop":                   "NET_CONN_DROP",
	"net.Listener.drop":               "NET_CONN_DROP",
	"os.Args.len":                     "ARGS_LEN",
	"math.sqrt":                       "MATH_SQRT",
	"math.floor":                      "MATH_FLOOR",
	"math.ceil":                       "MATH_CEIL",
	"math.round":                      "MATH_ROUND",
	"math.pow":                        "MATH_POW",
	"prelude.String.sliceBytes":       "STRING_SLICE_BYTES",
	"array.Array.clone":               "ARRAY_CLONE",
	"prelude.Char.fromInt":            "CHAR_FROM_INT",
}

// stdOpSuffixes maps a key's trailing method name to its op, for
// primitives every eligible type defines the same way (Int8.wrappingAdd,
// u64.wrappingAdd, ... all reach WRAPPING_ADD).
var stdOpSuffixes = map[string]string{
	"equals":             "EQUALS",
	"compareTo":          "COMPARE_TO",
	"hash":               "HASH",
	"wrappingAdd":        "WRAPPING_ADD",
	"wrappingSub":        "WRAPPING_SUB",
	"wrappingMul":        "WRAPPING_MUL",
	"wrappingShiftLeft":  "WRAPPING_SHL",
	"wrappingShiftRight": "WRAPPING_SHR",
	"rotateLeft":         "ROTATE_LEFT",
	"rotateRight":        "ROTATE_RIGHT",
}

var (
	stdOpID       = map[string]int{}
	stdOpByKey    = map[string]int{}
	stdOpBySuffix = map[string]int{}
)

func init() {
	for i, name := range stdOpNames {
		stdOpID[name] = i
	}
	for key, name := range stdOpKeys {
		stdOpByKey[key] = stdOpID[name]
	}
	for method, name := range stdOpSuffixes {
		stdOpBySuffix[method] = stdOpID[name]
	}
}

// stdOpFor resolves a std key to the id rt_std_id's C switch dispatches
// on. ok is false for a primitive not (yet) given an id; the caller falls
// back to rt_std for it.
func stdOpFor(key string) (int, bool) {
	key = strings.TrimPrefix(key, "std/")
	if id, ok := stdOpByKey[key]; ok {
		return id, true
	}
	if i := strings.LastIndexByte(key, '.'); i >= 0 {
		if id, ok := stdOpBySuffix[key[i+1:]]; ok {
			return id, true
		}
	}
	if strings.HasPrefix(key, "build.") {
		return stdOpID["BUILD_ONLY"], true
	}
	return 0, false
}
