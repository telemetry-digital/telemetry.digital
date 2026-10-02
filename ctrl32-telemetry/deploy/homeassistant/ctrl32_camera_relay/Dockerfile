# Home Assistant add-on: ctrl32 camera relay. The program is the ctrl32-telemetry release binary, downloaded from the
# server that published this add-on and checked against its SHA256SUMS.
FROM alpine:3.22
ARG BUILD_ARCH
ARG RELEASE_URL=https://portal.telemetry.digital/dl
RUN apk add --no-cache ca-certificates jq curl tzdata \
 && case "$BUILD_ARCH" in aarch64) A=arm64 ;; amd64) A=amd64 ;; armv7) A=arm ;; *) echo "unsupported $BUILD_ARCH"; exit 1 ;; esac \
 && curl -fsSLo /tmp/SHA256SUMS "$RELEASE_URL/SHA256SUMS" \
 && curl -fsSLo /usr/local/bin/ctrl32-telemetry "$RELEASE_URL/ctrl32-telemetry-linux-$A" \
 && want=$(grep " \*\{0,1\}ctrl32-telemetry-linux-$A\$" /tmp/SHA256SUMS | cut -d' ' -f1) \
 && got=$(sha256sum /usr/local/bin/ctrl32-telemetry | cut -d' ' -f1) \
 && [ -n "$want" ] && [ "$want" = "$got" ] \
 && chmod 0755 /usr/local/bin/ctrl32-telemetry && rm /tmp/SHA256SUMS \
 && /usr/local/bin/ctrl32-telemetry version
COPY run.sh /run.sh
RUN chmod 0755 /run.sh
ENTRYPOINT ["/run.sh"]
