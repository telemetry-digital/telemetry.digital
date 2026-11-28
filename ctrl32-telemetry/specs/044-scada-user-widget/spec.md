# 044 — Widget "Person at the controls" (0.98.0)

## Why

The owner asked for widgets of batches, trends and of the sign-in of the operators. The first two came with 0.97.0.
Signing in at the controls (SCADA users, spec 041) was offered only by the chip above the page — small on a touch
panel, and not where the author of a dashboard wants it.

## What

Widget **Person at the controls** (`scada_user`, group Control, no bindings):

- nobody signed in: a large button *Sign in at the controls* and, optionally, buttons with the names used on this
  computer — a name opens the sign-in dialog with the name filled in, the person types the PIN or the password;
- somebody signed in: the full name and the name, the groups (their names, not the keys), the time left before the
  sign-out without activity (counts down, activity extends it), a large button *Sign out at the controls*;
- it follows the chip in the header: a sign-in or sign-out anywhere on the page is seen at once (`ctrl32:scada`);
- settings: the groups, the time left and the names can each be hidden;
- no SCADA users in the organization: it says so (a link to *SCADA users* for an administrator);
- public links and displays: a note only — there is no session to sign in on top of.

Nothing changes on the server: the widget uses the session API of spec 041, and what a person may operate is still
decided by the server. The sign-in dialog accepts a name to fill in (`ctrl32.scadaSignIn({name})`).

## Tests

- Headless `tests/headless/scada_users.js` (en, sk; +9 checks): the offer, the names, the dialog with the name filled
  in, who signed in, the names of the groups, the countdown, the same person in the header, the sign-out.
- `TestEveryParamIsRendered`, `TestShowcaseMoreValid`.

## Not verified

A real touch panel; the widget inside a process picture and in the operator view (it is a plain widget, verified on a
dashboard).
