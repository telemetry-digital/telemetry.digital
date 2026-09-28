# Phone, tablet and desktop apps (PWA)

The web interface installs as a progressive web app (PWA): *Add to Home Screen* on a phone or tablet, *Install app*
in Chrome or Edge on a computer. Nothing of the data is stored on the device — the service worker caches only the
static shell (`app.js`, `app.css`, icons, the main manifest); every page and API response comes from the server.

## The main app

`/manifest.webmanifest` installs the whole application (start page `/`, shortcuts Cameras, Video walls, Home). With
a licensed white label (`docs/whitelabel.md`) it carries the organization's product name and icon.

## Own apps (Settings → Apps, 0.60.0)

An organization can define more installable apps over the same web application, for example a phone app that opens
the live cameras and shows only the Cameras menu, a tablet at the gate that opens one video wall, or a smart-home app
with Home and Energy. Each app has:

| Setting | Meaning |
|---|---|
| Name | Shown on the install page and in the app window header (1–64 characters). |
| Name on the home screen | Label under the icon (at most 24 characters; up to 12 fit on most phones — longer names get a warning). |
| Address | `/app/{address}`, lowercase letters, digits and dashes. |
| Start page | What opens when the app starts: Dashboard, Dashboards or one dashboard, Cameras live view, Video walls or one wall, one camera, Home, Energy, Incidents, Displays, Devices, Flows, Assets, or another path of the application. Only paths of this server are accepted (no other site, no `//`, no `javascript:`, not `/api/v1/…` or `/app/…`). |
| Menu sections | Top-level menu groups the app shows: Dashboards, Home, Energy, Cameras, Displays, Flows, Devices, Incidents, Assets, Firmware, Users, Settings, System. *My account* (password, sign out) is always there. |
| Own colour | Title bar colour of the app; otherwise the organization's colour. |
| Enabled | A disabled app answers 404 on its install page and manifest; installed icons open a "not found" page. |

The create dialog has presets that only fill the form: **Cameras** (live view, Cameras), **Video wall** (the first
video wall, Cameras), **Dashboards** (Dashboards), **Smart home** (Home, Energy). Every change needs a reason, is
written to the audit trail (`pwa_app.create`, `pwa_app.update`, `pwa_app.retire`) and is stored as a new version
(content framework, ADR-0010). Managing apps needs `content.write`.

### Installing an app

Open `https://your-server/app/{address}` on the device (the list in Settings → Apps links it; on a computer the page
shows a QR code to scan with the phone).

- **Android (Chrome, Edge, Samsung Internet)** and **computers (Chrome, Edge)**: tap *Install*, or use the browser
  menu → *Install app* / *Add to Home screen*.
- **iPhone and iPad (Safari)**: tap *Share* (the square with the arrow) → *Add to Home Screen* → *Add*. The page sets
  the home-screen name and icon for iOS.

Several apps and the main app can be installed side by side (each manifest has its own `id`).

### What the app does

The icon starts `/app/{address}`; in the app window (standalone) the page switches the window into the app mode at
once and opens the start page. The mode stays for every page opened in that window: the menu (sidebar and phone
drawer) shows only the app's sections plus My account, the header shows the app's name, the logo leads to the start
page, sign-in returns to the start page and sign-out stays in the app. *My account → Open the full application*
leaves the mode. In a normal browser tab, *Open* on the install page starts the app mode in that tab.

### In the app window (0.61.0)

Three options of the app profile shape the window; the **Cameras** preset turns all of them on (a phone app for the
cameras only). Existing apps keep the previous behaviour.

| Option | Default | Effect |
|---|---|---|
| **Locked to its sections** (`locked`) | off | No way into the full application: no *Open the full application*, `?full=1` is ignored, the install page has no *Full application* button. A page outside the chosen sections (typed address, link, back button) opens the start page instead; sign-in, password reset, *Password and MFA* and the licence page stay reachable. My account keeps only *Password and MFA*; sign out stays at the bottom of the menu. |
| **Zoom the page with two fingers** (`page_zoom`) | on | Off: pinch and double tap do not enlarge the page (viewport `user-scalable=no`, `touch-action`, and on iPhone the gesture events are cancelled — Safari ignores `user-scalable`). The camera picture still zooms with two fingers inside the player; a long press on the picture opens no browser menu. |
| **Cameras in live view and on video walls** (`camera_layout`) | `auto` | `auto`: one camera per row on screens narrower than 700 px, the grid elsewhere; `single`: always one per row; `grid`: always the grid. The grid/list switch next to the live view and in the wall bar overrides it on that device (remembered in the browser). |

Locking limits the app's screens, not the user's rights — see the next paragraph.

**An app is a view, not a permission boundary.** The menu is filtered by the user's permissions first — an app never
shows what the user may not see — and the server checks permissions on every request as always. Someone who types
another address in the app window still gets only what the role allows. To limit what a person can see, use roles
and camera groups (`docs/video.md`).

### Names and icons (white label)

| | Without a white-label license | With an active white label |
|---|---|---|
| Home-screen label | the app's name on the home screen | the app's name on the home screen |
| Full name (install dialog, app list) | "{name} · ctrl32 telemetry" | "{name}" |
| Icon | ctrl32 icon | the organization's icon (PNG also as the iPhone icon) |

### Public endpoints

`/app/{address}`, `/app/{address}/manifest.webmanifest` and `GET /api/v1/pwa-apps/{address}/launch` work without
sign-in (a phone must be able to install the app first); they contain only the app's name, short name, start page
path, section keys and colour — no data of the organization. For an anonymous request (browsers fetch manifests
without cookies) the app is looked up in the oldest organization that has the address; for a signed-in user in the
user's own organization.

### API

`/api/v1/pwa-apps` — list, `options` (sections and start pages), create, get, update, delete (see `/api/docs`, tag
*Apps (PWA)*).

```bash
curl -X POST https://server/api/v1/pwa-apps -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"slug":"cameras","name":"Cameras","short_name":"Cameras","start_url":"/video","sections":["cameras"],"reason":"gate phones"}'
```
