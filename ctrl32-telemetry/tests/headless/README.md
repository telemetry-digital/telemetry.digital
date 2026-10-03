# Headless browser probes

Node 22+ scripts that drive a headless Chrome over the DevTools protocol against a running server (default
`http://127.0.0.1:8080`, local development credentials admin/admin1234). They open a page, act like a user and print
what they found; screenshots go to `OUT` (default: the current directory). Never run them against production.

`tests/run-all.sh` (and so CI) runs every probe here except two tools meant to be run by hand: `docshots.js` (the
pictures of the product documentation, against the full demo) and `camera_relay.js` (needs a real camera behind a
relay, `CAM=<camera id>`).

    BASE=http://127.0.0.1:8080 OUT=/tmp node tests/headless/incidents.js
    BASE=http://127.0.0.1:8080 LOGIN=1 SLUGS=wall-prehlad OUT=/tmp node tests/headless/dash.js
