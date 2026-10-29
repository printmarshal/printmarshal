[Home](Home.md) · [Configuration reference](Configuration-reference.md) · [API reference](API-reference.md)

# Consumables and stock

## What is tracked

An installed supply is a reading on a device slot; a consumable is a stock item representing spare units on hand. Polls update installed levels; they do not set the operator's stock count. Inventory includes toner, ink, drum, fuser, waste receptacles, maintenance kits, staples and `other` (exact enum values are listed below). Cost and reorder URL are optional reference fields; there is no purchasing/accounting engine.

| Stock JSON key | Meaning |
| --- | --- |
| `name`, `type`, `color` | Human label, consumable type and normalized color. C/M/Y/K aliases normalize to cyan/magenta/yellow/black; grey becomes gray. |
| `partNumber` | Primary matching key against a supply description. |
| `compatibleModels` | Case-insensitive model/make substring entries; `*` matches all. |
| `unit`, `onHand` | Count unit and operator-owned quantity. Negative results are clamped to zero by stock adjustment. |
| `warningAt`, `criticalAt` | Low and critical thresholds; zero on hand always means `out`. |
| `location`, `notes`, `reorderUrl`, `unitCost` | Optional operator references; cost can be absent/null. |
| `id`, `createdAt`, `updatedAt` | Persisted identity/timestamps. |

Stock state is `out` at ≤0, otherwise `critical` at/below criticalAt, `low` at/below warningAt, else `ok`. Operator create/update/delete and stock actions are audited; thresholds also reconcile fleet stock alerts (`consumable:<id>`). Only admin changes module-wide settings.

## Counts and history

`POST /api/consumables/{id}/adjust-stock` accepts `set` or `delta`, plus `reason`. `set` is an absolute count, not an increment. Ordinary PATCH does not accept `onHand`; use stock endpoints so history stays complete. Batch save uses `POST /api/consumables/bulk-stock` with `items:[{id,onHand}]` and `reason`. Inspect returned `errors`, not just HTTP 200: batch/import responses can include per-item failures.

```bash
PM=http://localhost:8080
curl -s -H "Authorization: Bearer $SESSION" -H 'Content-Type: application/json' \
  -d '{"delta":3,"reason":"Delivery received"}' \
  "$PM/api/consumables/$CONSUMABLE_ID/adjust-stock"
```

Each stock event records actor, kind, delta, resulting quantity, reason, optional device/slot, status and timestamp. Kinds are `set`, `add`, `remove`, `replacement`, `import`. Replacement statuses are `pending`, `logged`, `applied`, `dismissed`, `confirmed`; confirmed refers to the original detection, while the decrement is a separate applied event. List item history with `/api/consumables/{id}/events`.

## Automatic and manual supply links

Links map `(deviceId,supplyIndex)` to a consumable ID and source `auto` or `manual`. Automatic links are recomputed after successful supply polling and stock edits; manual links are never overwritten by automatic matching. Set a manual link with `PUT /api/devices/{id}/supplies/{slot}/link` and `{"consumableId":"..."}`; DELETE removes it, after which later matching may recreate an automatic link.

Matching first finds part numbers contained in the supply description. One match wins; with several part-number matches it prefers a compatible model and otherwise takes the first candidate. Without a part-number match it uses mapped type/color/model. Items with no model list are fallback candidates only when unambiguous. Ambiguous or missing data should be resolved with a manual link, rather than assuming every automatically selected SKU is correct.

`GET /api/consumables/installed` is the fleet supply view; `unmatched=true` restricts it, and `forecast=false` suppresses expensive per-supply forecasting. `/api/devices/{id}/supplies` returns decorated installed supplies and pending replacements. The Unmatched view can suggest a stock item from scan data when enabled; suggestion is not automatic procurement.

## Replacement detection and confirmation

Compare consecutive stored supply readings by index. A rise of at least `replacementJumpPct` in remaining supply counts as replacement; waste receptacles invert the percentage, so emptying waste can be detected. A slot missing from either reading, missing percentages or class `other` is ignored. Only linked slots produce a stock replacement event.

| `autoDecrement` | Detection effect |
| --- | --- |
| `off` | Record `logged`; quantity unchanged. |
| `prompt` (default) | Record `pending`; operator confirms or dismisses. |
| `automatic` | Record `applied` and subtract one unit. |

Pending events are listed by `GET /api/consumables/replacements` (use `all=true` for other statuses). Confirm/dismiss use the corresponding POST endpoints. Confirmation conditionally changes pending → confirmed so a concurrent confirmation loses; if stock adjustment fails it attempts to return the detection to pending. The status claim and stock adjustment are not a single cross-table transaction; inspect logged rollback failures or interrupted operations before retrying manually. Dismiss records no decrement.

`POST /api/devices/{id}/supplies/{slot}/mark-replaced` accepts `decrement`, optional `consumableId`, and `reason`. This is an explicit stock-history action, not a device write and not a dry run; `decrement:false` logs without reducing quantity. It also marks matching pending detections confirmed.

## Forecasts and settings

Settings are saved under database key `consumables`, exposed by `GET/PUT /api/consumables/settings`. There are no JSON-file or direct environment keys for this module.

| JSON key | Default | Behavior |
| --- | --- | --- |
| `defaultWarningAt` | 2 | Defaults for newly created/suggested stock items. |
| `defaultCriticalAt` | 1 | Cannot exceed effective warning default; negative counts clamp to zero. |
| `autoDecrement` | `prompt` | `off`, `prompt`, `automatic`; invalid values rejected on save. |
| `replacementJumpPct` | 30 | Valid range 10–95; outside range resets to default. |
| `forecastMinDays` | 3 | Minimum history span; values below 1 reset to default. |
| `reorderLeadDays` | 14 | Reorder-soon horizon; negative resets to default. |
| `autoSuggestFromScans` | true | Suggest stock-item definitions for unmatched installed supplies. |

Forecasts use up to the previous 30 days of `supply.percent` telemetry, at least four samples, the configured minimum span and actual positive consumption. Large rises are replacement boundaries; small rises are ignored as measurement noise. Waste is inverted to remaining capacity. Unknown rate/day-to-empty is returned as unknown/null with a reason, not an invented prediction. Labels use slot plus description, so a description change can reduce usable history.

The item projection combines known per-device replacement times, then subsequent intervals of `100/rate`. It simulates up to three years; stockDays is the time of the onHand-th expected replacement (0 when empty). `reorderSoon` is true when known stockDays is at/below lead time. An item can be known from only some linked supplies; missing-history devices are excluded rather than estimated. Treat that as a partial forecast.

## CSV interchange

`GET /api/consumables.csv` exports these header keys:

```csv
id,name,type,color,partNumber,compatibleModels,unit,onHand,warningAt,criticalAt,location,notes,reorderUrl,unitCost
```

Import is a raw CSV POST to `/api/consumables/import`, `Content-Type:text/csv`, not multipart. At least a name header and one item row are needed. Existing item matching and stock changes occur row by row; response is `{created,updated,errors}`. `compatibleModels` uses semicolon-separated entries in CSV. The import body is capped at 4 MiB. Matching existing items tries id, then part number, then name. Export a sample first to preserve column spellings. The endpoint is operator-level and records import stock movements. Always inspect per-row errors before treating a batch as complete.

## Known discrepancies

The consumables forecast is distinct from `/api/reports/supplies`, whose fixed replacement threshold is 20 points and history minimum is half a day. The confirmation fix conditionally claims the event and attempts rollback on adjustment failure; it is not a general transactional exactly-once workflow surviving every crash boundary. Matching is deliberately conservative for model/type fallbacks, but multiple part-number matches can still choose a first candidate.

Sources: `internal/model/model.go`, `internal/consumables/{service,match,forecast}.go`, `internal/api/consumables.go`, `internal/store/{sqlite,postgres}_consumables.go`, `docs/consumables-feature-spec.md`.

## Exact item type values

`type` accepts `toner`, `ink`, `drum`, `fuser`, `maintenance_kit`, `waste`, `staples` or `other` (`internal/model/model.go`). These are JSON values; the maintenance kit uses an underscore.
