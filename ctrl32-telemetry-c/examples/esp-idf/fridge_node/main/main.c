/* ctrl32 telemetry — ESP-IDF example: fridge node (temperature + humidity every N seconds, commands, console, FOTA). */
#include <math.h>
#include <stdio.h>
#include <string.h>

#include "esp_event.h"
#include "esp_log.h"
#include "esp_netif.h"
#include "esp_wifi.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "nvs_flash.h"

#include "ctrl32_telemetry_idf.h"

#define FW_VERSION "1.0.0"
#define HW_ID "esp32-s3"
#define WIFI_SSID "your-wifi"
#define WIFI_PASS "your-password"
#define SERVER_URI "mqtts://telemetry.digital:8883"
#define DEVICE_ID "fridge-02"
#define TOKEN "paste-device-token"

static const char *TAG = "fridge";
static uint32_t interval_s = 60;

static void on_attrs(const char *json, size_t len, void *user) {
    double v;
    if (c32_attr_num(json, len, "interval_s", &v) && v >= 5) interval_s = (uint32_t)v;
    ESP_LOGI(TAG, "shared attributes: %.*s", (int)len, json);
}
static bool on_command(const char *method, const char *args, size_t alen, char *result, size_t rcap, char *err, size_t ecap, void *user) {
    if (strcmp(method, "set_interval") == 0) {
        double s;
        if (c32_attr_num(args, alen, "s", &s) && s >= 5) { interval_s = (uint32_t)s; snprintf(result, rcap, "{\"s\":%u}", (unsigned)interval_s); return true; }
        snprintf(err, ecap, "bad args"); return false;
    }
    snprintf(err, ecap, "unsupported"); return false;
}
static void on_console(const char *line, void *user) {
    char out[128];
    if (strcmp(line, "temp") == 0) snprintf(out, sizeof out, "temp %.2f", 4.5 + 0.3 * sin(esp_log_timestamp() / 60000.0));
    else snprintf(out, sizeof out, "unknown: %s", line);
    c32_idf_console_out(out);
}

static void wifi_event(void *arg, esp_event_base_t base, int32_t id, void *data) {
    if (base == WIFI_EVENT && id == WIFI_EVENT_STA_START) esp_wifi_connect();
    else if (base == WIFI_EVENT && id == WIFI_EVENT_STA_DISCONNECTED) esp_wifi_connect();
}

void app_main(void) {
    ESP_ERROR_CHECK(nvs_flash_init());
    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());
    esp_netif_create_default_wifi_sta();
    wifi_init_config_t wc = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&wc));
    ESP_ERROR_CHECK(esp_event_handler_register(WIFI_EVENT, ESP_EVENT_ANY_ID, wifi_event, NULL));
    wifi_config_t sta = {0};
    strncpy((char *)sta.sta.ssid, WIFI_SSID, sizeof sta.sta.ssid);
    strncpy((char *)sta.sta.password, WIFI_PASS, sizeof sta.sta.password);
    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &sta));
    ESP_ERROR_CHECK(esp_wifi_start());

    c32_idf_config_t cfg = {
        .uri = SERVER_URI, .device_id = DEVICE_ID, .token = TOKEN,
        .fw = FW_VERSION, .hw = HW_ID, .model = "fridge-node",
        .heartbeat_s = 300, .buffer_slots = 128, .auto_fota = true,
        .on_command = on_command, .on_attributes = on_attrs, .on_console = on_console,
    };
    ESP_ERROR_CHECK(c32_idf_start(&cfg));           /* reconnects and replays buffered samples by itself */

    for (;;) {
        c32_sample_t s; memset(&s, 0, sizeof s);
        s.ts = c32_idf_now(); s.seq = c32_idf_next_seq();
        c32_sample_add(&s, "temp", 4.5 + 0.3 * sin(esp_log_timestamp() / 60000.0));
        c32_sample_add(&s, "rh", 55 + (esp_log_timestamp() / 1000) % 3);
        bool sent = c32_idf_send(&s);
        ESP_LOGI(TAG, "sample %s (buffered %u)", sent ? "sent" : "queued", (unsigned)c32_idf_buffered());
        vTaskDelay(pdMS_TO_TICKS(interval_s * 1000));
    }
}
