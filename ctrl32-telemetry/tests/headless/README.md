# Headless browser probes

Node 22+ scripts that drive a headless Chrome over the DevTools protocol against a running server (default
`http://127.0.0.1:8080`, local development credentials admin/admin1234). They open a page, act like a user and print
what they found; screenshots go to `OUT` (default: the current directory). Never run them against production.

    BASE=http://127.0.0.1:8080 OUT=/tmp node tests/headless/incidents.js
    BASE=http://127.0.0.1:8080 LOGIN=1 SLUGS=wall-prehlad OUT=/tmp node tests/headless/dash.js
