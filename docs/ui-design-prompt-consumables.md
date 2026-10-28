# PrintMarshal — UI design prompt: Consumables (add-on)

Paste into a Claude design session. This is a **second, add-on** brief — it adds the
**Consumables** feature to the PrintMarshal UI from the main design brief. Keep the
existing visual system (the "Steel & Signal" palette, typography, components, and
light/dark themes) exactly as designed; design only what's described here, plus the
small edits it calls for on existing screens.

## Why this exists

PrintMarshal already monitors each device's installed supply **levels** (the per-device
Supplies card, the dashboard "supplies running low" panel). What's missing is a
**consumables stock inventory** — the spare toner / ink / drums / maintenance kits an
operations team keeps on the shelf, linked to the devices that use them, with stock
levels, thresholds, reorder-when-low signals, and consumption forecasting. Design that,
plus a fleet-wide view of installed supply levels.

This is **operations stock management, not cost accounting** — any cost or reorder
field is an optional reference only; there is no pricing, billing, or chargeback UI.

## Navigation

Add a **Consumables** item to the left sidebar (place it near Devices).

## Screens to design

1. **Consumables inventory (list)** — the primary screen.
   - A table of consumable **items**, each a stock-kept type. Columns: color swatch +
     name, type (toner / ink / drum / fuser / maintenance kit / waste / staples),
     part number / SKU, compatible models (chips), **on-hand quantity
     (inline-editable)**, stock-status pill (In stock / Low / Out), linked-device
     count, and a **forecast** cell ("~N days" / "Reorder soon" / "—" when unknown).
   - **Inline quantity editing with a batch-save bar**: let an operator edit several
     quantities, then save all at once, with clear unsaved-change affordances.
   - Filters: type, color, stock status, compatible model. Free-text search. CSV
     export. "Add consumable" action. Empty state (no items yet → add, or auto-suggest
     from discovered supplies).

2. **Consumable detail**
   - Header: name, color swatch, type, part number, stock-status pill, and an **Adjust
     stock** control (set / add / remove, with a reason).
   - Info card: compatible models (chips), warning & critical thresholds, storeroom /
     location, notes, optional reorder link.
   - **Linked devices** list: every device that uses this consumable, each with that
     device's **current installed level** of it (mini supply bar) — so demand reads at
     a glance.
   - **Stock history** chart (on-hand over time) and a **consumption / forecast** chart
     (projected depletion, with a "days remaining" estimate and a clear note when there
     isn't enough data yet).

3. **Fleet supplies view** (installed supply-level monitoring, fleet-wide)
   - All **installed** consumables across every device in one place — complementing the
     per-device Supplies card from the main design. Group / filter by type, color, and
     level; highlight "devices low / empty on X." A grouped list or a device × supply
     matrix.
   - Each row links to its stock item (showing on-hand count), so a low level connects
     to the stock available to fix it.

4. **Add / edit consumable** (form or side panel)
   - Fields: name, type, color, part number / SKU, compatible-model picker, unit,
     starting on-hand quantity, warning & critical thresholds, storeroom / location,
     notes, optional reorder link. Operator-owned values, clearly marked as never
     overwritten by scans.

## Edits to existing screens (please update these too)

- **Device detail → Supplies card:** link each installed supply to its consumable stock
  item, showing on-hand count ("3 in stock"); add a **Mark replaced** action that
  offers to decrement that item's stock.
- **Dashboard:** add a **"Consumables low on stock"** tile and panel (distinct from the
  existing "supplies running low," which is about device *levels*), plus an **upcoming
  replacements / reorder forecast** panel.
- **Alerts:** show **consumable stock alerts** (low / out) in the alert stream, styled
  like other alerts, sourced as "consumable."
- **Settings:** default stock thresholds, and an **auto-decrement-on-replacement**
  toggle (off / prompt / automatic).

## New components (match the existing system)

- **Color-swatch chip** for a consumable's color (C/M/Y/K and named colors).
- **Stock-status pill** (In stock / Low / Out) — color + icon + label.
- **Inline-editable quantity cell** + a **batch-save action bar**.
- **Compatible-models chip group.**
- **Forecast / "days remaining" indicator** (with an unknown state).
- **Consumption sparkline** and an **adjust-stock stepper / control**.

## States

Populated, empty, low, out, and **forecast-unknown** (insufficient history). Operator-
entered quantities and item fields are visually marked and never overwritten by scans.

## Deliverables

High-fidelity mockups (light + dark) of: the Consumables inventory list (with inline
edit + batch save), Consumable detail, Fleet supplies view, and the Add/Edit form —
plus the updated Device-detail Supplies card and the new Dashboard consumables
tile/panels. The new components above, and a short rationale. Keep everything
consistent with the main PrintMarshal UI and the Steel & Signal palette.
