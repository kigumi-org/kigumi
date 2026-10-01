package sem

// HeaderImportSkipped is internal/cheader's diagnostic for a C header with
// unsupported constructs, one per imported header; internal/driver builds it
// directly and reads Num itself, since parsing the generated package hasn't
// produced a sem.Result to call r.errAt on yet.
var HeaderImportSkipped = defineCode("E994", "header-import-skipped", "importing `{path}` skipped {n} declaration{s}: {list}; add the missing pieces by hand if the program needs them")
