# ADR-0011: UI — responsive layout, PWA, light/dark theme, organization branding, fixed product footer

**Status:** accepted 23 Sep 2026 (user: "mobile and tablet definitely yes, with a fixed width so nothing overflows, and PWA"; "light and dark theme, theme settings so colours and font can be changed"; "the only information in the footer is ctrl32 - telemetry, a GitHub icon opening a new window, a link to GitHub and a link to the license; the footer is fixed and cannot be changed"; "when nobody is signed in, a simple landing page")

## Decision

1. **Responsive, fixed-width content.** Every page uses a centered container (max 1200 px, 16 px gutters) and `overflow-x: hidden` on the page. Nothing may widen the page: tables scroll inside their own `.table-wrap`, images and charts are capped at 100 %. Phones and tablets are first-class targets (viewport with `viewport-fit=cover`, touch-sized controls).
2. **PWA.** The app ships a web manifest, SVG + PNG icons (192/512, maskable) and a service worker that caches only the static shell (CSS, JS, logo, icons) with stale-while-revalidate. API responses, pages with data, exports and reports are never cached in the browser: records always come from the server.
3. **Theme.** Light and dark palettes are CSS custom properties. The mode follows the system by default; the organization may set a default (`auto|light|dark`) and every user may override it for their browser (stored locally, not on the server). The organization theme (primary colour, font family, corner radius, font scale) is served by `GET /api/v1/branding` and applied on every page and in the PDF header.
4. **Branding is content.** Logo and theme live in `organization.branding` (JSONB), are edited through `PUT /api/v1/branding` with permission `content.write`, validated (mime, size ≤ 256 KiB, hex colour, plain font name, SVG without scripts) and audited with a reason. The logo is served with a restrictive CSP so an SVG can never execute scripts. Without a custom logo the built-in product logo is shown.
5. **Fixed product footer.** Every page renders the same footer from the binary: "ctrl32 · telemetry", a GitHub icon linking to the source repository in a new window (`rel="noopener noreferrer"`), a link to `/license`, and the version. It is not configurable through any setting or template, and keeping it is a condition of the license (ADR-0002).
6. **Landing page.** Unauthenticated visitors see a simple landing page (logo, one paragraph, sign-in button, feature cards) and never data, even when `public_read` is enabled for the API.
7. **Icons.** Only Tabler Icons paths (MIT), inlined, per ADR-0009. No external fonts or CDNs: the CSP allows `self` only.

## Consequences

- The PoC pages (`/`, `/login`, `/settings`, `/license`) already follow this ADR; the Lit + Tabler UI planned for phase 1 must keep the same tokens, footer component and PWA files.
- A custom font applies only if it is installed on the client or later self-hosted; hosting fonts on the server is a follow-up (font files as content items).
- Per-user theme choice is intentionally not stored server-side (no personal data needed); per-organization defaults are.
