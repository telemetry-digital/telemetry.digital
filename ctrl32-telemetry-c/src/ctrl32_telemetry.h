/*
 * ctrl32 telemetry — device protocol core (protocol v0.1), portable C99.
 *
 * This file has no dependencies beyond the C standard library and never allocates: every function writes into
 * caller-provided buffers. Transports (MQTT, HTTP, TLS, OTA) live in the ports: ports/arduino, ports/esp-idf.
 *
 * Topics (all under d/{deviceId}/):
 *   telemetry, attributes, status, cmd/ack, console/out, fota/status, attributes/request   device -> server
 *   attributes/shared (retained), attributes/response, cmd, console/in, fota/target        server -> device
 *
 * Telemetry: {"ts": 1758620000, "v": {"temp": 4.5, "door": true}, "seq": 12}  or a JSON array of those (batch);
 * "bf": 1 marks samples replayed from the offline buffer. "ts" is unix seconds; "rel": -30 means 30 s before now
 * (for devices without a clock). Without "ts"/"rel" the server stamps the message.
 */
#ifndef CTRL32_TELEMETRY_H
#define CTRL32_TELEMETRY_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define C32_VERSION "0.1.0"

#ifndef C32_MAX_VALUES
#define C32_MAX_VALUES 8       /* values per sample */
#endif
#ifndef C32_KEY_LEN
#define C32_KEY_LEN 16         /* datastream key incl. NUL: ^[a-z][a-z0-9_]{0,31}$ (shorter here to save RAM) */
#endif
#define C32_ID_LEN 40          /* uuid text + NUL */
#define C32_METHOD_LEN 33
#define C32_VER_LEN 33
#define C32_SHA256_LEN 65
#define C32_URL_LEN 256

typedef enum { C32_NUM = 0, C32_BOOL = 1, C32_STR = 2 } c32_vtype_t;

typedef struct {
    char key[C32_KEY_LEN];
    c32_vtype_t type;
    double num;                /* C32_NUM, C32_BOOL (0/1) */
    const char *str;           /* C32_STR: pointer valid until the sample is encoded (not copied into the ring) */
} c32_value_t;

typedef struct {
    int64_t ts;                /* unix seconds; 0 = no timestamp */
    int32_t rel;               /* seconds relative to now (negative = past); used when ts == 0 and rel != 0 */
    uint32_t seq;              /* monotonically increasing sample sequence (0 = none) */
    uint8_t backfilled;        /* set by the ring buffer on replay */
    uint8_t n;
    c32_value_t v[C32_MAX_VALUES];
} c32_sample_t;

/* ---- building messages (return bytes written excluding NUL, or -1 if cap is too small) ---- */
int c32_sample_add(c32_sample_t *s, const char *key, double value);          /* -1 for NaN/Inf (e.g. a failed sensor read) */
int c32_sample_add_bool(c32_sample_t *s, const char *key, int value);
int c32_sample_add_str(c32_sample_t *s, const char *key, const char *value);
int c32_telemetry_json(char *out, size_t cap, const c32_sample_t *samples, size_t n); /* n == 1 -> object, else array */

typedef struct {
    int online;                /* 1 */
    double battery_v;          /* 0 = omit */
    int rssi;                  /* 0 = omit */
    uint32_t uptime_s;
    const char *fw;            /* NULL = omit */
    uint32_t buffered;         /* samples waiting in the offline buffer */
} c32_status_t;
int c32_status_json(char *out, size_t cap, const c32_status_t *st);
int c32_attributes_json(char *out, size_t cap, const char *fw, const char *hw, const char *model, const char *ip);

/* ---- commands (protocol §3.4) ---- */
typedef struct {
    char id[C32_ID_LEN];
    char method[C32_METHOD_LEN];
    const char *args;          /* raw JSON of "a" inside the input buffer (not NUL-terminated) */
    size_t args_len;
    int64_t exp;               /* unix seconds, 0 = none */
} c32_cmd_t;
int c32_cmd_parse(const char *json, size_t len, c32_cmd_t *cmd);
/* result_json may be NULL; for ok == 0 pass the error text in err (may be NULL) */
int c32_ack_json(char *out, size_t cap, const char *id, int ok, const char *result_json, const char *err);

/* ---- shared attributes (protocol §3.2): flat object lookups ---- */
typedef enum { C32_J_NONE = 0, C32_J_NUM, C32_J_BOOL, C32_J_STR, C32_J_NULL, C32_J_OBJ, C32_J_ARR } c32_jtype_t;
typedef struct {
    c32_jtype_t type;
    double num;                /* C32_J_NUM, C32_J_BOOL */
    const char *raw;           /* start of the value inside the input (for strings: first char after the quote) */
    size_t len;                /* raw length (for strings: escaped length without quotes) */
} c32_jval_t;
int c32_json_get(const char *json, size_t len, const char *key, c32_jval_t *val);    /* 1 found, 0 not found, -1 malformed */
int c32_json_str(const c32_jval_t *val, char *out, size_t cap);                      /* unescape a string value */
int c32_attr_num(const char *json, size_t len, const char *key, double *out);        /* convenience: 1 if numeric */
int c32_attr_bool(const char *json, size_t len, const char *key, int *out);
int c32_attr_request_json(char *out, size_t cap, const char *const *keys, size_t nkeys); /* keys NULL -> {} */

/* ---- console (protocol §3.5) ---- */
int c32_console_in_parse(const char *json, size_t len, char *session, size_t scap, char *line, size_t lcap);
int c32_console_out_json(char *out, size_t cap, const char *session, int64_t t, const char *line);

/* ---- FOTA (protocol §3.6) ---- */
typedef struct {
    char fw_id[C32_ID_LEN];
    char ver[C32_VER_LEN];
    char hw[33];
    char min_ver[C32_VER_LEN];
    char sha256[C32_SHA256_LEN];
    char url[C32_URL_LEN];
    char campaign[C32_ID_LEN];
    uint32_t size;
} c32_fota_target_t;
int c32_fota_target_parse(const char *json, size_t len, c32_fota_target_t *t);        /* 1 ok, 0 empty (cleared), -1 malformed */
int c32_fota_status_json(char *out, size_t cap, const char *fw_id, const char *state, int pct, const char *ver, const char *err);
/* "1.2.3" style comparison: <0, 0, >0; non-numeric parts compare as strings */
int c32_version_cmp(const char *a, const char *b);
/* constant-time hex compare of two SHA-256 hex strings (case-insensitive) */
int c32_sha256_hex_equal(const char *a, const char *b);

/* ---- topics ---- */
int c32_topic(char *out, size_t cap, const char *device_id, const char *suffix);     /* "d/{id}/{suffix}" */
/* returns the suffix of a topic under d/{device_id}/ or NULL */
const char *c32_topic_suffix(const char *topic, const char *device_id);

/* ---- offline ring buffer (RAM; the port may persist it) ---- */
typedef struct {
    c32_sample_t *slots;
    size_t cap;
    size_t head;               /* oldest */
    size_t count;
    uint32_t dropped;          /* samples overwritten because the buffer was full */
} c32_ring_t;
void c32_ring_init(c32_ring_t *r, c32_sample_t *slots, size_t cap);
void c32_ring_push(c32_ring_t *r, const c32_sample_t *s);            /* overwrites the oldest when full */
size_t c32_ring_count(const c32_ring_t *r);
/* encode up to max_batch oldest samples as a backfill batch (bf=1); returns bytes or -1; *taken = samples encoded */
int c32_ring_encode_batch(const c32_ring_t *r, char *out, size_t cap, size_t max_batch, size_t *taken);
void c32_ring_drop(c32_ring_t *r, size_t n);                          /* after a successful publish */

/* ---- misc ---- */
int c32_json_escape(char *out, size_t cap, const char *s);            /* writes a quoted, escaped JSON string */
int c32_key_valid(const char *key);                                   /* ^[a-z][a-z0-9_]{0,31}$ */

#ifdef __cplusplus
}
#endif
#endif /* CTRL32_TELEMETRY_H */
