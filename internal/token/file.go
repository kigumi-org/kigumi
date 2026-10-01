package token

import "sort"

// File maps byte offsets to 1-based line and column numbers.
type File struct {
	ID SourceID
	// Generated marks compiler-made source (derived codecs): it has no
	// path an editor could open.
	Generated bool
	Name      string
	Src       []byte
	lines     []Pos
}

func NewFile(name string, src []byte) *File {
	f := &File{Name: name, Src: src, lines: []Pos{0}}
	for i, b := range src {
		if b == '\n' {
			f.lines = append(f.lines, Pos(i+1))
		}
	}
	return f
}

func (f *File) LineCount() int { return len(f.lines) }

// Line returns the 1-based line containing pos.
func (f *File) Line(pos Pos) int {
	return sort.Search(len(f.lines), func(i int) bool { return f.lines[i] > pos })
}

// Column returns the 1-based byte column of pos within its line.
func (f *File) Column(pos Pos) int {
	line := f.Line(pos)
	return int(pos-f.lines[line-1]) + 1
}

// LineSpan returns the byte range of a 1-based line, excluding the newline.
func (f *File) LineSpan(line int) Span {
	if line < 1 || line > len(f.lines) {
		return Span{}
	}
	start := f.lines[line-1]
	end := Pos(len(f.Src))
	if line < len(f.lines) {
		end = f.lines[line] - 1
	}
	return Span{Start: start, End: end}
}

func (f *File) LineText(line int) string {
	s := f.LineSpan(line)
	return string(f.Src[s.Start:s.End])
}
