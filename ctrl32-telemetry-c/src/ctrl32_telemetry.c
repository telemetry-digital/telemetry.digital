/* ctrl32 telemetry — device protocol core. C99, no allocation, no dependencies beyond libc. */
#include "ctrl32_telemetry.h"

#include <ctype.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* ---------------------------------------------------------------- small writer ---- */
typedef struct {
    char *p;
    size_t cap, len;
    int overflow;
} w_t;

static void w_init(w_t *w, char *buf, size_t cap) { w->p = buf; w->cap = cap; w->len = 0; w->overflow = (cap == 0); if (cap) buf[0] = 0; }
static void w_putc(w_t *w, char c) {
    if (w->len + 1 < w->cap) { w->p[w->len++] = c; w->p[w->len] = 0; } else { w->overflow = 1; }
}
static void w_puts(w_t *w, const char *s) { while (*s) w_putc(w, *s++); }
static void w_num(w_t *w, double v) {
    char tmp[32];
    if (!isfinite(v)) { w_puts(w, "null"); return; } /* NaN/Inf are not JSON numbers */
    if (v < 1e15 && v > -1e15 && v == (double)(long long)v) snprintf(tmp, sizeof tmp, "%lld", (long long)v);
    else snprintf(tmp, sizeof tmp, "%.6g", v);
    w_puts(w, tmp);
}
static void w_u32(w_t *w, uint32_t v) { char tmp[16]; snprintf(tmp, sizeof tmp, "%lu", (unsigned long)v); w_puts(w, tmp); }
static void w_i64(w_t *w, int64_t v) { char tmp[24]; snprintf(tmp, sizeof tmp, "%lld", (long long)v); w_puts(w, tmp); }
static void w_str(w_t *w, const char *s) {
    w_putc(w, '"');
    for (; *s; s++) {
        unsigned char c = (unsigned char)*s;
        switch (c) {
        case '"': w_puts(w, "\\\""); break;
        case '\\': w_puts(w, "\\\\"); break;
        case '\n': w_puts(w, "\\n"); break;
        case '\r': w_puts(w, "\\r"); break;
        case '\t': w_puts(w, "\\t"); break;
        default:
            if (c < 0x20) { char tmp[8]; snprintf(tmp, sizeof tmp, "\\u%04x", c); w_puts(w, tmp); }
            else w_putc(w, (char)c);
        }
    }
    w_putc(w, '"');
}
static int w_done(const w_t *w) { return w->overflow ? -1 : (int)w->len; }

int c32_json_escape(char *out, size_t cap, const char *s) { w_t w; w_init(&w, out, cap); w_str(&w, s); return w_done(&w); }

int c32_key_valid(const char *key) {
    size_t n = key ? strlen(key) : 0;
    if (n < 1 || n > 32 || !(key[0] >= 'a' && key[0] <= 'z')) return 0;
    for (size_t i = 1; i < n; i++) {
        char c = key[i];
        if (!((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_')) return 0;
    }
    return 1;
}

/* ---------------------------------------------------------------- samples ---- */
static int add_value(c32_sample_t *s, const char *key, c32_vtype_t t, double num, const char *str) {
    if (!s || s->n >= C32_MAX_VALUES || !c32_key_valid(key) || strlen(key) >= C32_KEY_LEN) return -1;
    c32_value_t *v = &s->v[s->n];
    strncpy(v->key, key, C32_KEY_LEN - 1);
    v->key[C32_KEY_LEN - 1] = 0;
    v->type = t; v->num = num; v->str = str;
    s->n++;
    return 0;
}
int c32_sample_add(c32_sample_t *s, const char *key, double value) { return isfinite(value) ? add_value(s, key, C32_NUM, value, NULL) : -1; }
int c32_sample_add_bool(c32_sample_t *s, const char *key, int value) { return add_value(s, key, C32_BOOL, value ? 1 : 0, NULL); }
int c32_sample_add_str(c32_sample_t *s, const char *key, const char *value) { return add_value(s, key, C32_STR, 0, value); }

static void w_sample(w_t *w, const c32_sample_t *s) {
    w_putc(w, '{');
    if (s->ts) { w_puts(w, "\"ts\":"); w_i64(w, s->ts); w_putc(w, ','); }
    else if (s->rel) { w_puts(w, "\"rel\":"); w_i64(w, s->rel); w_putc(w, ','); }
    w_puts(w, "\"v\":{");
    for (uint8_t i = 0; i < s->n; i++) {
        if (i) w_putc(w, ',');
        w_str(w, s->v[i].key);
        w_putc(w, ':');
        switch (s->v[i].type) {
        case C32_BOOL: w_puts(w, s->v[i].num ? "true" : "false"); break;
        case C32_STR: w_str(w, s->v[i].str ? s->v[i].str : ""); break;
        default: w_num(w, s->v[i].num);
        }
    }
    w_putc(w, '}');
    if (s->backfilled) w_puts(w, ",\"bf\":1");
    if (s->seq) { w_puts(w, ",\"seq\":"); w_u32(w, s->seq); }
    w_putc(w, '}');
}

int c32_telemetry_json(char *out, size_t cap, const c32_sample_t *samples, size_t n) {
    w_t w; w_init(&w, out, cap);
    if (n == 0) return -1;
    if (n == 1) { w_sample(&w, &samples[0]); return w_done(&w); }
    w_putc(&w, '[');
    for (size_t i = 0; i < n; i++) { if (i) w_putc(&w, ','); w_sample(&w, &samples[i]); }
    w_putc(&w, ']');
    return w_done(&w);
}

int c32_status_json(char *out, size_t cap, const c32_status_t *st) {
    w_t w; w_init(&w, out, cap);
    w_puts(&w, st->online ? "{\"online\":true" : "{\"online\":false");
    if (st->battery_v > 0) { w_puts(&w, ",\"bat\":"); w_num(&w, st->battery_v); }
    if (st->rssi) { w_puts(&w, ",\"rssi\":"); w_num(&w, st->rssi); }
    w_puts(&w, ",\"up_s\":"); w_u32(&w, st->uptime_s);
    if (st->fw) { w_puts(&w, ",\"fw\":"); w_str(&w, st->fw); }
    w_puts(&w, ",\"buf\":"); w_u32(&w, st->buffered);
    w_putc(&w, '}');
    return w_done(&w);
}

int c32_attributes_json(char *out, size_t cap, const char *fw, const char *hw, const char *model, const char *ip) {
    w_t w; w_init(&w, out, cap);
    int first = 1;
    w_putc(&w, '{');
#define ATTR(k, v) if (v) { if (!first) w_putc(&w, ','); first = 0; w_puts(&w, "\"" k "\":"); w_str(&w, v); }
    ATTR("fw", fw) ATTR("hw", hw) ATTR("model", model) ATTR("ip", ip)
#undef ATTR
    w_puts(&w, ",\"lib\":\"ctrl32-telemetry-c " C32_VERSION "\"}");
    return w_done(&w);
}

/* ---------------------------------------------------------------- JSON scanner ---- */
static const char *skip_ws(const char *p, const char *end) { while (p < end && (*p == ' ' || *p == '\t' || *p == '\n' || *p == '\r')) p++; return p; }

/* returns pointer past the string's closing quote, or NULL */
static const char *skip_string(const char *p, const char *end) {
    if (p >= end || *p != '"') return NULL;
    p++;
    while (p < end) {
        if (*p == '\\') { p += 2; continue; }
        if (*p == '"') return p + 1;
        p++;
    }
    return NULL;
}

/* skips one JSON value; returns pointer after it or NULL */
static const char *skip_value(const char *p, const char *end) {
    p = skip_ws(p, end);
    if (p >= end) return NULL;
    if (*p == '"') return skip_string(p, end);
    if (*p == '{' || *p == '[') {
        int depth = 0;
        while (p < end) {
            if (*p == '"') { p = skip_string(p, end); if (!p) return NULL; continue; }
            if (*p == '{' || *p == '[') depth++;
            else if (*p == '}' || *p == ']') { depth--; if (depth == 0) return p + 1; }
            p++;
        }
        return NULL;
    }
    while (p < end && *p != ',' && *p != '}' && *p != ']' && *p != ' ' && *p != '\n' && *p != '\r' && *p != '\t') p++;
    return p;
}

int c32_json_get(const char *json, size_t len, const char *key, c32_jval_t *val) {
    const char *p = json, *end = json + len;
    size_t klen = strlen(key);
    memset(val, 0, sizeof *val);
    p = skip_ws(p, end);
    if (p >= end || *p != '{') return -1;
    p++;
    for (;;) {
        p = skip_ws(p, end);
        if (p >= end) return -1;
        if (*p == '}') return 0;
        if (*p == ',') { p++; continue; }
        if (*p != '"') return -1;
        const char *ks = p + 1, *ke = skip_string(p, end);
        if (!ke) return -1;
        size_t kl = (size_t)(ke - 1 - ks);
        p = skip_ws(ke, end);
        if (p >= end || *p != ':') return -1;
        p = skip_ws(p + 1, end);
        const char *vs = p, *ve = skip_value(p, end);
        if (!ve) return -1;
        if (kl == klen && memcmp(ks, key, klen) == 0) {
            val->raw = vs; val->len = (size_t)(ve - vs);
            if (*vs == '"') { val->type = C32_J_STR; val->raw = vs + 1; val->len = (size_t)(ve - vs - 2); }
            else if (*vs == '{') val->type = C32_J_OBJ;
            else if (*vs == '[') val->type = C32_J_ARR;
            else if (val->len == 4 && memcmp(vs, "true", 4) == 0) { val->type = C32_J_BOOL; val->num = 1; }
            else if (val->len == 5 && memcmp(vs, "false", 5) == 0) { val->type = C32_J_BOOL; val->num = 0; }
            else if (val->len == 4 && memcmp(vs, "null", 4) == 0) val->type = C32_J_NULL;
            else { val->type = C32_J_NUM; val->num = strtod(vs, NULL); }
            return 1;
        }
        p = ve;
    }
}

int c32_json_str(const c32_jval_t *val, char *out, size_t cap) {
    if (!val || val->type != C32_J_STR || cap == 0) return -1;
    size_t o = 0;
    for (size_t i = 0; i < val->len; i++) {
        char c = val->raw[i];
        if (c == '\\' && i + 1 < val->len) {
            i++;
            switch (val->raw[i]) {
            case 'n': c = '\n'; break;
            case 'r': c = '\r'; break;
            case 't': c = '\t'; break;
            case 'u':
                if (i + 4 < val->len) { unsigned code = (unsigned)strtoul((char[]){val->raw[i+1], val->raw[i+2], val->raw[i+3], val->raw[i+4], 0}, NULL, 16); i += 4; c = code < 0x80 ? (char)code : '?'; }
                break;
            default: c = val->raw[i];
            }
        }
        if (o + 1 >= cap) { out[o] = 0; return -1; }
        out[o++] = c;
    }
    out[o] = 0;
    return (int)o;
}

static int get_str(const char *json, size_t len, const char *key, char *out, size_t cap) {
    c32_jval_t v;
    if (c32_json_get(json, len, key, &v) != 1 || v.type != C32_J_STR) { if (cap) out[0] = 0; return 0; }
    return c32_json_str(&v, out, cap) >= 0;
}

int c32_attr_num(const char *json, size_t len, const char *key, double *out) {
    c32_jval_t v;
    if (c32_json_get(json, len, key, &v) != 1 || (v.type != C32_J_NUM && v.type != C32_J_BOOL)) return 0;
    *out = v.num;
    return 1;
}
int c32_attr_bool(const char *json, size_t len, const char *key, int *out) {
    c32_jval_t v;
    if (c32_json_get(json, len, key, &v) != 1) return 0;
    if (v.type == C32_J_BOOL || v.type == C32_J_NUM) { *out = v.num != 0; return 1; }
    return 0;
}

int c32_attr_request_json(char *out, size_t cap, const char *const *keys, size_t nkeys) {
    w_t w; w_init(&w, out, cap);
    if (!keys || nkeys == 0) { w_puts(&w, "{}"); return w_done(&w); }
    w_puts(&w, "{\"keys\":[");
    for (size_t i = 0; i < nkeys; i++) { if (i) w_putc(&w, ','); w_str(&w, keys[i]); }
    w_puts(&w, "]}");
    return w_done(&w);
}

/* ---------------------------------------------------------------- commands ---- */
int c32_cmd_parse(const char *json, size_t len, c32_cmd_t *cmd) {
    c32_jval_t v;
    memset(cmd, 0, sizeof *cmd);
    if (!get_str(json, len, "id", cmd->id, sizeof cmd->id) || !cmd->id[0]) return -1;
    if (!get_str(json, len, "m", cmd->method, sizeof cmd->method) || !cmd->method[0]) return -1;
    if (c32_json_get(json, len, "a", &v) == 1 && (v.type == C32_J_OBJ || v.type == C32_J_ARR)) { cmd->args = v.raw; cmd->args_len = v.len; }
    if (c32_json_get(json, len, "exp", &v) == 1 && v.type == C32_J_NUM) cmd->exp = (int64_t)v.num;
    return 0;
}

int c32_ack_json(char *out, size_t cap, const char *id, int ok, const char *result_json, const char *err) {
    w_t w; w_init(&w, out, cap);
    w_puts(&w, "{\"id\":"); w_str(&w, id);
    if (ok) {
        w_puts(&w, ",\"ok\":true");
        if (result_json && *result_json) { w_puts(&w, ",\"r\":"); w_puts(&w, result_json); }
    } else {
        w_puts(&w, ",\"ok\":false,\"err\":"); w_str(&w, err ? err : "error");
    }
    w_putc(&w, '}');
    return w_done(&w);
}

/* ---------------------------------------------------------------- console ---- */
int c32_console_in_parse(const char *json, size_t len, char *session, size_t scap, char *line, size_t lcap) {
    if (!get_str(json, len, "s", session, scap) || !session[0]) return -1;
    if (!get_str(json, len, "l", line, lcap)) return -1;
    return 0;
}
int c32_console_out_json(char *out, size_t cap, const char *session, int64_t t, const char *line) {
    w_t w; w_init(&w, out, cap);
    w_puts(&w, "{\"s\":"); w_str(&w, session);
    if (t) { w_puts(&w, ",\"t\":"); w_i64(&w, t); }
    w_puts(&w, ",\"l\":"); w_str(&w, line);
    w_putc(&w, '}');
    return w_done(&w);
}

/* ---------------------------------------------------------------- FOTA ---- */
int c32_fota_target_parse(const char *json, size_t len, c32_fota_target_t *t) {
    c32_jval_t v;
    memset(t, 0, sizeof *t);
    const char *p = skip_ws(json, json + len);
    if (p >= json + len) return 0;                     /* empty retained payload = target cleared */
    if (!get_str(json, len, "fw_id", t->fw_id, sizeof t->fw_id) || !t->fw_id[0]) return -1;
    get_str(json, len, "ver", t->ver, sizeof t->ver);
    get_str(json, len, "hw", t->hw, sizeof t->hw);
    get_str(json, len, "min_ver", t->min_ver, sizeof t->min_ver);
    get_str(json, len, "sha256", t->sha256, sizeof t->sha256);
    get_str(json, len, "url", t->url, sizeof t->url);
    get_str(json, len, "campaign", t->campaign, sizeof t->campaign);
    if (c32_json_get(json, len, "size", &v) == 1 && v.type == C32_J_NUM) t->size = (uint32_t)v.num;
    if (!t->url[0] || !t->sha256[0] || t->size == 0) return -1;
    return 1;
}

int c32_fota_status_json(char *out, size_t cap, const char *fw_id, const char *state, int pct, const char *ver, const char *err) {
    w_t w; w_init(&w, out, cap);
    w_puts(&w, "{\"fw_id\":"); w_str(&w, fw_id);
    w_puts(&w, ",\"st\":"); w_str(&w, state);
    if (pct >= 0) { w_puts(&w, ",\"pct\":"); w_num(&w, pct); }
    if (ver && *ver) { w_puts(&w, ",\"ver\":"); w_str(&w, ver); }
    if (err && *err) { w_puts(&w, ",\"err\":"); w_str(&w, err); }
    w_putc(&w, '}');
    return w_done(&w);
}

int c32_version_cmp(const char *a, const char *b) {
    while (*a || *b) {
        if (isdigit((unsigned char)*a) && isdigit((unsigned char)*b)) {
            long x = strtol(a, (char **)&a, 10), y = strtol(b, (char **)&b, 10);
            if (x != y) return x < y ? -1 : 1;
        } else {
            if (*a != *b) return (unsigned char)*a < (unsigned char)*b ? -1 : 1;
            if (*a) a++;
            if (*b) b++;
        }
    }
    return 0;
}

int c32_sha256_hex_equal(const char *a, const char *b) {
    if (!a || !b) return 0;
    size_t la = strlen(a), lb = strlen(b);
    if (la != 64 || lb != 64) return 0;
    unsigned diff = 0;
    for (size_t i = 0; i < 64; i++) diff |= (unsigned)(tolower((unsigned char)a[i]) ^ tolower((unsigned char)b[i]));
    return diff == 0;
}

/* ---------------------------------------------------------------- topics ---- */
int c32_topic(char *out, size_t cap, const char *device_id, const char *suffix) {
    int n = snprintf(out, cap, "d/%s/%s", device_id, suffix);
    return (n < 0 || (size_t)n >= cap) ? -1 : n;
}
const char *c32_topic_suffix(const char *topic, const char *device_id) {
    size_t idl = strlen(device_id);
    if (strncmp(topic, "d/", 2) != 0) return NULL;
    if (strncmp(topic + 2, device_id, idl) != 0 || topic[2 + idl] != '/') return NULL;
    return topic + 3 + idl;
}

/* ---------------------------------------------------------------- ring buffer ---- */
void c32_ring_init(c32_ring_t *r, c32_sample_t *slots, size_t cap) { r->slots = slots; r->cap = cap; r->head = 0; r->count = 0; r->dropped = 0; }
void c32_ring_push(c32_ring_t *r, const c32_sample_t *s) {
    if (!r->cap) return;
    size_t idx = (r->head + r->count) % r->cap;
    if (r->count == r->cap) { r->head = (r->head + 1) % r->cap; r->dropped++; idx = (r->head + r->cap - 1) % r->cap; }
    else r->count++;
    r->slots[idx] = *s;
    r->slots[idx].backfilled = 1;
    for (uint8_t i = 0; i < r->slots[idx].n; i++) if (r->slots[idx].v[i].type == C32_STR) r->slots[idx].v[i].str = NULL; /* strings are not owned */
}
size_t c32_ring_count(const c32_ring_t *r) { return r->count; }
int c32_ring_encode_batch(const c32_ring_t *r, char *out, size_t cap, size_t max_batch, size_t *taken) {
    w_t w; w_init(&w, out, cap);
    size_t n = r->count < max_batch ? r->count : max_batch;
    *taken = 0;
    if (n == 0) return -1;
    w_putc(&w, '[');
    for (size_t i = 0; i < n; i++) {
        size_t mark = w.len;
        if (i) w_putc(&w, ',');
        w_sample(&w, &r->slots[(r->head + i) % r->cap]);
        if (w.overflow) { /* roll back to the last complete sample */
            if (*taken == 0) return -1;
            w.len = mark; w.p[w.len] = 0; w.overflow = 0;
            break;
        }
        (*taken)++;
    }
    w_putc(&w, ']');
    return w_done(&w);
}
void c32_ring_drop(c32_ring_t *r, size_t n) {
    if (n > r->count) n = r->count;
    r->head = (r->head + n) % r->cap;
    r->count -= n;
}
