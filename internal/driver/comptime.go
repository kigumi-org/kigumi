package driver

import (
	"kigumi/internal/diag"
	"kigumi/internal/mir"
	"kigumi/internal/vm"
)

// foldComptime evaluates the program's comptime blocks in the VM's sandbox
// and folds the results into the MIR; a failure becomes a diagnostic at
// the block.
func foldComptime(prog *mir.Program) error {
	res := prog.R
	for _, ce := range vm.FoldComptime(prog) {
		f := &res.Files[ce.Site.File]
		tree := res.Tree(ce.Site.File)
		f.Diags = append(f.Diags, diag.Errorf(diag.At(tree.File, tree.Span(ce.Site.Node)), "comptime: "+ce.Err.Error()))
	}
	return nil
}
