# Displays (kiosks)

A display is a screen at a wall — a PC, a mini-PC or a smart TV with a browser — that shows dashboards, process
screens (SCADA) and video wall screens in turn, without anyone signed in. It shows only what it is given, changes
nothing and can be disconnected at any time (spec 014).

## Pairing

1. On the computer at the wall open `https://<server>/display` in Chrome or Edge. It shows a code such as `K7QM-4TZP`.
2. In ctrl32 open **Displays → Pair display**, enter the code, name the display (for example *Control room – left
   monitor*) and choose what it shows.
3. The display starts showing its content within a few seconds.

The code is valid for 15 minutes and pairs once. Knowing the code is not enough to become the display: the kiosk keeps
a secret in an HttpOnly cookie, and only that browser collects the display's credential after the code was entered.
The credential is a random token in an HttpOnly, SameSite=Strict cookie; the server keeps only its hash. Pairing,
changes, commands and disconnecting are written to the audit trail (with the device's address and browser).

## What a display shows

- **Items in turn**: dashboards, process screens and screens of video walls, each for a number of seconds (10–3600).
  A single item stays on.
- **Enlarging cameras with an event**: *cameras of its video walls* or *all cameras* — a camera reporting motion,
  tampering, line crossing and so on fills the display for the chosen time (up to four at once).
- **Language** of the display.
- **Incident when the display stops showing** (default on): after the time set under *Cameras → Settings →
  Monitoring*.

A display reads only its own dashboards and walls, and plays only the cameras on them (camera widgets included — on
public links cameras are never shown), a camera an operator sent, or cameras with an active event when it enlarges all
cameras. Its cookie opens no other API.

## Production screens

A display is not only for cameras: any dashboard or process screen can be on it — for example a screen per line in
production with the pieces made in the shift (*Aggregated value*, sum), the plan fulfilment (*Progress bar* with the
plan as maximum), pieces per hour (*Bars over time*, sum per hour), the machine states (*Status*, *State timeline*),
speeds and temperatures (charts, gauges) and the process screen of the line. The time ranges *8 h*, *12 h* (a shift)
and *today* (since local midnight) exist for exactly this. The values refresh live.

Many displays: give each a **location** (hall, line) — the list can be filtered by it, and a message or command can
go to all displays of a location at once.

## Operator commands

On **Displays** (permission `display.control`, operators have it) or from a camera's page (*show on display*):

| Command | Effect |
|---|---|
| Send camera | the camera fills the display for the chosen time (only cameras the operator may see) |
| Identify | the display shows its name in large letters for a few seconds |
| Back to content | closes cameras sent or enlarged |
| Reload | the display reloads its page |
| Message | text on the display: *information* (band at the bottom), *warning* (band at the top) or *alarm* (the whole screen, flashing), for 1–60 minutes; to one display, the selected ones, all of a location, or all |

Pairing, changing and disconnecting displays needs `display.manage` (engineers and administrators).

## Setting up the computer

- Start the browser in kiosk mode when the computer starts, for example
  `chrome --kiosk --autoplay-policy=no-user-gesture-required https://<server>/display`
  (Windows: a shortcut in the Startup folder; Linux or Raspberry Pi: an autostart entry). The display stays paired over
  restarts and reconnects by itself after a network or server outage (it shows a red notice meanwhile).
- After a server update the display reloads its page by itself (the state carries `server_version`): 0–30 s later at
  random, at most once in 10 minutes. *Reload* reloads at once; a command issued before the page loaded is ignored.
  Signed-in wall pages check `/healthz` once a minute and reload the same way, but not while a camera is enlarged or a
  recording is open.
- The page keeps the screen awake (Wake Lock) and hides the cursor; the first click switches to full screen.
- Each display decodes its own video: for 20 camera tiles on the sub-stream a mini-PC with Intel graphics is enough.
  The server does not decode video — it only forwards the streams it already receives from the cameras.
- Several monitors on one computer: pair one display per monitor (open `/display` in a window on each monitor), or use
  the video wall's *Open on all monitors* with a signed-in user.

## Camera groups

A camera may belong to a group (*Camera management → Edit → Camera group*). A user can be limited to some groups
(*Users → user → Cameras*); they then see only those cameras — live view, recordings, walls, events, PTZ and marks,
also through the API. An operator limited to groups can send only their cameras to displays.
