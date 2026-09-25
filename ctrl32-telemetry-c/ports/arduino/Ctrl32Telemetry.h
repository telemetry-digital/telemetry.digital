/*
 * ctrl32 telemetry — Arduino port for ESP32 (protocol v0.1).
 *
 * Dependencies: ESP32 Arduino core 2.x/3.x, PubSubClient (MIT). TLS uses WiFiClientSecure; FOTA uses HTTPClient
 * and the Update library with SHA-256 verification (mbedtls); the pending-update marker lives in NVS (Preferences).
 *
 *   #include <Ctrl32Telemetry.h>
 *   Ctrl32Telemetry tel;
 *   void setup() { WiFi.begin(...); Ctrl32Config c; c.host = "telemetry.digital"; c.deviceId = "esp32-demo"; c.token = "..."; tel.begin(c); }
 *   void loop()  { tel.loop(); ... tel.send("temp", 4.5); }
 */
#ifndef CTRL32_TELEMETRY_ARDUINO_H
#define CTRL32_TELEMETRY_ARDUINO_H

#include <Arduino.h>
#include <functional>

#include "ctrl32_telemetry.h"

struct Ctrl32Config {
    const char *host = nullptr;        // MQTT broker host (the ctrl32 telemetry server)
    uint16_t port = 8883;              // 8883 TLS (internet), 1883 plain (intranet only)
    const char *deviceId = nullptr;    // external id = MQTT username
    const char *token = nullptr;       // device token = MQTT password
    bool tls = true;
    const char *caCert = nullptr;      // PEM root CA; nullptr with insecure=false uses the ESP32 bundle when available
    bool insecure = false;             // skip certificate verification (development only)
    const char *fw = "0.0.0";          // running firmware version (reported, compared for FOTA)
    const char *hw = "esp32";          // hardware id (must match the firmware image)
    const char *model = nullptr;
    uint16_t heartbeatS = 300;         // status heartbeat interval
    uint16_t keepAliveS = 30;
    size_t bufferSlots = 64;           // offline ring buffer (RAM), samples
    bool autoFota = true;              // apply fota/target automatically (hw matches, newer version)
    bool syncTime = true;              // configure SNTP so samples carry "ts"
    const char *ntp = "pool.ntp.org";
};

class Ctrl32Telemetry {
  public:
    using CommandHandler = std::function<bool(const char *method, const char *argsJson, size_t argsLen, String &resultJson, String &err)>;
    using AttributesHandler = std::function<void(const char *json, size_t len)>;
    using ConsoleHandler = std::function<void(const char *line)>;
    using FotaHandler = std::function<bool(const c32_fota_target_t &target)>;

    Ctrl32Telemetry();
    ~Ctrl32Telemetry();
    bool begin(const Ctrl32Config &cfg);
    void loop();                                   // call often; maintains the connection, heartbeat and backfill
    bool connected() const;

    bool send(const c32_sample_t &sample);         // publish now, or buffer when offline
    bool send(const char *key, double value);
    bool sendBool(const char *key, bool value);
    bool sendBatch(const c32_sample_t *samples, size_t n);
    void sendStatus();                             // heartbeat now
    void consoleOut(const char *line);             // only delivered while a console session is open
    bool consoleEnabled() const { return consoleEnabled_; }
    size_t buffered() const { return c32_ring_count(&ring_); }
    int64_t now() const;                           // unix seconds when time is synchronised, else 0
    uint32_t nextSeq() { return ++seq_; }

    void onCommand(CommandHandler h) { onCommand_ = h; }
    void onSharedAttributes(AttributesHandler h) { onAttrs_ = h; }
    void onConsoleLine(ConsoleHandler h) { onConsole_ = h; }
    void onFotaTarget(FotaHandler h) { onFota_ = h; }   // return false to refuse the update
    double sharedNumber(const char *key, double fallback) const;   // last received shared attributes
    bool sharedBool(const char *key, bool fallback) const;
    void requestSharedAttributes();

    // Claim-code provisioning over HTTPS: POST {baseUrl}/api/v1/provision {"claim","hw","fw"} -> token.
    static bool provision(const char *baseUrl, const char *claim, const char *hw, const char *fw, String &deviceId, String &token,
                          const char *caCert = nullptr, bool insecure = false);

  private:
    struct Impl;
    Impl *impl_;
    Ctrl32Config cfg_;
    c32_ring_t ring_;
    c32_sample_t *slots_ = nullptr;
    uint32_t seq_ = 0;
    bool consoleEnabled_ = false;
    String consoleSession_;
    String shared_;                                // last shared attributes document
    unsigned long lastHeartbeat_ = 0, lastAttempt_ = 0, backoffMs_ = 1000;
    CommandHandler onCommand_;
    AttributesHandler onAttrs_;
    ConsoleHandler onConsole_;
    FotaHandler onFota_;

    bool connect();
    void onConnected();
    void handleMessage(const char *topic, const uint8_t *payload, unsigned len);
    void handleCommand(const char *json, size_t len);
    void handleShared(const char *json, size_t len);
    void handleFota(const char *json, size_t len);
    bool publish(const char *suffix, const char *json, bool retain = false);
    void flushBuffer();
    void fotaStatus(const char *fwId, const char *state, int pct, const char *ver, const char *err);
    void runFota(const c32_fota_target_t &t);
    void reportPendingFota();
};

#endif
