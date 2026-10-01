package sem

import "kigumi/internal/syntax"

type ScopeKind uint8

const (
	ScopeUniverse ScopeKind = iota
	ScopePrelude
	ScopePackage
	ScopeFile
	ScopeEntry
	ScopeScript
	ScopeFn
	ScopeLambda
	ScopeBlock
	ScopeArm
)

// Binding is one name in a scope: an entity, or an overload set (Ent == 0).
type Binding struct {
	Ent EntityID
	Set OverloadSetID
}

type Scope struct {
	Kind   ScopeKind
	Parent ScopeID
	Node   syntax.NodeID
	File   FileID
	Fn     EntityID
	Loop   bool
	Names  map[string]Binding
}

func (r *Result) newScope(kind ScopeKind, parent ScopeID, file FileID, node syntax.NodeID) ScopeID {
	s := Scope{Kind: kind, Parent: parent, File: file, Node: node, Names: map[string]Binding{}}
	if parent != 0 {
		s.Fn = r.Scopes[parent].Fn
	}
	r.Scopes = append(r.Scopes, s)
	return ScopeID(len(r.Scopes) - 1)
}

func (r *Result) scope(id ScopeID) *Scope { return &r.Scopes[id] }

func (r *Result) define(id ScopeID, name string, b Binding) (Binding, bool) {
	names := r.Scopes[id].Names
	if prev, dup := names[name]; dup {
		return prev, true
	}
	names[name] = b
	return Binding{}, false
}

func (r *Result) lookup(id ScopeID, name string) (Binding, ScopeID, bool) {
	for id != 0 {
		s := &r.Scopes[id]
		if b, ok := s.Names[name]; ok {
			return b, id, true
		}
		id = s.Parent
	}
	return Binding{}, 0, false
}

// A lambda boundary between inner and outer makes a local reference a
// capture.
func (r *Result) crossesLambda(inner, outer ScopeID) bool {
	for id := inner; id != 0 && id != outer; id = r.Scopes[id].Parent {
		if r.Scopes[id].Kind == ScopeLambda {
			return true
		}
	}
	return false
}
