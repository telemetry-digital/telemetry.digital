// Built-in template: audit trail extract. The hash column allows re-verification against the database.
#import "layout.typ": *
#let d = load()
#show: page-setup.with(d)

#title-block(d)
#text(size: 8.5pt, fill: luma(80))[Each row carries the SHA-256 chain hash of the audit entry. Verify with `ctrl32-telemetry audit verify` or `GET /api/v1/audit/verify`.]
#v(6pt)
#data-table(d)
#end-marker()
