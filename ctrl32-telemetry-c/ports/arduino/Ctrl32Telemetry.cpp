/* ctrl32 telemetry — Arduino ESP32 port. */
#include "Ctrl32Telemetry.h"

#include <HTTPClient.h>
#include <Preferences.h>
#include <PubSubClient.h>
#include <Update.h>
#include <WiFi.h>
#include <WiFiClientSecure.h>
#include <mbedtls/sha256.h>
#include <time.h>

#ifndef C32_MQTT_BUFFER
#define C32_MQTT_BUFFER 4096
#endif
#ifndef C32_BATCH
#define C32_BATCH 20
#endif

struct Ctrl32Telemetry::Impl {
    WiFiClient plain;
    WiFiClientSecure secure;
    PubSubClient mqtt;
    Preferences prefs;
    char topic[96];
    char msg[C32_MQTT_BUFFER];
    Impl() : mqtt() {}
};

static Ctrl32Telemetry *g_instance = nullptr;

Ctrl32Telemetry::Ctrl32Telemetry() : impl_(new Impl()) { memset(&ring_, 0, sizeof ring_); g_instance = this; }
Ctrl32Telemetry::~Ctrl32Telemetry() { delete[] slots_; delete impl_; }

bool Ctrl32Telemetry::begin(const Ctrl32Config &cfg) {
    cfg_ = cfg;
    if (!cfg_.host || !cfg_.deviceId || !cfg_.token) return false;
    slots_ = new c32_sample_t[cfg_.bufferSlots];
    c32_ring_init(&ring_, slots_, cfg_.bufferSlots);
    if (cfg_.tls) {
        if (cfg_.insecure) impl_->secure.setInsecure();
        else if (cfg_.caCert) impl_->secure.setCACert(cfg_.caCert);
        impl_->mqtt.setClient(impl_->secure);
    } else {
        impl_->mqtt.setClient(impl_->plain);
    }
    impl_->mqtt.setServer(cfg_.host, cfg_.port);
    impl_->mqtt.setBufferSize(C32_MQTT_BUFFER);
    impl_->mqtt.setKeepAlive(cfg_.keepAliveS);
    impl_->mqtt.setCallback([](char *topic, uint8_t *payload, unsigned len) { if (g_instance) g_instance->handleMessage(topic, payload, len); });
    if (cfg_.syncTime) configTime(0, 0, cfg_.ntp);
    impl_->prefs.begin("ctrl32", false);
    return true;
}

int64_t Ctrl32Telemetry::now() const {
    time_t t = time(nullptr);
    return t > 1000000000 ? (int64_t)t : 0;
}

bool Ctrl32Telemetry::connected() const { return impl_->mqtt.connected(); }

bool Ctrl32Telemetry::connect() {
    if (WiFi.status() != WL_CONNECTED) return false;
    String clientId = String(cfg_.deviceId) + "-" + String((uint32_t)ESP.getEfuseMac(), HEX);
    if (!impl_->mqtt.connect(clientId.c_str(), cfg_.deviceId, cfg_.token)) return false;
    onConnected();
    return true;
}

void Ctrl32Telemetry::onConnected() {
    const char *subs[] = {"attributes/shared", "attributes/response", "cmd", "console/in", "fota/target"};
    for (const char *s : subs) { c32_topic(impl_->topic, sizeof impl_->topic, cfg_.deviceId, s); impl_->mqtt.subscribe(impl_->topic, 1); }
    c32_attributes_json(impl_->msg, sizeof impl_->msg, cfg_.fw, cfg_.hw, cfg_.model, WiFi.localIP().toString().c_str());
    publish("attributes", impl_->msg);
    sendStatus();
    requestSharedAttributes();
    reportPendingFota();
    flushBuffer();
    backoffMs_ = 1000;
}

void Ctrl32Telemetry::loop() {
    if (!impl_->mqtt.connected()) {
        unsigned long nowMs = millis();
        if (nowMs - lastAttempt_ >= backoffMs_) {
            lastAttempt_ = nowMs;
            if (!connect()) backoffMs_ = min<unsigned long>(backoffMs_ * 2, 300000UL);   // 1 s -> 5 min
        }
        return;
    }
    impl_->mqtt.loop();
    if (cfg_.heartbeatS && millis() - lastHeartbeat_ >= (unsigned long)cfg_.heartbeatS * 1000UL) sendStatus();
    if (c32_ring_count(&ring_)) flushBuffer();
}

bool Ctrl32Telemetry::publish(const char *suffix, const char *json, bool retain) {
    if (!impl_->mqtt.connected()) return false;
    c32_topic(impl_->topic, sizeof impl_->topic, cfg_.deviceId, suffix);
    return impl_->mqtt.publish(impl_->topic, json, retain);
}

bool Ctrl32Telemetry::send(const c32_sample_t &sample) {
    if (impl_->mqtt.connected() && c32_ring_count(&ring_) == 0) {
        if (c32_telemetry_json(impl_->msg, sizeof impl_->msg, &sample, 1) > 0 && publish("telemetry", impl_->msg)) return true;
    }
    c32_sample_t copy = sample;
    if (copy.ts == 0) { copy.ts = now(); copy.rel = 0; }     // buffered samples need an absolute time
    c32_ring_push(&ring_, &copy);
    return false;
}

bool Ctrl32Telemetry::send(const char *key, double value) {
    c32_sample_t s; memset(&s, 0, sizeof s);
    s.ts = now(); s.seq = nextSeq();
    if (c32_sample_add(&s, key, value) != 0) return false;
    return send(s);
}

bool Ctrl32Telemetry::sendBool(const char *key, bool value) {
    c32_sample_t s; memset(&s, 0, sizeof s);
    s.ts = now(); s.seq = nextSeq();
    if (c32_sample_add_bool(&s, key, value) != 0) return false;
    return send(s);
}

bool Ctrl32Telemetry::sendBatch(const c32_sample_t *samples, size_t n) {
    if (impl_->mqtt.connected() && c32_telemetry_json(impl_->msg, sizeof impl_->msg, samples, n) > 0) return publish("telemetry", impl_->msg);
    for (size_t i = 0; i < n; i++) send(samples[i]);
    return false;
}

void Ctrl32Telemetry::flushBuffer() {
    size_t taken = 0;
    while (impl_->mqtt.connected() && c32_ring_count(&ring_)) {
        if (c32_ring_encode_batch(&ring_, impl_->msg, sizeof impl_->msg, C32_BATCH, &taken) <= 0) { c32_ring_drop(&ring_, 1); continue; }
        if (!publish("telemetry", impl_->msg)) return;
        c32_ring_drop(&ring_, taken);
        impl_->mqtt.loop();
    }
}

void Ctrl32Telemetry::sendStatus() {
    c32_status_t st = {};
    st.online = 1; st.rssi = WiFi.RSSI(); st.uptime_s = millis() / 1000; st.fw = cfg_.fw; st.buffered = (uint32_t)c32_ring_count(&ring_);
    if (c32_status_json(impl_->msg, sizeof impl_->msg, &st) > 0) publish("status", impl_->msg);
    lastHeartbeat_ = millis();
}

void Ctrl32Telemetry::consoleOut(const char *line) {
    if (!consoleEnabled_ || consoleSession_.isEmpty()) return;
    if (c32_console_out_json(impl_->msg, sizeof impl_->msg, consoleSession_.c_str(), now(), line) > 0) publish("console/out", impl_->msg);
}

void Ctrl32Telemetry::requestSharedAttributes() { publish("attributes/request", "{}"); }

double Ctrl32Telemetry::sharedNumber(const char *key, double fallback) const {
    double v; return c32_attr_num(shared_.c_str(), shared_.length(), key, &v) ? v : fallback;
}
bool Ctrl32Telemetry::sharedBool(const char *key, bool fallback) const {
    int v; return c32_attr_bool(shared_.c_str(), shared_.length(), key, &v) ? v != 0 : fallback;
}

void Ctrl32Telemetry::handleMessage(const char *topic, const uint8_t *payload, unsigned len) {
    const char *suffix = c32_topic_suffix(topic, cfg_.deviceId);
    if (!suffix) return;
    const char *json = (const char *)payload;
    if (strcmp(suffix, "cmd") == 0) handleCommand(json, len);
    else if (strcmp(suffix, "attributes/shared") == 0 || strcmp(suffix, "attributes/response") == 0) handleShared(json, len);
    else if (strcmp(suffix, "console/in") == 0) {
        char sess[48], line[520];
        if (consoleEnabled_ && c32_console_in_parse(json, len, sess, sizeof sess, line, sizeof line) == 0) {
            consoleSession_ = sess;
            if (onConsole_) onConsole_(line);
        }
    } else if (strcmp(suffix, "fota/target") == 0) handleFota(json, len);
}

void Ctrl32Telemetry::handleCommand(const char *json, size_t len) {
    c32_cmd_t cmd;
    if (c32_cmd_parse(json, len, &cmd) != 0) return;
    if (cmd.exp && now() && now() > cmd.exp) return;               // expired: no ack, the server marks it expired
    String result, err;
    bool ok = false;
    if (strcmp(cmd.method, "ping") == 0) { ok = true; result = "{\"pong\":true}"; }
    else if (strcmp(cmd.method, "get_attrs") == 0) { ok = true; requestSharedAttributes(); }
    else if (strcmp(cmd.method, "reboot") == 0) { ok = true; }
    else if (onCommand_) ok = onCommand_(cmd.method, cmd.args ? cmd.args : "{}", cmd.args ? cmd.args_len : 2, result, err);
    else err = "unsupported";
    if (c32_ack_json(impl_->msg, sizeof impl_->msg, cmd.id, ok, result.c_str(), err.c_str()) > 0) publish("cmd/ack", impl_->msg);
    if (ok && strcmp(cmd.method, "reboot") == 0) { impl_->mqtt.loop(); delay(200); ESP.restart(); }
}

void Ctrl32Telemetry::handleShared(const char *json, size_t len) {
    shared_ = String(json, len);
    int en = 0;
    if (c32_attr_bool(json, len, "console.enabled", &en)) consoleEnabled_ = en != 0;
    if (onAttrs_) onAttrs_(json, len);
}

void Ctrl32Telemetry::fotaStatus(const char *fwId, const char *state, int pct, const char *ver, const char *err) {
    if (c32_fota_status_json(impl_->msg, sizeof impl_->msg, fwId, state, pct, ver, err) > 0) publish("fota/status", impl_->msg);
    impl_->mqtt.loop();
}

void Ctrl32Telemetry::handleFota(const char *json, size_t len) {
    c32_fota_target_t t;
    int r = c32_fota_target_parse(json, len, &t);
    if (r <= 0) return;                                            // cleared or malformed
    if (strcmp(t.hw, cfg_.hw) != 0) { fotaStatus(t.fw_id, "failed", -1, nullptr, "hw mismatch"); return; }
    if (c32_version_cmp(t.ver, cfg_.fw) <= 0) { fotaStatus(t.fw_id, "updated", 100, cfg_.fw, nullptr); return; }   // already running it
    if (t.min_ver[0] && c32_version_cmp(cfg_.fw, t.min_ver) < 0) { fotaStatus(t.fw_id, "failed", -1, nullptr, "min_ver"); return; }
    if (onFota_ && !onFota_(t)) return;
    if (!onFota_ && !cfg_.autoFota) return;
    runFota(t);
}

void Ctrl32Telemetry::runFota(const c32_fota_target_t &t) {
    fotaStatus(t.fw_id, "downloading", 0, nullptr, nullptr);
    WiFiClientSecure secure; WiFiClient plain;
    bool https = strncmp(t.url, "https://", 8) == 0;
    if (https) { if (cfg_.insecure) secure.setInsecure(); else if (cfg_.caCert) secure.setCACert(cfg_.caCert); }
    HTTPClient http;
    http.setTimeout(30000);
    if (!(https ? http.begin(secure, t.url) : http.begin(plain, t.url))) { fotaStatus(t.fw_id, "failed", -1, nullptr, "url"); return; }
    http.addHeader("Authorization", String("Bearer ") + cfg_.token);
    int code = http.GET();
    if (code != 200) { http.end(); fotaStatus(t.fw_id, "failed", -1, nullptr, (String("http ") + code).c_str()); return; }
    size_t total = http.getSize() > 0 ? (size_t)http.getSize() : t.size;
    if (total != t.size) { http.end(); fotaStatus(t.fw_id, "failed", -1, nullptr, "size mismatch"); return; }
    if (!Update.begin(total)) { http.end(); fotaStatus(t.fw_id, "failed", -1, nullptr, "no space"); return; }
    mbedtls_sha256_context sha; mbedtls_sha256_init(&sha); mbedtls_sha256_starts(&sha, 0);
    WiFiClient *stream = http.getStreamPtr();
    uint8_t buf[1024];
    size_t done = 0; int lastPct = 0;
    unsigned long lastData = millis();
    while (http.connected() && done < total) {
        size_t avail = stream->available();
        if (!avail) { if (millis() - lastData > 30000) break; delay(1); continue; }
        size_t n = stream->readBytes(buf, min(avail, sizeof buf));
        if (!n) continue;
        lastData = millis();
        if (Update.write(buf, n) != n) break;
        mbedtls_sha256_update(&sha, buf, n);
        done += n;
        int pct = (int)(done * 100 / total);
        if (pct >= lastPct + 10) { lastPct = pct; fotaStatus(t.fw_id, "downloading", pct, nullptr, nullptr); }
    }
    http.end();
    unsigned char digest[32]; mbedtls_sha256_finish(&sha, digest); mbedtls_sha256_free(&sha);
    char hex[65]; for (int i = 0; i < 32; i++) sprintf(hex + i * 2, "%02x", digest[i]);
    fotaStatus(t.fw_id, "verifying", 100, nullptr, nullptr);
    if (done != total || !c32_sha256_hex_equal(hex, t.sha256)) { Update.abort(); fotaStatus(t.fw_id, "failed", -1, nullptr, done != total ? "incomplete" : "sha256 mismatch"); return; }
    if (!Update.end(true)) { fotaStatus(t.fw_id, "failed", -1, nullptr, "flash"); return; }
    impl_->prefs.putString("fota_id", t.fw_id);
    impl_->prefs.putString("fota_ver", t.ver);
    impl_->prefs.putString("fota_prev", cfg_.fw);
    fotaStatus(t.fw_id, "applying", 100, nullptr, nullptr);
    delay(500);
    ESP.restart();
}

void Ctrl32Telemetry::reportPendingFota() {
    String id = impl_->prefs.getString("fota_id", "");
    if (id.isEmpty()) return;
    String ver = impl_->prefs.getString("fota_ver", "");
    if (ver == cfg_.fw) fotaStatus(id.c_str(), "updated", 100, cfg_.fw, nullptr);
    else fotaStatus(id.c_str(), "rolled_back", -1, cfg_.fw, "running previous version");
    impl_->prefs.remove("fota_id"); impl_->prefs.remove("fota_ver"); impl_->prefs.remove("fota_prev");
}

bool Ctrl32Telemetry::provision(const char *baseUrl, const char *claim, const char *hw, const char *fw, String &deviceId, String &token, const char *caCert, bool insecure) {
    WiFiClientSecure secure; WiFiClient plain;
    String url = String(baseUrl) + "/api/v1/provision";
    bool https = url.startsWith("https://");
    if (https) { if (insecure) secure.setInsecure(); else if (caCert) secure.setCACert(caCert); }
    HTTPClient http;
    if (!(https ? http.begin(secure, url) : http.begin(plain, url))) return false;
    http.addHeader("Content-Type", "application/json");
    char body[256];
    snprintf(body, sizeof body, "{\"claim\":\"%s\",\"hw\":\"%s\",\"fw\":\"%s\"}", claim, hw ? hw : "", fw ? fw : "");
    int code = http.POST((uint8_t *)body, strlen(body));
    String resp = http.getString();
    http.end();
    if (code != 200 && code != 201) return false;
    c32_jval_t v; char tmp[128];
    if (c32_json_get(resp.c_str(), resp.length(), "token", &v) == 1 && c32_json_str(&v, tmp, sizeof tmp) > 0) token = tmp; else return false;
    if (c32_json_get(resp.c_str(), resp.length(), "external_id", &v) == 1 && c32_json_str(&v, tmp, sizeof tmp) > 0) deviceId = tmp;
    return true;
}
