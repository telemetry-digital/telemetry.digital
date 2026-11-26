# ctrl32 telemetry — device library (C)

Device-side implementation of the ctrl32 telemetry protocol v0.1: telemetry with an offline buffer and backfill,
status heartbeat, client and shared attributes, commands with acknowledgement, remote console and FOTA with
SHA-256 verification. Free of charge, forever (see LICENSE).

> **Firmware signatures are not verified yet.** The library compares the SHA-256 of a downloaded image with the campaign
> and does not check the Ed25519 signature the protocol describes (`sig`, `kid`). Use over-the-air updates only with a
> server you trust and control until signature verification is added.

| Part | What it is | Dependencies |
|---|---|---|
| `src/` | portable C99 core: JSON building/parsing, topics, ring buffer. No allocation, no libc beyond `<stdio.h>`/`<string.h>`. | none |
| `ports/arduino/` | ESP32 Arduino class `Ctrl32Telemetry` | ESP32 core, PubSubClient (MIT) |
| `ports/esp-idf/` | ESP-IDF component `c32_idf_*` | esp-mqtt, esp_http_client, app_update, mbedtls, nvs |
| `examples/` | fridge node for both ports | |
| `tests/core_test.c` | host tests of the core (`tcc -Isrc tests/core_test.c src/ctrl32_telemetry.c -run`) | any C99 compiler |

STM32 (FreeRTOS + LwIP, and modem/AT) ports follow; they reuse the same core.

## Quick start (Arduino, ESP32)

1. On the server: Devices → New device (external id, e.g. `fridge-01`) → Issue a token. Assets → create the
   datastreams the device publishes (`temp`, `rh`) and assign them to the device (device page → Datastreams).
2. Install the library (copy this repository into `Arduino/libraries/ctrl32-telemetry`, or use PlatformIO
   `lib_deps = https://github.com/telemetry-digital/ctrl32-telemetry-c`) and PubSubClient.
3. Open `examples/arduino/fridge_node`, fill in Wi-Fi, server host, device id and token, flash.

```cpp
Ctrl32Telemetry tel;
Ctrl32Config c; c.host = "telemetry.digital"; c.deviceId = "fridge-01"; c.token = "…"; c.fw = "1.0.0"; c.hw = "esp32-s3";
tel.begin(c);
// loop():
tel.loop();
tel.send("temp", 4.5);           // buffered while offline, replayed as backfill (bf=1) after reconnect
```

Commands, shared attributes and the console arrive through callbacks (`onCommand`, `onSharedAttributes`,
`onConsoleLine`); the reserved methods `ping`, `get_attrs` and `reboot` are handled by the library. A FOTA
target is applied automatically when the hardware id matches and the version is newer: the image is streamed
into the OTA partition, its SHA-256 is compared with the campaign, the device reboots and reports `updated`
(or `rolled_back` when the new version does not come up).

Provisioning without a pre-shared token: `Ctrl32Telemetry::provision(baseUrl, claimCode, hw, fw, deviceId, token)`
exchanges a claim code from the Devices page for a token; store the token in NVS.

## Quick start (ESP-IDF)

Add `ports` as an extra component directory (see `examples/esp-idf/fridge_node/CMakeLists.txt`), then:

```c
c32_idf_config_t cfg = { .uri = "mqtts://telemetry.digital:8883", .device_id = "fridge-02", .token = "…", .fw = "1.0.0", .hw = "esp32-s3", .on_command = on_command };
c32_idf_start(&cfg);
c32_idf_send_num("temp", 4.5);
```

TLS uses the ESP-IDF certificate bundle by default (`CONFIG_MBEDTLS_CERTIFICATE_BUNDLE`); pass `ca_cert_pem` for a
private CA. FOTA needs an OTA partition table (`partitions_two_ota.csv`) and, for rollback, `CONFIG_BOOTLOADER_APP_ROLLBACK_ENABLE`.

## Protocol summary

Topics under `d/{deviceId}/`: `telemetry`, `attributes`, `status`, `cmd/ack`, `console/out`, `fota/status`,
`attributes/request` (device → server); `attributes/shared` (retained), `attributes/response`, `cmd`, `console/in`,
`fota/target` (server → device). Payloads are JSON; see `src/ctrl32_telemetry.h` and the server repository
`docs/protocol/device-protocol-v0.1.md`.

## Connecting devices without this library

Any MQTT or HTTP client can talk to a ctrl32 telemetry server; the integration reference with tested examples is
`docs/integration/device-integration.md` in the server repository. `examples/common/isrg_roots.h` holds the Let's
Encrypt root certificates (ISRG Root X1 and X2) as one PEM bundle for TLS on a public instance.

## Building the checks

```
tcc -Wall -Isrc tests/core_test.c src/ctrl32_telemetry.c -run     # host tests
pio run -d ci                                                    # compile the Arduino example for ESP32-S3
pio run -d ci/espidf                                             # compile the ESP-IDF example for ESP32-S3 (PlatformIO downloads ESP-IDF)
```
