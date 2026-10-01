package llgen

// externs is rendered from runtimeFns (runtime_fns.go) so the two never
// drift apart.
var externs = renderExterns()
