/* Host unit tests for the protocol core. Build: tcc -Isrc tests/core_test.c src/ctrl32_telemetry.c -run  (or any C99 compiler) */
#include <stdio.h>
#include <string.h>

#include "ctrl32_telemetry.h"

static int fails = 0, passes = 0;
#define CHECK(cond, msg) do { if (cond) { passes++; } else { fails++; printf("  FAIL %s (line %d)\n", msg, __LINE__); } } while (0)
#define CHECK_STR(a, b, msg) do { if (strcmp((a), (b)) == 0) { passes++; } else { fails++; printf("  FAIL %s: got %s want %s\n", msg, (a), (b)); } } while (0)

int main(void) {
    char buf[1024];
    /* telemetry single sample */
    c32_sample_t s; memset(&s, 0, sizeof s);
    s.ts = 1758620000; s.seq = 12;
    CHECK(c32_sample_add(&s, "temp", 4.5) == 0, "add temp");
    CHECK(c32_sample_add_bool(&s, "door", 1) == 0, "add door");
    CHECK(c32_sample_add_str(&s, "mode", "eco \"x\"") == 0, "add str");
    CHECK(c32_sample_add(&s, "Bad Key", 1) == -1, "invalid key rejected");
    CHECK(c32_telemetry_json(buf, sizeof buf, &s, 1) > 0, "encode");
    CHECK_STR(buf, "{\"ts\":1758620000,\"v\":{\"temp\":4.5,\"door\":true,\"mode\":\"eco \\\"x\\\"\"},\"seq\":12}", "telemetry json");
    CHECK(c32_telemetry_json(buf, 20, &s, 1) == -1, "small buffer -> -1");
    /* relative time */
    c32_sample_t r; memset(&r, 0, sizeof r); r.rel = -30; c32_sample_add(&r, "rh", 55);
    c32_telemetry_json(buf, sizeof buf, &r, 1);
    CHECK_STR(buf, "{\"rel\":-30,\"v\":{\"rh\":55}}", "relative sample");
    /* batch */
    c32_sample_t two[2] = {s, r};
    CHECK(c32_telemetry_json(buf, sizeof buf, two, 2) > 0 && buf[0] == '[' && buf[strlen(buf) - 1] == ']', "batch is an array");
    /* status + attributes */
    c32_status_t st = {1, 3.61, -71, 86400, "1.0.0", 0};
    c32_status_json(buf, sizeof buf, &st);
    CHECK_STR(buf, "{\"online\":true,\"bat\":3.61,\"rssi\":-71,\"up_s\":86400,\"fw\":\"1.0.0\",\"buf\":0}", "status json");
    c32_attributes_json(buf, sizeof buf, "1.0.0", "esp32-s3", NULL, "10.0.0.5");
    CHECK(strstr(buf, "\"hw\":\"esp32-s3\"") && strstr(buf, "\"lib\":\"ctrl32-telemetry-c") && !strstr(buf, "model"), "attributes json");
    /* command parse + ack */
    const char *cmd = "{\"id\": \"c-01J9\", \"m\": \"set_interval\", \"a\": {\"s\": 60, \"nested\": {\"x\": [1,2]}}, \"exp\": 1758620600}";
    c32_cmd_t c;
    CHECK(c32_cmd_parse(cmd, strlen(cmd), &c) == 0, "cmd parse");
    CHECK_STR(c.id, "c-01J9", "cmd id");
    CHECK_STR(c.method, "set_interval", "cmd method");
    CHECK(c.exp == 1758620600, "cmd exp");
    CHECK(c.args && c.args_len == strlen("{\"s\": 60, \"nested\": {\"x\": [1,2]}}"), "cmd args span");
    double sv; CHECK(c32_attr_num(c.args, c.args_len, "s", &sv) == 1 && sv == 60, "args lookup");
    CHECK(c32_cmd_parse("{\"m\":\"x\"}", 9, &c) == -1, "cmd without id rejected");
    c32_ack_json(buf, sizeof buf, "c-01J9", 1, "{\"s\":60}", NULL);
    CHECK_STR(buf, "{\"id\":\"c-01J9\",\"ok\":true,\"r\":{\"s\":60}}", "ack ok");
    c32_ack_json(buf, sizeof buf, "c-01J9", 0, NULL, "unsupported");
    CHECK_STR(buf, "{\"id\":\"c-01J9\",\"ok\":false,\"err\":\"unsupported\"}", "ack err");
    /* shared attributes */
    const char *attrs = "{\"interval_s\": 300, \"temp_hi\": 8.0, \"console.enabled\": false, \"tz\": \"Europe/Bratislava\", \"obj\": {\"a\": \"}\"}, \"last\": true}";
    double iv; int en; c32_jval_t v; char str[64];
    CHECK(c32_attr_num(attrs, strlen(attrs), "interval_s", &iv) == 1 && iv == 300, "interval_s");
    CHECK(c32_attr_bool(attrs, strlen(attrs), "console.enabled", &en) == 1 && en == 0, "console.enabled");
    CHECK(c32_attr_bool(attrs, strlen(attrs), "last", &en) == 1 && en == 1, "key after nested object");
    CHECK(c32_json_get(attrs, strlen(attrs), "tz", &v) == 1 && v.type == C32_J_STR && c32_json_str(&v, str, sizeof str) > 0, "tz string");
    CHECK_STR(str, "Europe/Bratislava", "tz value");
    CHECK(c32_json_get(attrs, strlen(attrs), "missing", &v) == 0, "missing key");
    CHECK(c32_json_get("not json", 8, "x", &v) == -1, "malformed");
    const char *keys[] = {"interval_s", "temp_hi"};
    c32_attr_request_json(buf, sizeof buf, keys, 2);
    CHECK_STR(buf, "{\"keys\":[\"interval_s\",\"temp_hi\"]}", "attr request");
    c32_attr_request_json(buf, sizeof buf, NULL, 0);
    CHECK_STR(buf, "{}", "attr request all");
    /* console */
    char sess[48], line[128];
    CHECK(c32_console_in_parse("{\"s\":\"sess-1\",\"l\":\"wifi status\\n\"}", 36, sess, sizeof sess, line, sizeof line) == 0, "console in");
    CHECK_STR(sess, "sess-1", "session"); CHECK_STR(line, "wifi status\n", "line unescaped");
    c32_console_out_json(buf, sizeof buf, "sess-1", 1758620001, "RSSI -71 dBm");
    CHECK_STR(buf, "{\"s\":\"sess-1\",\"t\":1758620001,\"l\":\"RSSI -71 dBm\"}", "console out");
    /* fota */
    const char *tgt = "{\"fw_id\":\"f-1\",\"ver\":\"1.5.0\",\"hw\":\"esp32-s3\",\"size\":120000,\"sha256\":\"ab\",\"url\":\"https://srv/api/v1/d/fota/f-1/blob\",\"min_ver\":\"1.3.0\",\"campaign\":\"c-1\"}";
    c32_fota_target_t t;
    CHECK(c32_fota_target_parse(tgt, strlen(tgt), &t) == 1, "fota parse");
    CHECK_STR(t.url, "https://srv/api/v1/d/fota/f-1/blob", "fota url"); CHECK(t.size == 120000, "fota size"); CHECK_STR(t.min_ver, "1.3.0", "min ver");
    CHECK(c32_fota_target_parse("", 0, &t) == 0, "cleared target");
    CHECK(c32_fota_target_parse("{\"fw_id\":\"x\"}", 12, &t) == -1, "incomplete target");
    c32_fota_status_json(buf, sizeof buf, "f-1", "downloading", 42, NULL, NULL);
    CHECK_STR(buf, "{\"fw_id\":\"f-1\",\"st\":\"downloading\",\"pct\":42}", "fota status");
    c32_fota_status_json(buf, sizeof buf, "f-1", "failed", -1, NULL, "sig_invalid");
    CHECK_STR(buf, "{\"fw_id\":\"f-1\",\"st\":\"failed\",\"err\":\"sig_invalid\"}", "fota failed");
    CHECK(c32_version_cmp("1.5.0", "1.10.0") < 0 && c32_version_cmp("2.0", "1.9.9") > 0 && c32_version_cmp("1.0.0", "1.0.0") == 0, "version cmp");
    CHECK(c32_sha256_hex_equal("ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"), "sha equal");
    CHECK(!c32_sha256_hex_equal("ab", "ab"), "sha wrong length");
    /* topics */
    c32_topic(buf, sizeof buf, "esp32-demo", "telemetry");
    CHECK_STR(buf, "d/esp32-demo/telemetry", "topic");
    CHECK(strcmp(c32_topic_suffix("d/esp32-demo/cmd/ack", "esp32-demo"), "cmd/ack") == 0, "topic suffix");
    CHECK(c32_topic_suffix("d/other/cmd", "esp32-demo") == NULL, "foreign topic");
    /* ring buffer */
    c32_sample_t slots[3]; c32_ring_t ring; c32_ring_init(&ring, slots, 3);
    for (int i = 0; i < 5; i++) { c32_sample_t x; memset(&x, 0, sizeof x); x.ts = 1000 + i; x.seq = (uint32_t)i + 1; c32_sample_add(&x, "temp", i); c32_ring_push(&ring, &x); }
    CHECK(c32_ring_count(&ring) == 3 && ring.dropped == 2, "ring overwrites oldest");
    size_t taken;
    CHECK(c32_ring_encode_batch(&ring, buf, sizeof buf, 10, &taken) > 0 && taken == 3, "batch encode");
    CHECK(strstr(buf, "\"ts\":1002") && strstr(buf, "\"bf\":1") && strstr(buf, "\"seq\":5"), "batch content (oldest surviving first, bf flag)");
    CHECK(c32_ring_encode_batch(&ring, buf, 60, 10, &taken) > 0 && taken == 1, "batch limited by buffer size");
    c32_ring_drop(&ring, 2);
    CHECK(c32_ring_count(&ring) == 1, "drop");
    printf("core tests: %d passed, %d failed\n", passes, fails);
    return fails ? 1 : 0;
}
