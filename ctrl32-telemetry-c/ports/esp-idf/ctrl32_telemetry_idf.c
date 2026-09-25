/* ctrl32 telemetry — ESP-IDF port: esp-mqtt + esp_http_client + esp_ota_ops + mbedtls sha256 + NVS marker. */
#include "ctrl32_telemetry_idf.h"

#include <string.h>
#include <time.h>

#include "esp_crt_bundle.h"
#include "esp_http_client.h"
#include "esp_log.h"
#include "esp_netif.h"
#include "esp_ota_ops.h"
#include "esp_sntp.h"
#include "esp_system.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "freertos/task.h"
#include "mbedtls/sha256.h"
#include "mqtt_client.h"
#include "nvs.h"

#define TAG "ctrl32"
#ifndef C32_IDF_MSG
#define C32_IDF_MSG 4096
#endif
#ifndef C32_IDF_BATCH
#define C32_IDF_BATCH 20
#endif

static c32_idf_config_t cfg_;
static esp_mqtt_client_handle_t mqtt_;
static bool connected_;
static c32_ring_t ring_;
static c32_sample_t *slots_;
static SemaphoreHandle_t lock_;
static char msg_[C32_IDF_MSG];
static char topic_[96];
static char shared_[2048];
static size_t shared_len_;
static bool console_enabled_;
static char console_session_[48];
static uint32_t seq_;
static int64_t last_heartbeat_;
static esp_timer_handle_t hb_timer_;

static void lock(void) { xSemaphoreTakeRecursive(lock_, portMAX_DELAY); }
static void unlock(void) { xSemaphoreGiveRecursive(lock_); }

int64_t c32_idf_now(void) { time_t t = time(NULL); return t > 1000000000 ? (int64_t)t : 0; }
uint32_t c32_idf_next_seq(void) { return ++seq_; }
bool c32_idf_connected(void) { return connected_; }
size_t c32_idf_buffered(void) { return c32_ring_count(&ring_); }
bool c32_idf_console_enabled(void) { return console_enabled_; }

static bool publish(const char *suffix, const char *json, bool retain) {
    if (!connected_) return false;
    c32_topic(topic_, sizeof topic_, cfg_.device_id, suffix);
    return esp_mqtt_client_publish(mqtt_, topic_, json, 0, 1, retain ? 1 : 0) >= 0;
}

static void flush_buffer(void) {
    size_t taken = 0;
    lock();
    while (connected_ && c32_ring_count(&ring_)) {
        if (c32_ring_encode_batch(&ring_, msg_, sizeof msg_, C32_IDF_BATCH, &taken) <= 0) { c32_ring_drop(&ring_, 1); continue; }
        if (!publish("telemetry", msg_, false)) break;
        c32_ring_drop(&ring_, taken);
    }
    unlock();
}

bool c32_idf_send(const c32_sample_t *sample) {
    bool sent = false;
    lock();
    if (connected_ && c32_ring_count(&ring_) == 0 && c32_telemetry_json(msg_, sizeof msg_, sample, 1) > 0) sent = publish("telemetry", msg_, false);
    if (!sent) {
        c32_sample_t copy = *sample;
        if (copy.ts == 0) { copy.ts = c32_idf_now(); copy.rel = 0; }
        c32_ring_push(&ring_, &copy);
    }
    unlock();
    return sent;
}

bool c32_idf_send_num(const char *key, double value) {
    c32_sample_t s; memset(&s, 0, sizeof s);
    s.ts = c32_idf_now(); s.seq = c32_idf_next_seq();
    if (c32_sample_add(&s, key, value) != 0) return false;
    return c32_idf_send(&s);
}

void c32_idf_send_status(void) {
    c32_status_t st = {0};
    st.online = 1; st.uptime_s = (uint32_t)(esp_timer_get_time() / 1000000); st.fw = cfg_.fw; st.buffered = (uint32_t)c32_ring_count(&ring_);
    lock();
    if (c32_status_json(msg_, sizeof msg_, &st) > 0) publish("status", msg_, false);
    unlock();
    last_heartbeat_ = esp_timer_get_time();
}

void c32_idf_console_out(const char *line) {
    if (!console_enabled_ || !console_session_[0]) return;
    lock();
    if (c32_console_out_json(msg_, sizeof msg_, console_session_, c32_idf_now(), line) > 0) publish("console/out", msg_, false);
    unlock();
}

void c32_idf_request_shared(void) { publish("attributes/request", "{}", false); }
bool c32_idf_shared_num(const char *key, double *out) { return c32_attr_num(shared_, shared_len_, key, out) == 1; }
bool c32_idf_shared_bool(const char *key, bool *out) { int v; if (c32_attr_bool(shared_, shared_len_, key, &v) != 1) return false; *out = v != 0; return true; }

static void fota_status(const char *fw_id, const char *state, int pct, const char *ver, const char *err) {
    lock();
    if (c32_fota_status_json(msg_, sizeof msg_, fw_id, state, pct, ver, err) > 0) publish("fota/status", msg_, false);
    unlock();
}

static void report_pending_fota(void) {
    nvs_handle_t h;
    if (nvs_open("ctrl32", NVS_READWRITE, &h) != ESP_OK) return;
    char id[C32_ID_LEN] = {0}, ver[C32_VER_LEN] = {0};
    size_t l1 = sizeof id, l2 = sizeof ver;
    if (nvs_get_str(h, "fota_id", id, &l1) == ESP_OK && id[0]) {
        nvs_get_str(h, "fota_ver", ver, &l2);
        if (strcmp(ver, cfg_.fw) == 0) { fota_status(id, "updated", 100, cfg_.fw, NULL); esp_ota_mark_app_valid_cancel_rollback(); }
        else fota_status(id, "rolled_back", -1, cfg_.fw, "running previous version");
        nvs_erase_key(h, "fota_id"); nvs_erase_key(h, "fota_ver"); nvs_commit(h);
    }
    nvs_close(h);
}

typedef struct { c32_fota_target_t t; } fota_job_t;

static void fota_task(void *arg) {
    fota_job_t *job = (fota_job_t *)arg;
    c32_fota_target_t *t = &job->t;
    fota_status(t->fw_id, "downloading", 0, NULL, NULL);
    esp_http_client_config_t hc = { .url = t->url, .timeout_ms = 30000, .buffer_size = 4096 };
    if (cfg_.ca_cert_pem) hc.cert_pem = cfg_.ca_cert_pem; else if (!cfg_.insecure) hc.crt_bundle_attach = esp_crt_bundle_attach;
    if (cfg_.insecure) hc.skip_cert_common_name_check = true;
    esp_http_client_handle_t c = esp_http_client_init(&hc);
    char auth[80]; snprintf(auth, sizeof auth, "Bearer %s", cfg_.token);
    esp_http_client_set_header(c, "Authorization", auth);
    const char *err = NULL;
    esp_ota_handle_t ota = 0;
    const esp_partition_t *part = esp_ota_get_next_update_partition(NULL);
    if (!part) err = "no ota partition";
    else if (esp_http_client_open(c, 0) != ESP_OK) err = "connect";
    else {
        int64_t total = esp_http_client_fetch_headers(c);
        int status = esp_http_client_get_status_code(c);
        if (status != 200) err = "http status";
        else if (total > 0 && (uint32_t)total != t->size) err = "size mismatch";
        else if (esp_ota_begin(part, t->size, &ota) != ESP_OK) err = "ota begin";
        else {
            mbedtls_sha256_context sha; mbedtls_sha256_init(&sha); mbedtls_sha256_starts(&sha, 0);
            static uint8_t buf[2048];
            uint32_t done = 0; int last_pct = 0;
            while (done < t->size) {
                int n = esp_http_client_read(c, (char *)buf, sizeof buf);
                if (n < 0) { err = "read"; break; }
                if (n == 0) { if (esp_http_client_is_complete_data_received(c)) break; continue; }
                if (esp_ota_write(ota, buf, n) != ESP_OK) { err = "flash write"; break; }
                mbedtls_sha256_update(&sha, buf, n);
                done += n;
                int pct = (int)((uint64_t)done * 100 / t->size);
                if (pct >= last_pct + 10) { last_pct = pct; fota_status(t->fw_id, "downloading", pct, NULL, NULL); }
            }
            unsigned char digest[32]; mbedtls_sha256_finish(&sha, digest); mbedtls_sha256_free(&sha);
            char hex[65]; for (int i = 0; i < 32; i++) sprintf(hex + i * 2, "%02x", digest[i]);
            fota_status(t->fw_id, "verifying", 100, NULL, NULL);
            if (!err && done != t->size) err = "incomplete";
            if (!err && !c32_sha256_hex_equal(hex, t->sha256)) err = "sha256 mismatch";
            if (err) esp_ota_abort(ota);
            else if (esp_ota_end(ota) != ESP_OK) err = "ota end";
            else if (esp_ota_set_boot_partition(part) != ESP_OK) err = "set boot";
        }
    }
    esp_http_client_cleanup(c);
    if (err) { fota_status(t->fw_id, "failed", -1, NULL, err); free(job); vTaskDelete(NULL); return; }
    nvs_handle_t h;
    if (nvs_open("ctrl32", NVS_READWRITE, &h) == ESP_OK) { nvs_set_str(h, "fota_id", t->fw_id); nvs_set_str(h, "fota_ver", t->ver); nvs_commit(h); nvs_close(h); }
    fota_status(t->fw_id, "applying", 100, NULL, NULL);
    vTaskDelay(pdMS_TO_TICKS(700));
    free(job);
    esp_restart();
}

static void handle_fota(const char *json, size_t len) {
    c32_fota_target_t t;
    if (c32_fota_target_parse(json, len, &t) <= 0) return;
    if (strcmp(t.hw, cfg_.hw) != 0) { fota_status(t.fw_id, "failed", -1, NULL, "hw mismatch"); return; }
    if (c32_version_cmp(t.ver, cfg_.fw) <= 0) { fota_status(t.fw_id, "updated", 100, cfg_.fw, NULL); return; }
    if (t.min_ver[0] && c32_version_cmp(cfg_.fw, t.min_ver) < 0) { fota_status(t.fw_id, "failed", -1, NULL, "min_ver"); return; }
    if (cfg_.on_fota && !cfg_.on_fota(&t, cfg_.user)) return;
    if (!cfg_.on_fota && !cfg_.auto_fota) return;
    fota_job_t *job = malloc(sizeof *job);
    if (!job) return;
    job->t = t;
    xTaskCreate(fota_task, "c32_fota", 8192, job, 5, NULL);
}

static void handle_command(const char *json, size_t len) {
    c32_cmd_t cmd;
    if (c32_cmd_parse(json, len, &cmd) != 0) return;
    if (cmd.exp && c32_idf_now() && c32_idf_now() > cmd.exp) return;
    char result[512] = {0}, err[96] = {0};
    bool ok = false;
    if (strcmp(cmd.method, "ping") == 0) { ok = true; strcpy(result, "{\"pong\":true}"); }
    else if (strcmp(cmd.method, "get_attrs") == 0) { ok = true; c32_idf_request_shared(); }
    else if (strcmp(cmd.method, "reboot") == 0) ok = true;
    else if (cfg_.on_command) ok = cfg_.on_command(cmd.method, cmd.args ? cmd.args : "{}", cmd.args ? cmd.args_len : 2, result, sizeof result, err, sizeof err, cfg_.user);
    else strcpy(err, "unsupported");
    lock();
    if (c32_ack_json(msg_, sizeof msg_, cmd.id, ok, result, err) > 0) publish("cmd/ack", msg_, false);
    unlock();
    if (ok && strcmp(cmd.method, "reboot") == 0) { vTaskDelay(pdMS_TO_TICKS(300)); esp_restart(); }
}

static void handle_message(const char *topic, size_t tlen, const char *data, size_t len) {
    char tbuf[128];
    if (tlen >= sizeof tbuf) return;
    memcpy(tbuf, topic, tlen); tbuf[tlen] = 0;
    const char *suffix = c32_topic_suffix(tbuf, cfg_.device_id);
    if (!suffix) return;
    if (strcmp(suffix, "cmd") == 0) handle_command(data, len);
    else if (strcmp(suffix, "attributes/shared") == 0 || strcmp(suffix, "attributes/response") == 0) {
        if (len < sizeof shared_) { memcpy(shared_, data, len); shared_[len] = 0; shared_len_ = len; }
        int en; if (c32_attr_bool(data, len, "console.enabled", &en)) console_enabled_ = en != 0;
        if (cfg_.on_attributes) cfg_.on_attributes(data, len, cfg_.user);
    } else if (strcmp(suffix, "console/in") == 0) {
        char line[520];
        if (console_enabled_ && c32_console_in_parse(data, len, console_session_, sizeof console_session_, line, sizeof line) == 0 && cfg_.on_console) cfg_.on_console(line, cfg_.user);
    } else if (strcmp(suffix, "fota/target") == 0) handle_fota(data, len);
}

static void on_connected(void) {
    connected_ = true;
    const char *subs[] = {"attributes/shared", "attributes/response", "cmd", "console/in", "fota/target"};
    for (size_t i = 0; i < sizeof subs / sizeof subs[0]; i++) { c32_topic(topic_, sizeof topic_, cfg_.device_id, subs[i]); esp_mqtt_client_subscribe(mqtt_, topic_, 1); }
    esp_netif_ip_info_t ip; char ips[20] = "";
    esp_netif_t *netif = esp_netif_get_default_netif();
    if (netif && esp_netif_get_ip_info(netif, &ip) == ESP_OK) snprintf(ips, sizeof ips, IPSTR, IP2STR(&ip.ip));
    lock();
    if (c32_attributes_json(msg_, sizeof msg_, cfg_.fw, cfg_.hw, cfg_.model, ips[0] ? ips : NULL) > 0) publish("attributes", msg_, false);
    unlock();
    c32_idf_send_status();
    c32_idf_request_shared();
    report_pending_fota();
    flush_buffer();
    ESP_LOGI(TAG, "connected as %s", cfg_.device_id);
}

static void mqtt_event(void *arg, esp_event_base_t base, int32_t id, void *data) {
    esp_mqtt_event_handle_t e = (esp_mqtt_event_handle_t)data;
    switch ((esp_mqtt_event_id_t)id) {
    case MQTT_EVENT_CONNECTED: on_connected(); break;
    case MQTT_EVENT_DISCONNECTED: connected_ = false; ESP_LOGW(TAG, "disconnected"); break;
    case MQTT_EVENT_DATA:
        if (e->current_data_offset == 0 && e->total_data_len == e->data_len) handle_message(e->topic, e->topic_len, e->data, e->data_len);
        else ESP_LOGW(TAG, "fragmented message dropped (%d bytes)", e->total_data_len);
        break;
    default: break;
    }
}

static void hb_cb(void *arg) { if (connected_) { c32_idf_send_status(); flush_buffer(); } }

esp_err_t c32_idf_start(const c32_idf_config_t *cfg) {
    if (!cfg || !cfg->uri || !cfg->device_id || !cfg->token) return ESP_ERR_INVALID_ARG;
    cfg_ = *cfg;
    if (!cfg_.fw) cfg_.fw = "0.0.0";
    if (!cfg_.hw) cfg_.hw = "esp32";
    if (!cfg_.heartbeat_s) cfg_.heartbeat_s = 300;
    if (!cfg_.buffer_slots) cfg_.buffer_slots = 64;
    if (!cfg->on_fota && !cfg->auto_fota) cfg_.auto_fota = true;   /* zero-initialised config = auto */
    lock_ = xSemaphoreCreateRecursiveMutex();
    slots_ = calloc(cfg_.buffer_slots, sizeof(c32_sample_t));
    c32_ring_init(&ring_, slots_, cfg_.buffer_slots);
    if (!esp_sntp_enabled()) { esp_sntp_setoperatingmode(SNTP_OPMODE_POLL); esp_sntp_setservername(0, "pool.ntp.org"); esp_sntp_init(); }
    esp_mqtt_client_config_t mc = {0};
    mc.broker.address.uri = cfg_.uri;
    mc.credentials.username = cfg_.device_id;
    mc.credentials.authentication.password = cfg_.token;
    mc.credentials.client_id = cfg_.device_id;
    mc.session.keepalive = 30;
    mc.buffer.size = C32_IDF_MSG;
    mc.network.reconnect_timeout_ms = 5000;
    if (cfg_.ca_cert_pem) mc.broker.verification.certificate = cfg_.ca_cert_pem;
    else if (!cfg_.insecure) mc.broker.verification.crt_bundle_attach = esp_crt_bundle_attach;
    if (cfg_.insecure) mc.broker.verification.skip_cert_common_name_check = true;
    mqtt_ = esp_mqtt_client_init(&mc);
    if (!mqtt_) return ESP_FAIL;
    esp_mqtt_client_register_event(mqtt_, ESP_EVENT_ANY_ID, mqtt_event, NULL);
    const esp_timer_create_args_t ta = { .callback = hb_cb, .name = "c32_hb" };
    esp_timer_create(&ta, &hb_timer_);
    esp_timer_start_periodic(hb_timer_, (uint64_t)cfg_.heartbeat_s * 1000000ULL);
    return esp_mqtt_client_start(mqtt_);
}

esp_err_t c32_idf_provision(const char *base_url, const char *claim, const char *hw, const char *fw, const char *ca_cert_pem,
                            char *device_id, size_t id_cap, char *token, size_t token_cap) {
    char url[160]; snprintf(url, sizeof url, "%s/api/v1/provision", base_url);
    esp_http_client_config_t hc = { .url = url, .timeout_ms = 15000, .method = HTTP_METHOD_POST };
    if (ca_cert_pem) hc.cert_pem = ca_cert_pem; else hc.crt_bundle_attach = esp_crt_bundle_attach;
    esp_http_client_handle_t c = esp_http_client_init(&hc);
    char body[200]; int bl = snprintf(body, sizeof body, "{\"claim\":\"%s\",\"hw\":\"%s\",\"fw\":\"%s\"}", claim, hw ? hw : "", fw ? fw : "");
    esp_http_client_set_header(c, "Content-Type", "application/json");
    esp_err_t err = esp_http_client_open(c, bl);
    if (err != ESP_OK) { esp_http_client_cleanup(c); return err; }
    esp_http_client_write(c, body, bl);
    esp_http_client_fetch_headers(c);
    char resp[512]; int n = esp_http_client_read_response(c, resp, sizeof resp - 1);
    int status = esp_http_client_get_status_code(c);
    esp_http_client_cleanup(c);
    if (n <= 0 || (status != 200 && status != 201)) return ESP_FAIL;
    resp[n] = 0;
    c32_jval_t v;
    if (c32_json_get(resp, n, "token", &v) != 1 || c32_json_str(&v, token, token_cap) <= 0) return ESP_FAIL;
    if (device_id && c32_json_get(resp, n, "device_id", &v) == 1) c32_json_str(&v, device_id, id_cap);
    return ESP_OK;
}
