// This file adds the OQ-8 provenance-completeness projection (design v0.4.0
// §4.3 items 3–4): the NodeVerified child table (verified[] rows copied verbatim)
// and the tier/stale derivation view. It is additive to the schema projection in
// ddl.go and introduces NO new trust logic: is_human reuses okf.IsHumanActor —
// the SAME predicate okf.TrustTier applies — and the verified[] attestations are
// copied VERBATIM from the concept (okf.ProjectTrust / projectActorstamps), never
// re-derived. It is read-only: it emits rows and view text and never writes to a
// bundle or mints an identity.
//
// Two honesty properties this file exists to hold, verifiable against the emitted
// bytes:
//   - NodeVerified rows are a lossless copy of each concept's verified[] (order
//     preserved; by/at verbatim as authored; is_human = the human: prefix test).
//     The trust SIGNAL here is is_human, derived from the §7 actor prefix — never
//     from how the attestation was spelled in the source.
//   - tier/stale are exactly reconstructible from Nodes.stale_after + NodeVerified
//     (RecomputeTier/RecomputeStale are the Go statement of the view's SQL, pinned
//     by test against okf.TrustTier / okf.IsStale).
package graph

import (
	"bytes"
	"encoding/csv"
	"strconv"

	"github.com/ghchinoy/binder/internal/okf"
)

// nodeVerifiedColumns is the fixed NodeVerified child-table column order (OQ-8
// item 3; design §4.5): the verbatim projection of each concept's verified[]
// — the lossless record the frozen tier derives from. (node_key, seq) is the
// primary key; is_human is okf.IsHumanActor(by), the SAME predicate TrustTier
// uses, so the snapshot tier and this table cannot disagree.
var nodeVerifiedColumns = []ddlColumn{
	{"node_key", "STRING(MAX)", true, "= Nodes.node_key; never minted"},
	{"seq", "INT64", true, "OQ-8: stable index within the concept's verified[]"},
	{"by", "STRING(MAX)", false, "OQ-8: verified[].by, verbatim as authored"},
	{"at", "STRING(MAX)", false, "OQ-8: verified[].at, verbatim as authored"},
	{"is_human", "BOOL", false, "OQ-8: okf.IsHumanActor(by) — the TrustTier predicate"},
}

// nodeVerifiedCSVHeader is the fixed CSV header for node_verified.csv. Column
// order matches nodeVerifiedColumns so the row artifact and the DDL agree.
var nodeVerifiedCSVHeader = []string{"node_key", "seq", "by", "at", "is_human"}

// NodeVerifiedRow is one emitted NodeVerified row: a single verified[] entry with
// its owning node_key, its stable index seq, the verbatim by/at, and is_human.
type NodeVerifiedRow struct {
	NodeKey string
	Seq     int
	By      string
	At      string
	IsHuman bool
}

// NodeVerifiedRows projects every concept's verified[] into NodeVerified rows, in
// node order (Build's stable sort-by-ID, preserved by Project) then authored seq.
// Order is preserved VERBATIM; by/at are copied verbatim; is_human is
// okf.IsHumanActor. A node with no verified[] contributes no rows. There is no
// second identity path: node_key is the Key already computed by Project.
func (p *Projection) NodeVerifiedRows() []NodeVerifiedRow {
	var rows []NodeVerifiedRow
	for _, n := range p.Nodes {
		for i, v := range n.Verified {
			rows = append(rows, NodeVerifiedRow{
				NodeKey: n.Key,
				Seq:     i,
				By:      v.By,
				At:      v.At,
				IsHuman: okf.IsHumanActor(v.By),
			})
		}
	}
	return rows
}

// NodeVerifiedCSV renders node_verified.csv: a fixed header then one line per
// NodeVerifiedRow, encoded with encoding/csv (LF line endings, minimal quoting).
// It is deterministic and lossless — by/at are written exactly as authored;
// CSV quoting is structural framing, not a transform of the value.
func (p *Projection) NodeVerifiedCSV() []byte {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	// csv.Writer only errors when its io.Writer errors; a bytes.Buffer never does.
	_ = w.Write(nodeVerifiedCSVHeader)
	for _, r := range p.NodeVerifiedRows() {
		_ = w.Write([]string{
			r.NodeKey,
			strconv.Itoa(r.Seq),
			r.By,
			r.At,
			strconv.FormatBool(r.IsHuman),
		})
	}
	w.Flush()
	return buf.Bytes()
}

// derivationViewSQL is the offline SQL/PGQ text that RECOMPUTES tier/stale from
// the STORED FACTS (Nodes.stale_after + the NodeVerified child table) — a real
// derivation, not a restatement of the frozen Nodes.tier / Nodes.stale scalars.
// It is fixed text (no bundle data, no baked date), so it is byte-identical for
// any bundle. The view recomputes as of CURRENT_DATE(); the header documents
// substituting a DATE literal to recompute for any chosen date.
//
// The CASE mirrors okf.TrustTier exactly: a node with no NodeVerified rows
// (LEFT JOIN → NULL counts, coalesced to 0) is 'unverified'; any is_human row
// makes it 'human-reviewed'; otherwise 'machine-confirmed'. The stale expression
// mirrors okf.IsStale exactly: a node without stale_after is never stale;
// otherwise stale iff as_of >= stale_after.
const derivationViewSQL = `-- derivation.sql — offline tier/stale recomputation (binder project).
-- Recomputes the trust tier and staleness from the STORED FACTS
-- (Nodes.stale_after + the NodeVerified child table), so no consumer is stuck
-- with the frozen snapshot in Nodes.tier / Nodes.stale. This is a real
-- derivation over the attestations, not a copy of the frozen scalar.
--
--   tier : 'unverified'        when the node has no NodeVerified rows
--          'human-reviewed'    when any NodeVerified.is_human is TRUE
--          'machine-confirmed' otherwise
--          (mirrors okf.TrustTier over verified[])
--   stale: stale_after IS NOT NULL AND <as_of> >= stale_after
--          (mirrors okf.IsStale; a node without stale_after is never stale)
--
-- The view recomputes as of CURRENT_DATE(). To recompute for any chosen date,
-- replace CURRENT_DATE() with a DATE literal, e.g. DATE '2026-08-18'.
CREATE VIEW NodeTrustDerived AS
SELECT
  n.node_key,
  CASE
    WHEN COALESCE(v.verified_count, 0) = 0 THEN 'unverified'
    WHEN COALESCE(v.human_count, 0) > 0    THEN 'human-reviewed'
    ELSE 'machine-confirmed'
  END AS tier,
  (n.stale_after IS NOT NULL AND CURRENT_DATE() >= n.stale_after) AS stale
FROM Nodes AS n
LEFT JOIN (
  SELECT
    node_key,
    COUNT(*) AS verified_count,
    COUNTIF(is_human) AS human_count
  FROM NodeVerified
  GROUP BY node_key
) AS v ON v.node_key = n.node_key;
`

// DerivationView returns the derivation.sql bytes (see derivationViewSQL). It is
// a pure constant — deterministic and independent of the bundle.
func (p *Projection) DerivationView() []byte {
	return []byte(derivationViewSQL)
}

// RecomputeTier is the Go statement of exactly what DerivationView()'s CASE
// computes: the trust tier reconstructed from stored NodeVerified rows ALONE.
// Proven by test to equal okf.TrustTier for the same concept, so the frozen
// Nodes.tier is exactly reconstructible from the child table (never a stored or
// fabricated trust fact).
func RecomputeTier(rows []NodeVerifiedRow) okf.Tier {
	if len(rows) == 0 {
		return okf.TierUnverified
	}
	for _, r := range rows {
		if r.IsHuman {
			return okf.TierHumanReviewed
		}
	}
	return okf.TierMachineConfirmed
}

// RecomputeStale is the Go statement of DerivationView()'s stale expression:
// staleness reconstructed from the stored stale_after column and a chosen as-of
// date. Proven by test to equal okf.IsStale. A node without stale_after is never
// stale; otherwise stale iff asOf >= staleAfter (ISO dates compare lexically =
// chronologically).
func RecomputeStale(staleAfter, asOf string) bool {
	if staleAfter == "" {
		return false
	}
	return asOf >= staleAfter
}
