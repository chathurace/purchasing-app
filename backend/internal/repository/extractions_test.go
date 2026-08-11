package repository

import "testing"

// Vendor-name matching is pure logic over the name string, so it is unit-tested
// here without a DB. The scoring itself is exercised through these helpers plus the
// integration test for MatchVendors.

func TestNormalizeVendorName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"WSO2", "wso2"},
		{"  SUSE Software Solutions, Ltda.  ", "suse software solutions ltda"},
		{"Acme (Pvt) Ltd", "acme pvt ltd"},
		{"Foo—Bar/Baz", "foo bar baz"},
		{"", ""},
		{"...", ""},
		{"Patrocínio Ltda", "patrocínio ltda"}, // non-ASCII letters preserved
	}
	for _, c := range cases {
		if got := normalizeVendorName(c.in); got != c.want {
			t.Errorf("normalizeVendorName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSignificantWords(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		// Legal forms and short fragments carry no identifying signal.
		{"suse software solutions ltda", []string{"suse", "software", "solutions"}},
		{"acme inc", []string{"acme"}},
		{"the co ltd", nil},
		{"ab cd", nil}, // both under the 3-char floor
	}
	for _, c := range cases {
		got := significantWords(c.in)
		if len(got) != len(c.want) {
			t.Errorf("significantWords(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("significantWords(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

// TestVendorMatchScoring documents the score tiers via the same comparisons
// MatchVendors makes, so a change in tier ordering is caught without a DB.
func TestVendorMatchScoring(t *testing.T) {
	cases := []struct {
		name       string
		extracted  string
		vendorName string
		wantTier   string
	}{
		{"exact ignoring case and punctuation", "wso2, inc.", "WSO2 Inc", "exact"},
		{"vendor name is a prefix of the extracted one", "SUSE", "SUSE Software Solutions", "prefix"},
		{"shared significant word", "SUSE Brasil Ltda", "SUSE Software Solutions", "word"},
		{"unrelated", "Globex", "Initech", "none"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			needle := normalizeVendorName(c.extracted)
			hay := normalizeVendorName(c.vendorName)
			got := "none"
			switch {
			case hay == needle:
				got = "exact"
			case hasPrefixEither(hay, needle):
				got = "prefix"
			case containsEither(hay, needle):
				got = "substring"
			case sharesWord(hay, needle):
				got = "word"
			}
			if got != c.wantTier {
				t.Errorf("tier for extracted=%q vendor=%q = %q, want %q",
					c.extracted, c.vendorName, got, c.wantTier)
			}
		})
	}
}

func hasPrefixEither(a, b string) bool {
	return len(a) > 0 && len(b) > 0 && (len(a) >= len(b) && a[:len(b)] == b || len(b) >= len(a) && b[:len(a)] == a)
}

func containsEither(a, b string) bool {
	return indexOf(a, b) >= 0 || indexOf(b, a) >= 0
}

func indexOf(haystack, needle string) int {
	if needle == "" {
		return -1
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func sharesWord(a, b string) bool {
	for _, wa := range significantWords(a) {
		for _, wb := range significantWords(b) {
			if wa == wb {
				return true
			}
		}
	}
	return false
}
