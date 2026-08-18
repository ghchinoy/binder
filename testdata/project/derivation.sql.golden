-- derivation.sql — offline tier/stale recomputation (binder project).
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
