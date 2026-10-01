package mir

import (
	"fmt"

	"kigumi/internal/sem"
)

func (p *Program) funcName(ent sem.EntityID) string {
	e := p.R.Entity(ent)
	switch e.Kind {
	case sem.EntImplicitMain:
		return "main"
	case sem.EntTest:
		return "test." + e.Name
	case sem.EntClosure:
		return fmt.Sprintf("%s.closure%d", p.funcName(e.Parent), p.closureOrdinal(ent))
	}
	name := p.R.Packages[e.Pkg].Path + "." + e.Name
	if owner := p.R.Fn(ent).Owner; owner != 0 {
		name = p.R.Packages[e.Pkg].Path + "." + p.R.Entity(owner).Name + "." + e.Name
	}
	return name
}
