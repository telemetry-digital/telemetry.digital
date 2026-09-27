# API contract

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | /api/v1/server/license | system.admin | installed, valid, problem, licensee, features, domains, expires (absent = perpetual), perpetual, trusted_keys, white_label |
| PUT | /api/v1/server/license | system.admin | `{token, reason}`; 400 bad signature / other server / expired |
| DELETE | /api/v1/server/license?reason= | system.admin | |
| GET | /api/v1/branding | public | `product`, `icon_url`, `footer` {text, links, show_version, white_label, intro, github, license, fixed}, `white_label.licensed` |
| PUT | /api/v1/branding | content.write | `white_label` {…, icon {mime,data}, clear_icon}; 402 when enabling without a license |
| GET | /api/v1/branding/white-label | content.write | stored settings, `licensed`, `active` |
| GET | /api/v1/branding/icon | public | white-label icon or redirect to /favicon.svg |
| GET | /manifest.webmanifest | public | name/short_name/icon from the white label |
