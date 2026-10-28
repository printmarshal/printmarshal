# PrintMarshal — build spec add-on: Consumables inventory & supply management

> Read alongside the main build spec (`printmarshal-build-spec.md`). This adds the
> consumables feature; it extends the main spec's §8.2 (inventory), §8.3 (monitoring),
> §9 (data model), §10 (API), and the roadmap. The companion UI brief is
> `ui-design-prompt-consumables.md`.

## 1. Scope

Two related capabilities:

- **A. Fleet supply-level monitoring.** Installed supply levels per device
  (toner / ink / drum / fuser / waste) are already read via the `read_supplies`
  capability and shown per device. This adds a **fleet-wide** view and ties those
  levels to stock.
- **B. Consumables stock inventory (the new part).** Track the **spare consumable
  stock** an operator keeps, linked to the devices/models that use each item, with
  stock levels, thresholds, low-stock alerts, consumption forecasting, and an optional
  reorder reference.

This is **operations stock management, not cost accounting.** Cost/reorder fields are
optional references only — no pricing, billing, quota, or chargeback logic (consistent
with the main spec's non-goals).

## 2. Concepts and entities

- **Consumable (stock item):** a type of consumable kept in stock. Fields: id, name,
  type (toner / ink / drum / fuser / maintenance-kit / waste / staples / other),
  color (nullable), part number / SKU, compatible device models/families, unit,
  **on-hand quantity**, **warning & critical stock thresholds**, storeroom/location,
  notes, optional reorder reference/URL, optional unit-cost (reference only).
  **Operator-owned — never overwritten by a scan.**
- **Installed supply (per device):** the consumable currently in a device slot, with
  its level / capacity / description from `read_supplies` telemetry (already captured).
  Each installed supply is **linked to a Consumable stock item** when one matches.
- **Stock event (history / audit):** every stock change — manual set/add/remove, or an
  auto-decrement on replacement — with actor, timestamp, reason, and resulting
  quantity. Drives stock history and, with telemetry, the forecast.

## 3. Behaviours

- **Match installed supply → stock item:** resolve primarily by **part number**; fall
  back to (color + type + compatible model). Surface **unmatched** installed supplies
  so an operator can link them or create a stock item (optionally auto-suggest new
  items from discovered supplies).
- **Stock alerts:** raise warning / critical when on-hand ≤ the item's thresholds,
  through the existing alert model (source = "consumable") and the notification
  pipeline.
- **Consumption & forecasting:** from the **rate supply levels drop** (per-device
  telemetry) across all devices linked to a consumable, estimate a consumption rate,
  then project (a) when **on-hand stock** will deplete and (b) when **each device** will
  need a replacement — surfaced as "days remaining" / "reorder soon." Require a minimum
  history window and degrade gracefully to "unknown" when data is thin.
- **Replacement detection + optional auto-decrement:** when a device's installed supply
  level **jumps up** (reset to ~full), infer a replacement; per a configurable setting
  (off / prompt / automatic), decrement the matching stock item's on-hand quantity.
  Always log it as a stock event.
- **Inline edit / batch save:** operators set on-hand quantities inline and commit in a
  batch; these values are never overwritten by scans.
- **Import / export:** CSV export and import of the consumables inventory.

## 4. Data model additions

- `consumable` — the stock item (fields in §2).
- `consumable_stock_event` — stock history + audit.
- `consumable_compat` — compatible device models/families per consumable (or an array
  on `consumable`).
- A link from each telemetry / installed-supply record to `consumable` (the resolved
  match), nullable until matched.

## 5. API additions

- `GET /consumables` (filter by type / color / stock status / model) · `POST /consumables`
- `GET /consumables/{id}` · `PATCH /consumables/{id}` · `DELETE /consumables/{id}`
- `POST /consumables/{id}/adjust-stock` — body `{set | delta, reason}` (audited)
- `GET /consumables/{id}/devices` — linked devices + their current installed level
- `GET /consumables/forecast` (and per item) — projected depletion / days remaining
- `GET /devices/{id}/supplies` — now includes, per installed supply, the linked
  consumable and its on-hand stock
- `POST /devices/{id}/supplies/{slot}/mark-replaced` — optional stock decrement
- bulk stock update; CSV export / import endpoints

All stock writes are audited; operator-entered values are protected from scan
overwrite.

## 6. UI

See the companion brief **`ui-design-prompt-consumables.md`**: a new **Consumables**
navigation section (inventory list with inline edit + batch save, consumable detail,
fleet supplies view, add/edit form), plus additions to Device detail (Supplies card
links to stock + "mark replaced"), Dashboard (consumables-low-on-stock tile +
reorder-forecast panel), Alerts (consumable stock alerts), and Settings (default
thresholds, auto-decrement toggle).

## 7. Roadmap placement

Build with monitoring and inventory — it sits on the `read_supplies` telemetry Phase 1
already produces:

- **Phase 1 (monitoring):** the fleet supply-level view (A), from existing telemetry.
- **Phase 2 (inventory):** the consumables stock inventory (B) — items, linking,
  thresholds/alerts, inline edit, CSV — then forecasting and optional auto-decrement.

## 8. Principles and constraints

- **Operator data is never overwritten by scans** (main spec principle 5): stock
  quantities, thresholds, and item fields are operator-owned.
- **Device / ops-centric, not accounting:** no cost / billing / chargeback; cost and
  reorder are optional reference fields only.
- **Brand-agnostic:** compatible-model references use neutral placeholders in any
  examples.
