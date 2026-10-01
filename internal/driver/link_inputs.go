package driver

import "path/filepath"

// linkInputs: with pinCompDir, out becomes absolute and inputs relative to
// dir, because cc runs with cmd.Dir = dir while inputs may be cwd-relative.
func linkInputs(pinCompDir bool, dir, out string, inputs []string) (string, []string, error) {
	if !pinCompDir {
		return out, inputs, nil
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", nil, err
	}
	rel := make([]string, 0, len(inputs))
	for _, in := range inputs {
		absIn, err := filepath.Abs(in)
		if err != nil {
			return "", nil, err
		}
		r, err := filepath.Rel(dir, absIn)
		if err != nil {
			return "", nil, err
		}
		rel = append(rel, r)
	}
	return abs, rel, nil
}
