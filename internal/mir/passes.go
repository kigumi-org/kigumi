package mir

import "fmt"

// Transform is one pass over a program: a fold, a rewrite, an
// optimization.
type Transform struct {
	Name string
	Run  func(*Program) error
}

// Apply verifies the program, then runs each transform and verifies again.
func (p *Program) Apply(passes ...Transform) error {
	if err := p.Verify(); err != nil {
		return fmt.Errorf("before passes: %w", err)
	}
	for _, t := range passes {
		if err := t.Run(p); err != nil {
			return fmt.Errorf("%s: %w", t.Name, err)
		}
		if err := p.Verify(); err != nil {
			return fmt.Errorf("after %s: %w", t.Name, err)
		}
	}
	return nil
}
