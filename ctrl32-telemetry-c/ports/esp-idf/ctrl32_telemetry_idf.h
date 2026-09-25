/*
 * ctrl32 telemetry — ESP-IDF port (protocol v0.1). Component: add this directory as an extra component and
 * `REQUIRES ctrl32_telemetry` (mqtt, esp_http_client, app_update, mbedtls, nvs_flash, esp_wifi are pulled in).
 *
 *   c32_idf_config_t cfg = { .uri = "mqtts://telemetry.digital:8883", .device_id = "fridge-01", .token = "...", .fw = "1.0.0", .hw = "esp32-s3" };
 *   c32_idf_start(&cfg);                 // connects, subscribes, replays the offline buffer, reports status
 *   c32_idf_send_num("temp", 4.5);       // buffered while offline
 */
#ifndef CTRL32_TELEMETRY_IDF_H
#define CTRL32_TELEMETRY_IDF_H

#include "esp_err.h"
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "ctrl32_telemetry.h"

#ifdef __cplusplus
extern "C" {
#endif

/* Command handler: return true and fill result (JSON, may stay empty) or false and fill err. */
typedef bool (*c32_idf_cmd_cb_t)(const char *method, const char *args, size_t args_len, char *result, size_t result_cap, char *err, size_t err_cap, void *user);
typedef void (*c32_idf_attrs_cb_t)(const char *json, size_t len, void *user);
typedef void (*c32_idf_console_cb_t)(const char *line, void *user);
typedef bool (*c32_idf_fota_cb_t)(const c32_fota_target_t *target, void *user);   /* return false to refuse */

typedef struct {
    const char *uri;              /* mqtts://host:8883 or mqtt://host:1883 (intranet only) */
    const char *device_id;        /* external id = MQTT username */
    const char *token;            /* device token = MQTT password */
    const char *ca_cert_pem;      /* NULL = use the ESP x509 certificate bundle (CONFIG_MBEDTLS_CERTIFICATE_BUNDLE) */
    bool insecure;                /* skip server verification (development only) */
    const char *fw;
    const char *hw;
    const char *model;
    uint16_t heartbeat_s;         /* default 300 */
    size_t buffer_slots;          /* default 64 */
    bool auto_fota;               /* default true */
    c32_idf_cmd_cb_t on_command;
    c32_idf_attrs_cb_t on_attributes;
    c32_idf_console_cb_t on_console;
    c32_idf_fota_cb_t on_fota;
    void *user;
} c32_idf_config_t;

esp_err_t c32_idf_start(const c32_idf_config_t *cfg);   /* requires an initialised network interface and NVS */
bool c32_idf_connected(void);
bool c32_idf_send(const c32_sample_t *sample);           /* publishes now or buffers */
bool c32_idf_send_num(const char *key, double value);
void c32_idf_send_status(void);
void c32_idf_console_out(const char *line);
bool c32_idf_console_enabled(void);
size_t c32_idf_buffered(void);
int64_t c32_idf_now(void);                               /* unix seconds if SNTP synchronised, else 0 */
uint32_t c32_idf_next_seq(void);
bool c32_idf_shared_num(const char *key, double *out);
bool c32_idf_shared_bool(const char *key, bool *out);
void c32_idf_request_shared(void);

/* Claim-code provisioning over HTTPS: POST {base_url}/api/v1/provision {"claim","hw","fw"} -> token (and device id). */
esp_err_t c32_idf_provision(const char *base_url, const char *claim, const char *hw, const char *fw, const char *ca_cert_pem,
                            char *device_id, size_t id_cap, char *token, size_t token_cap);

#ifdef __cplusplus
}
#endif
#endif
