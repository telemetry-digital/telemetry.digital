// ctrl32 telemetry — Arduino ESP32 example: a fridge node that reports temperature and humidity, keeps samples
// while offline, handles commands (set_interval), shared attributes, the remote console and FOTA.
//
// Server side: create the device on /devices (external id below), issue a token, assign datastreams "temp" and "rh".
#include <WiFi.h>
#include <Ctrl32Telemetry.h>

#define FW_VERSION "1.0.0"
#define HW_ID "esp32-s3"

const char *WIFI_SSID = "your-wifi";
const char *WIFI_PASS = "your-password";
const char *SERVER = "telemetry.digital";      // your ctrl32 telemetry instance
const char *DEVICE_ID = "fridge-01";           // external id
const char *TOKEN = "paste-device-token";      // from /devices -> Issue token (or use Ctrl32Telemetry::provision)

Ctrl32Telemetry tel;
uint32_t intervalS = 60;
unsigned long lastSample = 0;

float readTemperature() { return 4.5 + 0.3 * sin(millis() / 60000.0); }   // replace with a real sensor
float readHumidity() { return 55 + (millis() / 1000) % 3; }

void setup() {
    Serial.begin(115200);
    WiFi.begin(WIFI_SSID, WIFI_PASS);
    while (WiFi.status() != WL_CONNECTED) { delay(500); Serial.print("."); }
    Serial.println("\nwifi ok");

    Ctrl32Config c;
    c.host = SERVER; c.port = 8883; c.tls = true;
    c.insecure = true;                              // development: no CA check. Production: c.caCert = ROOT_CA_PEM;
    c.deviceId = DEVICE_ID; c.token = TOKEN;
    c.fw = FW_VERSION; c.hw = HW_ID; c.model = "fridge-node";
    c.heartbeatS = 300; c.bufferSlots = 128;

    tel.onSharedAttributes([](const char *json, size_t len) {
        double v;
        if (c32_attr_num(json, len, "interval_s", &v) && v >= 5) intervalS = (uint32_t)v;
        Serial.printf("shared attributes: %.*s\n", (int)len, json);
    });
    tel.onCommand([](const char *method, const char *args, size_t alen, String &result, String &err) -> bool {
        if (strcmp(method, "set_interval") == 0) {
            double s;
            if (c32_attr_num(args, alen, "s", &s) && s >= 5) { intervalS = (uint32_t)s; result = "{\"s\":" + String(intervalS) + "}"; return true; }
            err = "bad args"; return false;
        }
        err = "unsupported"; return false;
    });
    tel.onConsoleLine([](const char *line) {
        if (strcmp(line, "wifi status") == 0) tel.consoleOut((String("RSSI ") + WiFi.RSSI() + " dBm, ip " + WiFi.localIP().toString()).c_str());
        else if (strcmp(line, "temp") == 0) tel.consoleOut((String("temp ") + readTemperature()).c_str());
        else tel.consoleOut((String("unknown: ") + line).c_str());
    });
    tel.begin(c);
}

void loop() {
    tel.loop();
    if (millis() - lastSample >= intervalS * 1000UL) {
        lastSample = millis();
        c32_sample_t s; memset(&s, 0, sizeof s);
        s.ts = tel.now(); s.seq = tel.nextSeq();
        c32_sample_add(&s, "temp", readTemperature());
        c32_sample_add(&s, "rh", readHumidity());
        bool sent = tel.send(s);                     // buffered automatically when offline, replayed with bf=1
        Serial.printf("sample %s (buffered %u)\n", sent ? "sent" : "queued", (unsigned)tel.buffered());
    }
}
