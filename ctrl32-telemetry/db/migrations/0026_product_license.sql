-- Paid features (spec 007): the product license installed on this server, issued by the licensor with tools/licgen and
-- verified by the binary against its built-in public keys. One row; the white-label settings themselves live in
-- organization.branding.white_label and apply only while the license is valid.
CREATE TABLE core.product_license (
  id           smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  token        text NOT NULL CHECK (length(token) <= 8192),
  installed_at timestamptz NOT NULL DEFAULT now(),
  installed_by text NOT NULL
);
GRANT SELECT, INSERT, UPDATE, DELETE ON core.product_license TO ctrl32_app;
