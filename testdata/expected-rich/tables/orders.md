---
type: BigQuery Table
title: Orders Table
description: One row per completed customer order.
tags:
  - sales
  - data
source: https://example.com/orders-origin
generated: { by: human:alice, at: 2026-01-02T09:00:00Z }
status: stable
stale_after: 2027-01-01
usage_window: { from: 2026-01-01, to: 2026-12-31 }
verified:
  - by: human:bob
    at: 2026-02-01T10:00:00Z
  - by: binder/0.1.0
    at: 2026-02-02T10:00:00Z
sources:
  - id: schema-doc
    resource: https://example.com/orders-schema
    title: Orders schema
  - resource: https://example.com/orders-origin
  - resource: https://cloud.example.com/bq
    title: BigQuery docs
  - resource: https://example.com/reference
---

# Schema

See the [introduction](/intro.md) for context. #data

# Citations

- [BigQuery docs](https://cloud.example.com/bq)
- https://example.com/reference
