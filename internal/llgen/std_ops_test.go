package llgen

import (
	"os"
	"regexp"
	"testing"
)

var (
	enumBlockRe = regexp.MustCompile(`(?s)enum std_op \{(.*?)\};`)
	enumEntryRe = regexp.MustCompile(`OP_([A-Z0-9_]+)`)
	caseLabelRe = regexp.MustCompile(`case OP_([A-Z0-9_]+):`)
)

// TestStdOpsMatchC checks rt_core.c's `enum std_op` matches stdOpNames in
// order, and that rt_std_id has exactly one case per declared op.
func TestStdOpsMatchC(t *testing.T) {
	src, err := os.ReadFile("runtime/rt_core.c")
	if err != nil {
		t.Fatal(err)
	}
	block := enumBlockRe.FindSubmatch(src)
	if block == nil {
		t.Fatal("enum std_op { ... } not found in rt_core.c")
	}
	var enumNames []string
	for _, m := range enumEntryRe.FindAllSubmatch(block[1], -1) {
		enumNames = append(enumNames, string(m[1]))
	}
	if len(enumNames) != len(stdOpNames) {
		t.Fatalf("enum std_op has %d entries, stdOpNames has %d", len(enumNames), len(stdOpNames))
	}
	for i, name := range stdOpNames {
		if enumNames[i] != name {
			t.Errorf("id %d: Go has %s, C's enum std_op has %s at the same position", i, name, enumNames[i])
		}
	}
	cases := map[string]int{}
	for _, m := range caseLabelRe.FindAllSubmatch(src, -1) {
		cases[string(m[1])]++
	}
	declared := map[string]bool{}
	for _, name := range enumNames {
		declared[name] = true
		if cases[name] == 0 {
			t.Errorf("OP_%s is declared but rt_std_id has no case for it", name)
		} else if cases[name] > 1 {
			t.Errorf("OP_%s has %d cases in rt_std_id, want exactly one", name, cases[name])
		}
	}
	for name := range cases {
		if !declared[name] {
			t.Errorf("rt_std_id has a case for OP_%s, which enum std_op does not declare", name)
		}
	}
}
