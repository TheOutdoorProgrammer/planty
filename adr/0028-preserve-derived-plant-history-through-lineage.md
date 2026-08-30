# 28. Preserve derived plant history through lineage

Date: 2026-08-30

## Status

Accepted.

## Context and Problem Statement

A mixed seedling record can produce several independently tracked plants. Each child needs the source's earlier history without duplicating mutable observations, photos, and verdicts into records that would drift.

## Considered Options

1. Store a parent link and inherit source history through the derivation time
2. Copy every historical row into each child when it is created
3. Leave history only on the source and show a link

## Decision Outcome

Create each derived plant with a single immutable parent relation and derivation timestamp. Timeline reads walk the ancestry chain and include ancestor events only through the applicable derivation cutoff. Current sensor assignments and future events remain local to the child. The source stays independent and can later be marked removed without deleting any history.

## Consequences

### Good

- Every child retains its seedling story
- One historical event has one canonical row
- Multi-generation propagation remains possible
- Removing the source does not break descendants

### Bad

- Timeline queries become more complex
- Inherited events need source attribution in API and UI
- The model deliberately supports one parent per derived plant

### Rejected because

- Copying creates duplicate facts and makes corrections inconsistent
- A source-only link does not satisfy the requirement that each new plant holds its earlier history
