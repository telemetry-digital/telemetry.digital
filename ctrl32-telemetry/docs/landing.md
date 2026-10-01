# Front page editor

The page visitors see before signing in (`/`) is edited graphically: *System → Front page* (`/landing/edit`,
permission `system.admin`). Specification: `specs/012-landing-editor`.

## Editing

- **Add a block** (left): hero, features, text, image, text + image, numbers, call to action, contact, divider. A new
  block is inserted after the selected one.
- **Blocks** (left): drag to reorder, ▲▼ move, ⧉ duplicate, ✕ delete.
- **Preview** (centre): click a block to select it; click a text to type directly in the page. Computer, tablet and
  phone widths show how the page adapts.
- **Properties** (right): texts, image (logo, uploaded images, *Upload image*), alignment, columns, spacing, colours
  with an optional gradient, feature icons, numbers, buttons with links, contact details. Without a selected block:
  page settings — browser title, description for search engines, background, text and accent colours, width.
- Text: a blank line starts a paragraph, lines starting with "- " form a list, `**bold**`.
- Undo/redo (Ctrl+Z, Ctrl+Shift+Z), a warning before leaving with unsaved changes.

## Versions and publishing

*Save draft* stores a new version (with a reason in the audit trail) without changing what visitors see. *Publish*
stores and publishes at once. *Versions* lists every saved version: open one to continue from it, publish an older one,
or *Show the built-in page again*. Visitors always get the published version. Without a published page (0.63.2) the
address `/` leads visitors straight to the sign-in page, which shows a short overview of the system next to the form
(devices and data, dashboards and SCADA, video NVR, energy and reports, automation and AI); the editor's first draft
starts from the built-in blocks, so nothing changes for visitors until you publish.

## Safety

The page is public, so it is data, not HTML:

- the server accepts only the known block types and fields (unknown fields are refused), plain text with length
  limits, `#rrggbb` colours, icons from the catalogue and links to a page on this server, `https://`, `mailto:` or
  `tel:` — never `javascript:`, `data:`, plain `http:` or other hosts via `//`;
- the browser inserts every text as text (`<script>` typed into a text is shown as those characters);
- images are the server's own: PNG, JPEG, WebP (checked by content, not by name) and SVG (sanitised: scripts, event
  handlers, foreign references removed) up to 2 MiB, served with a sandbox policy and without type sniffing;
- the page cannot remove the sign-in button in the header, so no edit can lock users out;
- every save, publication, withdrawal and image upload is audited (`landing.*`).
