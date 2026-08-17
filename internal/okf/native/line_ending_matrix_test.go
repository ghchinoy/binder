package native

import (
	"fmt"
	"strings"
	"testing"
)

// Test 6 — THE REGRESSION WALL for issue #123.
//
// The 64-case matrix from the plan's Appendix A, promoted to a table test:
// four top-level keys, the four interior line endings drawn from {LF, CR} in all
// 2^4 = 16 combinations, and 4 rewrite positions = 64 cells. For EVERY cell the
// same four properties are asserted, which together catch all four measured
// outcomes plus the read-side refusal:
//
//   - the input is ACCEPTED on read      (else: outcome 5, unterminated-fence refusal)
//   - the output RE-PARSES               (else: outcome 4, the brick)
//   - NO key is lost                     (else: outcome 1, the silent destroy)
//   - NO key is duplicated               (else: outcome 2, the stale duplicate)
//   - NO non-target value is mutated     (else: outcome 3, value absorption)
//
// THE CLOCK NOTE. This is a codec-level test — no enrich, no config origin, no
// SOURCE_DATE_EPOCH. The rewrite is a direct map mutation with a value chosen here,
// so no dedup/collision can mask a cell.
//
// On the pre-fix code the plan measured 16 clean / 12 destroyed / 12 stale-duplicate
// / 24 input-refused across this space; the buggy cells fail here today. The all-LF
// cells are the built-in control and pass today.

// buildMatrixDoc assembles a 4-key frontmatter with the given interior line
// endings (one per key, endings[i] follows key i; endings[3] precedes the closing
// fence). Opening/closing fences are always LF so the block is recognised as
// frontmatter at all (a pure classic-Mac file is rejected earlier, by design).
func buildMatrixDoc(keys, vals, endings []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	for i := range keys {
		b.WriteString(keys[i])
		b.WriteString(": ")
		b.WriteString(vals[i])
		b.WriteString(endings[i])
	}
	b.WriteString("---\n# Body\n")
	return b.String()
}

func TestLineEndingShapeMatrix(t *testing.T) {
	keys := []string{"a", "b", "c", "d"}
	baseVals := []string{"va", "vb", "vc", "vd"}
	const newVal = "REWRITTEN"

	c := New()
	var loneCRSeen, cleanSeen int

	for shape := 0; shape < 16; shape++ {
		endings := make([]string, 4)
		hasLoneCR := false
		for bit := 0; bit < 4; bit++ {
			if shape&(1<<bit) != 0 {
				endings[bit] = "\r"
				hasLoneCR = true
			} else {
				endings[bit] = "\n"
			}
		}
		if hasLoneCR {
			loneCRSeen++
		} else {
			cleanSeen++
		}

		for pos := 0; pos < 4; pos++ {
			name := fmt.Sprintf("endings_%04b/rewrite_%s", shape, keys[pos])
			t.Run(name, func(t *testing.T) {
				vals := append([]string(nil), baseVals...)
				raw := buildMatrixDoc(keys, vals, endings)

				con, err := c.ParseConcept("doc.md", []byte(raw))
				if err != nil {
					// Outcome 5: the input is refused on read.
					t.Fatalf("input refused on read: %v\ninput: %q", err, raw)
				}

				con.Frontmatter.Set(keys[pos], newVal) // rewrite one key in place

				out, err := c.Serialize(con)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				re, reErr := c.ParseConcept("doc.md", out)
				if reErr != nil {
					// Outcome 4: binder wrote a file it cannot read back.
					t.Fatalf("output does not re-parse (%v):\n%q", reErr, out)
				}

				// No key lost, right order (outcome 1).
				gotKeys := re.Frontmatter.Keys()
				if strings.Join(gotKeys, ",") != strings.Join(keys, ",") {
					t.Errorf("key set changed: got %v want %v\noutput:\n%q", gotKeys, keys, out)
				}

				// No key duplicated in the serialized bytes (outcome 2).
				fm := frontmatterOf(t, string(out))
				for _, k := range keys {
					if n := strings.Count(fm, k+":"); n != 1 {
						t.Errorf("key %q appears %d times in output, want 1 (stale duplicate)\noutput frontmatter:\n%q", k, n, fm)
					}
				}

				// Only the target value changed (outcome 3).
				for i, k := range keys {
					want := baseVals[i]
					if i == pos {
						want = newVal
					}
					if v, _ := re.Frontmatter.Get(k); v != want {
						t.Errorf("value of %q = %#v, want %q (unexpected mutation)\noutput:\n%q", k, v, want, out)
					}
				}
			})
		}
	}

	// Anti-vacuity for the harness itself: the space really did include both
	// lone-CR shapes (the ones under test) and the clean control shape.
	if loneCRSeen != 15 || cleanSeen != 1 {
		t.Fatalf("matrix construction wrong: loneCR shapes=%d clean shapes=%d (want 15/1)", loneCRSeen, cleanSeen)
	}
}

// TestLineEndingShapeMatrix_ValueShapes is the value-shape extension the plan asked
// for. The full cross product — {16 endings} x {4 positions} x {5 value shapes} =
// 320 cells — is combinatorially unreasonable AND redundant: the settled
// discriminator (repro-samples §5.3, plan §1.2) is not the full ending permutation
// but WHERE the lone CR sits relative to the rewritten key's value. So this is a
// deliberately reduced, representative subset:
//
//   {5 value shapes: block map, block seq, flow seq, flow map, block scalar}
//     x {lone CR BEFORE the shaped key, lone CR INSIDE its multi-line value}
//
// and the shaped key is the one rewritten. DROPPED, and why:
//   - the other 3 rewrite positions: covered scalar-wise by the 64-case matrix; the
//     mechanism is position-independent (one lone CR costs one following key).
//   - the full 16-ending permutation per shape: the discriminator is CR-inside vs
//     CR-adjacent, not the permutation; both are represented.
//   - flow/block scalar as a lone CR-BEFORE-only vs INSIDE distinction where a shape
//     is single-line (flow seq/map have no meaningful interior line): only the
//     BEFORE variant is used for those, noted per row.
//
// Rewriting the shaped key to a plain scalar exercises the "changed key with a
// multi-line block value" splice path (the brick route) as well as the destroy/dup
// routes. Assertions match the core wall: accepted, re-parses, no key lost, no key
// duplicated, only the target changed.
func TestLineEndingShapeMatrix_ValueShapes(t *testing.T) {
	// Each case is a full frontmatter interior with a `lead` scalar, a shaped middle
	// key, and a `trail` scalar. `rewrite` names the key set to a new scalar value.
	cases := []struct {
		name    string
		fm      string // interior between the fences
		rewrite string
	}{
		{
			"block_map/cr_before",
			"lead: L\r" + "meta:\n  k1: v1\n  k2: v2\n" + "trail: T\n",
			"meta",
		},
		{
			"block_map/cr_inside",
			"lead: L\n" + "meta:\n  k1: v1\r  k2: v2\n" + "trail: T\n",
			"meta",
		},
		{
			"block_seq/cr_before",
			"lead: L\r" + "items:\n  - one\n  - two\n" + "trail: T\n",
			"items",
		},
		{
			"block_seq/cr_inside",
			"lead: L\n" + "items:\n  - one\r  - two\n" + "trail: T\n",
			"items",
		},
		{
			"flow_seq/cr_before", // single-line value: only the CR-adjacent variant applies
			"lead: L\r" + "list: [one, two]\n" + "trail: T\n",
			"list",
		},
		{
			"flow_map/cr_before", // single-line value: only the CR-adjacent variant applies
			"lead: L\r" + "obj: {k1: v1, k2: v2}\n" + "trail: T\n",
			"obj",
		},
		{
			"block_scalar/cr_before",
			"lead: L\r" + "note: |\n  line1\n  line2\n" + "trail: T\n",
			"note",
		},
		{
			"block_scalar/cr_inside",
			"lead: L\n" + "note: |\n  line1\r  line2\n" + "trail: T\n",
			"note",
		},
	}

	c := New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := "---\n" + tc.fm + "---\n# Body\n"
			con, err := c.ParseConcept("doc.md", []byte(raw))
			if err != nil {
				t.Fatalf("input refused on read: %v\ninput: %q", err, raw)
			}
			wantKeys := con.Frontmatter.Keys()
			// Anti-vacuity: lead / shaped / trail are all present up front.
			if !con.Frontmatter.Has("lead") || !con.Frontmatter.Has(tc.rewrite) || !con.Frontmatter.Has("trail") {
				t.Fatalf("setup: expected lead/%s/trail keys, got %v", tc.rewrite, wantKeys)
			}

			con.Frontmatter.Set(tc.rewrite, newScalar) // rewrite the shaped key

			out, err := c.Serialize(con)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			re, reErr := c.ParseConcept("doc.md", out)
			if reErr != nil {
				t.Fatalf("output does not re-parse (%v):\n%q", reErr, out)
			}

			// No key lost / no reordering.
			if got := re.Frontmatter.Keys(); strings.Join(got, ",") != strings.Join(wantKeys, ",") {
				t.Errorf("key set changed: got %v want %v\noutput:\n%q", got, wantKeys, out)
			}
			// No duplicate of the surrounding scalar keys or the shaped key.
			fm := frontmatterOf(t, string(out))
			for _, k := range []string{"lead", tc.rewrite, "trail"} {
				if n := strings.Count(fm, k+":"); n != 1 {
					t.Errorf("key %q appears %d times, want 1 (stale duplicate)\noutput frontmatter:\n%q", k, n, fm)
				}
			}
			// Surrounding scalars unmutated; shaped key holds the new value.
			if v, _ := re.Frontmatter.Get("lead"); v != "L" {
				t.Errorf("lead mutated: got %#v want %q\noutput:\n%q", v, "L", out)
			}
			if v, _ := re.Frontmatter.Get("trail"); v != "T" {
				t.Errorf("trail mutated: got %#v want %q\noutput:\n%q", v, "T", out)
			}
			if v, _ := re.Frontmatter.Get(tc.rewrite); v != newScalar {
				t.Errorf("rewritten key %q = %#v, want %q\noutput:\n%q", tc.rewrite, v, newScalar, out)
			}
		})
	}
}

// TestLineEndingShapeMatrix_BlockSeqPerPosition is the load-bearing companion to
// the scalar 64-cell matrix. Measurement (relayed by binder-030-em) confirmed the
// scalar matrix produces ZERO bricks — the brick requires a lone CR INSIDE the
// rewritten key's multi-line BLOCK value, which no scalar cell has. So this test
// keeps at least one block-sequence shape at EVERY rewrite position: for each of the
// four positions, that position's key is a block sequence with an interior lone CR,
// and it is the key rewritten (a stamp-style append). This is the cell that bricks —
// the headline outcome the scalar matrix cannot reach.
//
// The dropped dimensions, named: the full {16 endings} x {5 value shapes} per
// position cross product is not enumerated (combinatorially unreasonable and
// redundant with the discriminator being CR-inside-the-block-value); between-key
// separators here are LF so the ONE lone CR under test sits inside the rewritten
// block value and the brick is isolated per position. The other four value shapes at
// a single position are covered by TestLineEndingShapeMatrix_ValueShapes.
func TestLineEndingShapeMatrix_BlockSeqPerPosition(t *testing.T) {
	keys := []string{"a", "b", "c", "d"}
	scalarVals := []string{"va", "vb", "vc", "vd"}
	c := New()

	for pos := 0; pos < 4; pos++ {
		t.Run("rewrite_"+keys[pos], func(t *testing.T) {
			var b strings.Builder
			b.WriteString("---\n")
			for i, k := range keys {
				if i == pos {
					// block sequence with a lone CR between its two items
					b.WriteString(k + ":\n  - one\r  - two\n")
				} else {
					b.WriteString(k + ": " + scalarVals[i] + "\n")
				}
			}
			b.WriteString("---\n# Body\n")
			raw := b.String()

			con, err := c.ParseConcept("doc.md", []byte(raw))
			if err != nil {
				t.Fatalf("input refused on read: %v\ninput: %q", err, raw)
			}
			// Anti-vacuity: the rewritten key really is a 2-item block sequence.
			bv, _ := con.Frontmatter.Get(keys[pos])
			list, ok := bv.([]any)
			if !ok || len(list) != 2 {
				t.Fatalf("setup: %q is not a 2-item block sequence (got %#v)", keys[pos], bv)
			}

			// Rewrite the block-sequence key: append an item (the applyVerifiedBy shape).
			con.Frontmatter.Set(keys[pos], append(list, "three"))
			out, err := c.Serialize(con)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			// Anti-vacuity: the append actually landed.
			if !strings.Contains(string(out), "three") {
				t.Fatalf("append did not land — nothing written:\n%q", out)
			}

			// THE BRICK PROPERTY: binder must be able to read back what it wrote.
			re, reErr := c.ParseConcept("doc.md", out)
			if reErr != nil {
				t.Fatalf("brick at position %d: binder wrote a file it cannot read back (%v):\n%q", pos, reErr, out)
			}
			// No key lost, none duplicated, surrounding scalars intact.
			if got := re.Frontmatter.Keys(); strings.Join(got, ",") != strings.Join(keys, ",") {
				t.Errorf("key set changed: got %v want %v\noutput:\n%q", got, keys, out)
			}
			fm := frontmatterOf(t, string(out))
			for _, k := range keys {
				if n := strings.Count(fm, k+":"); n != 1 {
					t.Errorf("key %q appears %d times, want 1\noutput frontmatter:\n%q", k, n, fm)
				}
			}
			for i, k := range keys {
				if i == pos {
					continue
				}
				if v, _ := re.Frontmatter.Get(k); v != scalarVals[i] {
					t.Errorf("scalar %q mutated: got %#v want %q\noutput:\n%q", k, v, scalarVals[i], out)
				}
			}
		})
	}
}

const newScalar = "REWRITTEN"
