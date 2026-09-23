package broadcast_test

import "testing"

// oneSection unwraps a table that this fixture expects to be a single section.
//
// SDT and BAT build TABLES -- section_number 0 through last_section_number -- because a real Sky
// line-up does not fit in the 1021 bytes a section holds. The firmware fixtures in this package
// announce a channel or two and genuinely are one section, and they go on to mutate the bytes they
// push, so this states that expectation rather than indexing [0] and hoping. A fixture that grows
// past one section fails here instead of silently testing a third of itself.
func oneSection(t *testing.T, sections [][]byte, err error) []byte {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 {
		t.Fatalf("this fixture was expected to fit one section and built %d; pushing only the "+
			"first would put an incomplete table on air", len(sections))
	}
	return sections[0]
}
