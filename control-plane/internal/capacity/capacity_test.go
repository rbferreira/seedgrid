package capacity

import (
	"context"
	"testing"
)

// TestFits_UnknownSizeNeverBlocks: size <=0 (magnet without metadata) never blocks —
// there is no way to judge, and blocking would be a false positive (ADR-032).
func TestFits_UnknownSizeNeverBlocks(t *testing.T) {
	fits, known, _, err := Fits(context.Background(), nil, "any", 0, 5<<30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fits || known {
		t.Errorf("unknown size: fits=%v known=%v, want fits=true known=false", fits, known)
	}
}

// TestProjectedFreeExpr_MentionsTheFields ensures the SQL expression (single source)
// references the expected fields — a refactor that renames them breaks all 3 consumers
// at once; this test fails first.
func TestProjectedFreeExpr_MentionsTheFields(t *testing.T) {
	expr := ProjectedFreeExpr()
	for _, sub := range []string{"disk_free_bytes", "size_bytes", "progress", "h.", "n.id"} {
		if !contains(expr, sub) {
			t.Errorf("ProjectedFreeExpr does not mention %q: %s", sub, expr)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
