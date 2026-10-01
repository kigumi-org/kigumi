package sem

import (
	"cmp"
	"slices"

	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

type PackageInfo struct {
	Path      string
	Module    string
	Parent    PackageID
	Scope     ScopeID
	SCC       int
	Entry     FileID
	HasScript bool
	Std       bool
	Files     []FileID
}

type Result struct {
	Types     *TypeTable
	Entities  []Entity
	Scopes    []Scope
	Files     []FileInfo
	Packages  []PackageInfo
	Bodies    []BodyRef
	Overloads []OverloadSet
	Places    []Place
	Instances map[EntityID][]Instance

	TypeDecls  []TypeDeclInfo
	Fns        []FnInfo
	Ifaces     []InterfaceInfo
	Consts     []ConstInfo
	Locals     []LocalInfo
	Closures   []ClosureInfo
	Fields     []FieldInfo
	Variants   []VariantInfo
	TypeParams []TypeParamInfo
	// Comptime lists the blocks the driver must evaluate before lowering.
	Comptime []ComptimeSite

	target       Target
	noDefault    bool
	noPlatform   bool
	denyPkgs     map[string]bool
	layerCeiling Layer
	hasCeiling   bool
	artifactName string
	arch         string
	universe     ScopeID
	prelude      ScopeID
	pathIndex    map[string]PackageID
	fileIndex    map[*syntax.Tree]FileID
	placeIndex   map[string]PlaceID
	seen         map[diagKey]struct{}
	derives      []deriveRequest
	mod          *Module
	sources      *token.SourceStore
	langItems    map[string]EntityID

	langItemMissing         map[string]bool
	dropMemo                map[TypeID]EffectSummary
	threadOpen              map[threadKey]int
	threadLow               int
	pkgEntity               []EntityID
	fileScopes              []ScopeID
	implicitMain            map[PackageID]EntityID
	importEdges             [][]PackageID
	stdImportGraph          map[PackageID][]PackageID
	pendingImports          []pendingImport
	order                   []PackageID
	bodiesChecked           bool
	declScopes              map[EntityID]ScopeID
	aliasState              map[EntityID]uint8
	aliasParams             map[EntityID][]EntityID
	primEntity              map[TypeID]EntityID
	variantNames            map[PackageID]map[string][]EntityID
	conformMemo             map[conformKey]conformResult
	identPaths              map[*syntax.Tree]map[syntax.NodeID]syntax.NodeID
	pendingWitnessCallbacks []pendingWitnessCallback
}

func check(mod *Module) *Result {
	r := newResult(mod)
	r.prepareProgram()
	r.collect()
	r.resolveImports()
	r.declareTypes()
	r.declareSignatures()
	r.declareConsts()
	r.checkImpls()
	r.closeFieldParamForwarding()
	r.checkBodies()
	r.resolvePendingWitnessCallbacks()
	r.checkEffects()
	r.checkNomono()
	r.reportUnused()
	r.emit()
	return r
}

func newResult(mod *Module) *Result {
	if mod.Sources == nil {
		mod.Sources = &token.SourceStore{}
	}
	for _, p := range mod.Packages {
		for _, t := range p.Files {
			if t.File.ID == 0 {
				mod.Sources.Add(t.File)
			}
		}
	}
	r := &Result{
		sources:      mod.Sources,
		mod:          mod,
		Types:        newTypeTable(cmp.Or(mod.PtrBits, 64)),
		Entities:     []Entity{{}},
		Scopes:       []Scope{{}},
		Overloads:    []OverloadSet{{}},
		Places:       []Place{{}},
		Instances:    map[EntityID][]Instance{},
		target:       mod.Target,
		noDefault:    mod.NoDefaultAllocator,
		noPlatform:   mod.NoPlatform,
		denyPkgs:     denySet(mod.Deny),
		layerCeiling: LayerShortName[mod.LayerCeiling],
		hasCeiling:   mod.LayerCeiling != "",
		artifactName: mod.ArtifactName,
		arch:         cmp.Or(mod.Arch, "amd64"),
		pathIndex:    map[string]PackageID{},
		fileIndex:    map[*syntax.Tree]FileID{},
		placeIndex:   map[string]PlaceID{},
		seen:         map[diagKey]struct{}{},
		langItems:    map[string]EntityID{},

		langItemMissing: map[string]bool{},
		implicitMain:    map[PackageID]EntityID{},
		declScopes:      map[EntityID]ScopeID{},
		aliasState:      map[EntityID]uint8{},
		aliasParams:     map[EntityID][]EntityID{},
		primEntity:      map[TypeID]EntityID{},
		variantNames:    map[PackageID]map[string][]EntityID{},
		conformMemo:     map[conformKey]conformResult{},
		dropMemo:        map[TypeID]EffectSummary{},
		identPaths:      map[*syntax.Tree]map[syntax.NodeID]syntax.NodeID{},
	}
	r.Files = []FileInfo{{}}
	r.Packages = []PackageInfo{{Path: "", Module: ""}}
	r.universe = r.newScope(ScopeUniverse, 0, 0, 0)
	r.prelude = r.newScope(ScopePrelude, r.universe, 0, 0)
	r.declareUniverse()
	r.addPackages(mod)
	return r
}

func moduleRank(m string) int {
	switch m {
	case "":
		return 0
	case "std":
		return 2
	}
	return 1
}

// Packages and files are sorted here so ids and diagnostics are deterministic.
func (r *Result) addPackages(mod *Module) {
	pkgs := slices.Clone(mod.Packages)
	slices.SortStableFunc(pkgs, func(a, b *Package) int {
		return cmp.Or(cmp.Compare(moduleRank(a.Module), moduleRank(b.Module)),
			cmp.Compare(a.Module, b.Module), cmp.Compare(a.Path, b.Path))
	})
	for _, p := range pkgs {
		id := PackageID(len(r.Packages))
		info := PackageInfo{Path: p.Path, Module: p.Module, Std: p.Std}
		info.Scope = r.newScope(ScopePackage, r.prelude, 0, 0)
		files := slices.Clone(p.Files)
		slices.SortStableFunc(files, func(a, b *syntax.Tree) int { return cmp.Compare(a.File.Name, b.File.Name) })
		for _, t := range files {
			fid := FileID(len(r.Files))
			r.Files = append(r.Files, newFileInfo(t, id))
			r.fileIndex[t] = fid
			info.Files = append(info.Files, fid)
			if t == p.Entry {
				info.Entry = fid
			}
		}
		r.Packages = append(r.Packages, info)
		if _, dup := r.pathIndex[p.Path]; !dup {
			r.pathIndex[p.Path] = id
		}
	}
}
