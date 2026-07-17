# Public Model Mapping Design

Date: 2026-07-17

## Goal

Allow admins to define a downstream public model name that maps to one or more provider channels with per-model priority. A toolbar button left of “同步模型” opens the mapping manager.

## Data model

### `public_model_mappings`

| Column | Notes |
|--------|-------|
| id | PK |
| external_id | Unique downstream model name, no provider prefix |
| enabled | bool |
| created_at / updated_at | timestamps |

### `public_model_mapping_targets`

| Column | Notes |
|--------|-------|
| id | PK |
| mapping_id | FK cascade |
| provider | `grok_build` / `grok_web` / `grok_console` |
| upstream_model | Provider-native model id |
| priority | Lower wins; normalized to 1..n on save |
| enabled | bool |

Unique: `(mapping_id, provider, upstream_model)`.

## Resolution

1. Explicit `Build|Web|Console/...` keeps existing namespace lookup.
2. Else if an enabled mapping matches `external_id`:
   - Walk enabled targets by priority.
   - Resolve each target to an internal route (prefer `Provider/external_id` when upstream matches; else provider+upstream; else create manual route).
   - Keep only routes currently available (same availability predicate as today).
   - Return that ordered list to the gateway.
3. Else fall back to existing `PublicIDCandidates` order (Build → Web → Console).

Gateway selection is unchanged: first allowed available route wins.

## Admin API

- `GET/POST /api/admin/v1/model-mappings`
- `PATCH/DELETE /api/admin/v1/model-mappings/:id`

Validation: unique external_id, no provider prefix, ≥1 enabled target, valid provider/upstream.

## Public model list

`/v1/models` includes enabled mapping external IDs that currently resolve to at least one available route.

## Sync compatibility fix

`UpsertDiscovered`: if a discovered public ID is reserved as an alias pointing at a route with the same upstream model, skip instead of failing the whole sync batch.

## UI

Models toolbar: `[模型映射] [同步模型] [添加模型]`.

Mapping dialog: list mappings; create/edit form with external name, ordered channel rows (provider + upstream + enable), save/delete.
