# Intent: a dashboard each admin can shape, and one that shows security posture

Author: the maintainer (product owner), from a first look at the dashboard on a built container, 31 October 2026. Status: draft. Issue: none yet (open one when accepted).

## Problem
The dashboard shows a fixed set of widgets: a row of six fleet totals (devices, online and offline, critical and warning alerts, supplies low or empty, consumables low on stock, jobs running), a fleet-health strip by device state, recent alerts and activity, and more below. They look good on an empty instance. Once a fleet is in, every widget fills with information from every printer and the page becomes busy and overwhelming; an administrator has no way to decide which widgets to show, which to hide, or where each one sits. The 30 October design audit found the same page from the other side: six number cards of identical weight, nothing on security posture, and raw event names leaking into the activity feed.

## Proposed outcome
An administrator can shape the dashboard: hide and show each widget, collapse a widget to its title, and move widgets into the order they want, from an edit mode on the page itself. The choice persists, as a personal layout for the signed-in user and as an instance default an administrator can set for everyone who has not shaped their own. A new security-posture widget joins the catalogue, showing the fleet's compliance score, counts of compliant, non-compliant and not-assessed devices, and the number of items reported as unknown, with the same "unknown is never pass" rule as the Security page. Alerts and security posture carry more visual weight than routine totals in the default layout.

## Affected users and systems
Viewers, operators and administrators (anyone who sees the dashboard); `web/src/pages/Dashboard.tsx` and the widget components it renders; `GET /api/dashboard` (`internal/api/server.go`) and the consumables and compliance summaries it draws on (`GET /api/compliance`); a new per-user layout record and an instance default in the settings store; the API reference and the wiki's monitoring page.

## Constraints
- Layouts are presentation only: hiding a widget never changes what a principal may read, and a group-scoped principal's widgets stay filtered as the lists are today.
- Every widget keeps working with the layout feature absent (no layout record means today's page); removing or renaming a widget in a later release must not break a stored layout.
- No new dependencies for the first version; drag-and-drop and resizing are later work and must not be designed out.
- Mobile keeps a single column and the same order.
- Read-only: nothing here touches a device.

## Open questions
- Does a personal layout belong to the user record or to a per-user settings key, and what happens to it when the user is deleted?
- Should the instance default be editable by operators as well as administrators?
- Which widgets should the default layout hide for a fresh instance?
