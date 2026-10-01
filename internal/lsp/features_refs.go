package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// entityLocation points at the declaring name of an entity; nil for
// entities without source (predeclared types, intrinsics).
func (s *Server) entityLocation(snap *snapshot, ent sem.EntityID) *protocol.Location {
	e := snap.res.Entity(ent)
	if e.File == 0 {
		return nil
	}
	target := snap.res.Tree(e.File)
	if target.File.Generated {
		return nil
	}
	sp := target.Span(e.Node)
	if e.Tok != 0 {
		sp = target.Toks[e.Tok].Span
	}
	return &protocol.Location{URI: uri.File(s.pathOfFile(snap, target.File.Name)), Range: spanRange(target.File, sp)}
}

// nameSpan is the span of the name a use node refers with: the member
// token of a member expression, the whole node otherwise.
func nameSpan(t *syntax.Tree, n syntax.NodeID) token.Span {
	if t.Kind(n) == syntax.MemberExpr {
		return t.Toks[t.Nodes[n].Tok].Span
	}
	return t.Span(n)
}

// References lists every use of the entity (and, for methods, of the
// interface requirements or witnesses it corresponds to, as gopls does).
func (s *Server) References(ctx context.Context, p *protocol.ReferenceParams) ([]protocol.Location, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, t, _, ent := s.entityAt(ctx, p.TextDocumentPositionParams)
	if t == nil || ent == 0 {
		return nil, nil
	}
	targets := map[sem.EntityID]bool{ent: true}
	if isMethod(snap.res, ent) {
		for _, m := range implementations(snap.res, ent) {
			targets[m] = true
		}
	}
	out := []protocol.Location{}
	for id := range targets {
		if p.Context.IncludeDeclaration {
			if loc := s.entityLocation(snap, id); loc != nil {
				out = append(out, *loc)
			}
		}
	}
	for i := 1; i < len(snap.res.Files); i++ {
		f := &snap.res.Files[i]
		if f.Tree.File.Generated {
			continue
		}
		for n, use := range f.Uses {
			if targets[use] {
				out = append(out, protocol.Location{URI: uri.File(s.pathOfFile(snap, f.Tree.File.Name)), Range: spanRange(f.Tree.File, nameSpan(f.Tree, syntax.NodeID(n)))})
			}
		}
	}
	return out, nil
}

// Implementation follows gopls: an interface leads to the types
// conforming to it, a type to the interfaces it conforms to, and a method
// to its counterparts on the other side.
func (s *Server) Implementation(ctx context.Context, p *protocol.ImplementationParams) (protocol.DefinitionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, t, _, ent := s.entityAt(ctx, p.TextDocumentPositionParams)
	out := protocol.LocationSlice{}
	if t == nil || ent == 0 {
		return out, nil
	}
	for _, target := range implementations(snap.res, ent) {
		if loc := s.entityLocation(snap, target); loc != nil {
			out = append(out, *loc)
		}
	}
	return out, nil
}

func isMethod(res *sem.Result, ent sem.EntityID) bool {
	return res.Entity(ent).Kind == sem.EntFn && res.Fn(ent).Owner != 0
}

// implementations pairs interfaces with conforming types and requirements
// with the methods witnessing them, in either direction.
func implementations(res *sem.Result, ent sem.EntityID) []sem.EntityID {
	e := res.Entity(ent)
	var out []sem.EntityID
	switch {
	case e.Kind == sem.EntInterface:
		for _, ty := range declared(res, sem.EntType) {
			if _, ok := conforms(res, ty, ent); ok {
				out = append(out, ty)
			}
		}
	case e.Kind == sem.EntType:
		for _, iface := range declared(res, sem.EntInterface) {
			if _, ok := conforms(res, ent, iface); ok {
				out = append(out, iface)
			}
		}
	case isMethod(res, ent) && res.Entity(res.Fn(ent).Owner).Kind == sem.EntInterface:
		iface := res.Fn(ent).Owner
		for _, ty := range declared(res, sem.EntType) {
			if witnesses, ok := conforms(res, ty, iface); ok {
				for i, req := range res.Iface(iface).Reqs {
					if req == ent {
						out = append(out, witnesses[i])
					}
				}
			}
		}
	case isMethod(res, ent):
		owner := res.Fn(ent).Owner
		for _, iface := range declared(res, sem.EntInterface) {
			if witnesses, ok := conforms(res, owner, iface); ok {
				for i, w := range witnesses {
					if w == ent {
						out = append(out, res.Iface(iface).Reqs[i])
					}
				}
			}
		}
	}
	return out
}

func conforms(res *sem.Result, ty, iface sem.EntityID) ([]sem.EntityID, bool) {
	return res.Implements(res.SelfType(ty), res.SelfType(iface), res.Entity(ty).Pkg)
}

// declared lists the source-declared entities of one kind.
func declared(res *sem.Result, kind sem.EntityKind) []sem.EntityID {
	var out []sem.EntityID
	for i := range res.Entities {
		if e := &res.Entities[i]; e.Kind == kind && e.File != 0 {
			out = append(out, sem.EntityID(i))
		}
	}
	return out
}
