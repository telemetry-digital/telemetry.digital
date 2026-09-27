-- Online activation of the white-label license (0.59.0): the order number of the ctrl32 licence bought on ctrl32.com,
-- sent with this server's host name to the licence worker (lic.ctrl32.com/telemetry/activate), which returns a 30-day
-- license inside the refund window and the perpetual one afterwards. The server renews it daily while the installed
-- license expires within 20 days; a refusal (refunded, used for the editor, host limit) stops the renewal. One row.
CREATE TABLE core.license_activation (
  id           smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  order_ref    text NOT NULL CHECK (length(order_ref) BETWEEN 1 AND 128),
  host         text NOT NULL,
  kind         text CHECK (kind IN ('starter', 'permanent')),
  activated_at timestamptz NOT NULL DEFAULT now(),
  activated_by text NOT NULL,
  last_attempt timestamptz,
  last_ok      timestamptz,
  last_error   text,
  refused      boolean NOT NULL DEFAULT false
);
GRANT SELECT, INSERT, UPDATE, DELETE ON core.license_activation TO ctrl32_app;
