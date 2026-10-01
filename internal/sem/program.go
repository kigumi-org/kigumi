package sem

import (
	"strings"

	"kigumi/internal/syntax"
)

// must run before every other pass.
func (r *Result) prepareProgram() {
	r.pkgEntity = make([]EntityID, len(r.Packages))
	r.fileScopes = make([]ScopeID, len(r.Files))
	for id := 1; id < len(r.Packages); id++ {
		p := &r.Packages[id]
		p.Parent = r.findParent(PackageID(id))
		r.pkgEntity[id] = r.newEntity(Entity{Kind: EntPackage, Name: p.Path, Pkg: PackageID(id), Vis: Visibility{Level: VisPub}})
		for _, f := range p.Files {
			scope := r.newScope(ScopeFile, p.Scope, f, r.Files[f].Tree.Root)
			r.fileScopes[f] = scope
			r.Files[f].Scopes[r.tree(f).Root] = scope
			r.Files[f].Scopes[r.Files[f].Tree.Root] = scope
		}
	}
}

func (r *Result) findParent(id PackageID) PackageID {
	p := r.Packages[id]
	path := p.Path
	for {
		i := strings.LastIndexByte(path, '/')
		if i < 0 {
			return 0
		}
		path = path[:i]
		if anc, ok := r.pathIndex[path]; ok && r.Packages[anc].Module == p.Module {
			return anc
		}
	}
}

func (r *Result) isAncestor(anc, pkg PackageID) bool {
	a, p := r.Packages[anc], r.Packages[pkg]
	return a.Module == p.Module && anc != pkg && strings.HasPrefix(p.Path, a.Path+"/")
}

func (r *Result) sameModule(a, b PackageID) bool {
	return r.Packages[a].Module == r.Packages[b].Module
}

func (r *Result) packageOf(f FileID) PackageID { return r.Files[f].Pkg }

func (r *Result) tree(f FileID) *syntax.Tree { return r.Files[f].Tree }

func (r *Result) topDecls(f FileID) []syntax.NodeID {
	t := r.tree(f)
	return t.Children(t.Root)
}

func isStatementKind(k syntax.NodeKind) bool {
	switch k {
	case syntax.LetStmt, syntax.ExprStmt, syntax.AssignStmt, syntax.DeferStmt, syntax.ErrdeferStmt:
		return true
	}
	return false
}
