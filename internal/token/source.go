package token

// SourceID identifies a source file across a compilation; 0 means the file
// was never registered (a throwaway parse) and resolves to itself.
type SourceID uint32

// SourceStore hands out SourceIDs and finds files by them.
type SourceStore struct {
	files []*File
}

// Add registers f and stamps its ID; a file already registered keeps it.
func (s *SourceStore) Add(f *File) SourceID {
	if f.ID != 0 && int(f.ID) <= len(s.files) && s.files[f.ID-1] == f {
		return f.ID
	}
	s.files = append(s.files, f)
	f.ID = SourceID(len(s.files))
	return f.ID
}

func (s *SourceStore) File(id SourceID) *File {
	if id == 0 || int(id) > len(s.files) {
		return nil
	}
	return s.files[id-1]
}

func (s *SourceStore) Len() int { return len(s.files) }

// Source lets a store resolve diagnostic locations (diag.Sources).
func (s *SourceStore) Source(id SourceID) *File { return s.File(id) }

// Source lets a lone file resolve its own locations: an unregistered
// location (0) or its own ID.
func (f *File) Source(id SourceID) *File {
	if id == 0 || id == f.ID {
		return f
	}
	return nil
}
