# Trusted license keys

Each `*.pub` file holds one base64 Ed25519 public key (lines starting with `#` are comments). Licenses signed with the
matching private key are accepted by the binary. Create a key pair with `go run ./tools/licgen keygen -out <dir>`, keep
the private key offline (never in this repository) and commit only the `.pub` file here.

Until a key is committed, release builds accept no license and white labeling cannot be activated.
