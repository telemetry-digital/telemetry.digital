# ESP32 a Raspberry Pi: poslať prvú telemetriu na telemetry.digital

Stav: 24. 9. 2026, ctrl32 telemetry 0.13.x, telemetry.digital je produkčná inštancia (bez verejného čítania,
MFA povinné, MQTT zvonku len cez TLS na porte 8883).

Cieľ: do 30 minút mať na Devices dve zariadenia **online** a v datastreamoch prvé uložené hodnoty.

## 1. Čo treba vedieť o protokole (stačí toto)

| Parameter | Hodnota |
|---|---|
| Broker | `telemetry.digital`, port **8883**, TLS (certifikát Let's Encrypt, koreň ISRG Root X1) |
| Client ID | external id zariadenia (napr. `esp32-a`) |
| Username | to isté external id |
| Password | token zariadenia `c32d_…` (z UI, zobrazí sa raz) |
| Téma pre merania | `d/<external id>/telemetry` |
| Payload | `{"ts": 1758700000, "v": {"t_c": 21.5, "rh": 40.2}}` — `ts` v sekundách UTC (voliteľné, bez neho platí čas servera); dávka = pole objektov, dobehnuté vzorky s `"bf": 1` |
| Kľúče | `^[a-z][a-z0-9_]{0,31}$` (malé písmená, číslice, podčiarkovník); musia byť priradené k datastreamom, inak sa správa uloží ako surová so stavom `unknown_key` a do datastreamu nič nepríde |
| Stav (heartbeat) | `d/<id>/status` — `{"online": true, "rssi": -61, "up_s": 3600, "free": 120000, "fw": "1.0.0"}`; LWT `{"online": false}` na tej istej téme |
| Keep-alive | 60 s |

Zariadenie smie publikovať len do vlastných tém `d/<id>/…`; iné témy broker odmietne.

## 2. Príprava v UI (raz, 5 minút)

1. Prihlás sa (nové heslo a autentifikátor sa vyžiadajú pri prvom prihlásení).
2. **Assets → Sites → New site**: `Stôl`.
3. **Assets → Assets → New asset**: `ESP32 A` a `Raspberry Pi` (site Stôl).
4. **Datastreams** (v detaile assetu → New datastream), pre každý signál kľúč, veličina, jednotka, očakávaný interval:
   - ESP32 A: `t_c` (teplota, °C, 60 s), `rh` (vlhkosť, %, 60 s)
   - Raspberry Pi: `cpu_c` (teplota CPU, °C, 30 s), `load1` (zaťaženie, –, 30 s), voliteľne `t_c` z DS18B20
5. **Devices → New device**: external id `esp32-a`, transport `mqtt` → v detaile **Issue / rotate token** → skopíruj `c32d_…` (zobrazí sa raz). To isté pre `rpi-1`.
6. V detaile zariadenia → záložka **Datastreams** → priraď kľúč zariadenia (`t_c`, `rh` …) k datastreamu assetu. Bez tohto kroku sa hodnoty neukladajú do datastreamov.

## 3. Raspberry Pi (Python, 10 minút)

Raspberry Pi OS (Bookworm alebo novší), pripojené na internet.

```bash
sudo apt-get install -y python3-paho-mqtt || pip3 install --break-system-packages paho-mqtt
sudo mkdir -p /opt/ctrl32 && sudo nano /opt/ctrl32/telemetry.py
```

`/opt/ctrl32/telemetry.py` (doplň `DEVICE_ID` a `TOKEN`):

```python
#!/usr/bin/env python3
# ctrl32 telemetry — Raspberry Pi publisher: CPU temperature, load, optional DS18B20, heartbeat. TLS 8883.
import json, os, ssl, time, glob
import paho.mqtt.client as mqtt

HOST = "telemetry.digital"; PORT = 8883
DEVICE_ID = os.environ.get("CTRL32_DEVICE", "rpi-1")
TOKEN = os.environ.get("CTRL32_TOKEN", "c32d_PASTE_TOKEN_HERE")
EVERY = int(os.environ.get("CTRL32_EVERY", "30"))       # seconds between samples
FW = "rpi-telemetry 1.0"

def cpu_temp():
    with open("/sys/class/thermal/thermal_zone0/temp") as f:
        return round(int(f.read().strip()) / 1000.0, 1)

def ds18b20():
    for p in glob.glob("/sys/bus/w1/devices/28-*/w1_slave"):
        with open(p) as f:
            raw = f.read()
        if "YES" in raw and "t=" in raw:
            return round(int(raw.split("t=")[1]) / 1000.0, 2)
    return None

def make_client():
    try:  # paho-mqtt 2.x
        c = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=DEVICE_ID, protocol=mqtt.MQTTv311)
    except AttributeError:  # paho-mqtt 1.x
        c = mqtt.Client(client_id=DEVICE_ID, protocol=mqtt.MQTTv311)
    c.username_pw_set(DEVICE_ID, TOKEN)
    c.tls_set(cert_reqs=ssl.CERT_REQUIRED)             # system CA store (Let's Encrypt is trusted)
    c.will_set(f"d/{DEVICE_ID}/status", json.dumps({"online": False}), qos=1, retain=False)
    c.reconnect_delay_set(min_delay=2, max_delay=60)
    return c

buffer = []                                            # samples kept while offline, sent as backfill

def main():
    c = make_client()
    c.connect(HOST, PORT, keepalive=60)
    c.loop_start()
    started = time.time()
    while True:
        v = {"cpu_c": cpu_temp(), "load1": round(os.getloadavg()[0], 2)}
        t = ds18b20()
        if t is not None:
            v["t_c"] = t
        sample = {"ts": int(time.time()), "v": v}
        if c.is_connected():
            if buffer:
                for s in buffer:
                    s["bf"] = 1
                c.publish(f"d/{DEVICE_ID}/telemetry", json.dumps(buffer), qos=1)
                buffer.clear()
            c.publish(f"d/{DEVICE_ID}/telemetry", json.dumps(sample), qos=1)
            c.publish(f"d/{DEVICE_ID}/status", json.dumps({"online": True, "up_s": int(time.time() - started), "fw": FW}), qos=0)
        else:
            buffer.append(sample)
            del buffer[:-720]                          # keep the last 6 hours at 30 s
        time.sleep(EVERY)

if __name__ == "__main__":
    main()
```

Spustenie na skúšku a potom ako služba:

```bash
CTRL32_DEVICE=rpi-1 CTRL32_TOKEN=c32d_... python3 /opt/ctrl32/telemetry.py
sudo tee /etc/systemd/system/ctrl32-rpi.service >/dev/null <<'EOF'
[Unit]
Description=ctrl32 telemetry publisher
After=network-online.target
Wants=network-online.target
[Service]
Environment=CTRL32_DEVICE=rpi-1
Environment=CTRL32_TOKEN=c32d_PASTE_TOKEN_HERE
ExecStart=/usr/bin/python3 /opt/ctrl32/telemetry.py
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload && sudo systemctl enable --now ctrl32-rpi && journalctl -u ctrl32-rpi -f
```

DS18B20 na Pi: `dtoverlay=w1-gpio` v `/boot/firmware/config.txt`, dátový vodič na GPIO4, 4,7 kΩ pull-up na 3,3 V, reštart.

## 4. ESP32 (Arduino / PlatformIO, 15 minút)

1. Repo `ctrl32-telemetry-c`, príklad `examples/arduino/fridge_node/fridge_node.ino` (PlatformIO: `pio run -d ci -t upload` s doskou `esp32-s3-devkitc-1`, alebo Arduino IDE 2.x s knižnicou PubSubClient a súbormi z `ports/arduino` + `src`).
2. V súbore nastav:
   - `WIFI_SSID`, `WIFI_PASS` (2,4 GHz),
   - `SERVER = "telemetry.digital"`, `DEVICE_ID = "esp32-a"`, `TOKEN = "c32d_…"`,
   - `c.port = 8883; c.tls = true; c.insecure = false; c.caCert = ROOT_CA_PEM;` a `ROOT_CA_PEM` = ISRG Root X1 (nižšie),
   - interval publikovania 60 s.
3. Čas: pre overenie TLS certifikátu musí mať ESP32 správny čas — knižnica po pripojení na Wi‑Fi volá NTP (`pool.ntp.org`); ak je sieť bez internetového NTP, nastav vlastný NTP server v sketchi.
4. Senzor SHT40 na I²C (SDA GPIO8, SCL GPIO9 na S3-DevKitC-1; pri inej doske podľa pinoutu); DS18B20 na GPIO4 s 4,7 kΩ.
5. Nahraj, sleduj sériový monitor: `connected`, potom každú minútu `published`.

ISRG Root X1 (Let's Encrypt), vlož ako `ROOT_CA_PEM`:

```
-----BEGIN CERTIFICATE-----
MIIFazCCA1OgAwIBAgIRAIIQz7DSQONZRGPgu2OCiwAwDQYJKoZIhvcNAQELBQAw
TzELMAkGA1UEBhMCVVMxKTAnBgNVBAoTIEludGVybmV0IFNlY3VyaXR5IFJlc2Vh
cmNoIEdyb3VwMRUwEwYDVQQDEwxJU1JHIFJvb3QgWDEwHhcNMTUwNjA0MTEwNDM4
WhcNMzUwNjA0MTEwNDM4WjBPMQswCQYDVQQGEwJVUzEpMCcGA1UEChMgSW50ZXJu
ZXQgU2VjdXJpdHkgUmVzZWFyY2ggR3JvdXAxFTATBgNVBAMTDElTUkcgUm9vdCBY
MTCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAK3oJHP0FDfzm54rVygc
h77ct984kIxuPOZXoHj3dcKi/vVqbvYATyjb3miGbESTtrFj/RQSa78f0uoxmyF+
0TM8ukj13Xnfs7j/EvEhmkvBioZxaUpmZmyPfjxwv60pIgbz5MDmgK7iS4+3mX6U
A5/TR5d8mUgjU+g4rk8Kb4Mu0UlXjIB0ttov0DiNewNwIRt18jA8+o+u3dpjq+sW
T8KOEUt+zwvo/7V3LvSye0rgTBIlDHCNAymg4VMk7BPZ7hm/ELNKjD+Jo2FR3qyH
B5T0Y3HsLuJvW5iB4YlcNHlsdu87kGJ55tukmi8mxdAQ4Q7e2RCOFvu396j3x+UC
B5iPNgiV5+I3lg02dZ77DnKxHZu8A/lJBdiB3QW0KtZB6awBdpUKD9jf1b0SHzUv
KBds0pjBqAlkd25HN7rOrFleaJ1/ctaJxQZBKT5ZPt0m9STJEadao0xAH0ahmbWn
OlFuhjuefXKnEgV4We0+UXgVCwOPjdAvBbI+e0ocS3MFEvzG6uBQE3xDk3SzynTn
jh8BCNAw1FtxNrQHusEwMFxIt4I7mKZ9YIqioymCzLq9gwQbooMDQaHWBfEbwrbw
qHyGO0aoSCqI3Haadr8faqU9GY/rOPNk3sgrDQoo//fb4hVC1CLQJ13hef4Y53CI
rU7m2Ys6xt0nUW7/vGT1M0NPAgMBAAGjQjBAMA4GA1UdDwEB/wQEAwIBBjAPBgNV
HRMBAf8EBTADAQH/MB0GA1UdDgQWBBR5tFnme7bl5AFzgAiIyBpY9umbbjANBgkq
hkiG9w0BAQsFAAOCAgEAVR9YqbyyqFDQDLHYGmkgJykIrGF1XIpu+ILlaS/V9lZL
ubhzEFnTIZd+50xx+7LSYK05qAvqFyFWhfFQDlnrzuBZ6brJFe+GnY+EgPbk6ZGQ
3BebYhtF8GaV0nxvwuo77x/Py9auJ/GpsMiu/X1+mvoiBOv/2X/qkSsisRcOj/KK
NFtY2PwByVS5uCbMiogziUwthDyC3+6WVwW6LLv3xLfHTjuCvjHIInNzktHCgKQ5
ORAzI4JMPJ+GslWYHb4phowim57iaztXOoJwTdwJx4nLCgdNbOhdjsnvzqvHu7Ur
TkXWStAmzOVyyghqpZXjFaH3pO3JLF+l+/+sKAIuvtd7u+Nxe5AW0wdeRlN8NwdC
jNPElpzVmbUq4JUagEiuTDkHzsxHpFKVK7q4+63SM1N95R1NbdWhscdCb+ZAJzVc
oyi3B43njTOQ5yOf+1CceWxG1bQVs5ZufpsMljq4Ui0/1lvh+wjChP4kqKOJ2qxq
4RgqsahDYVvTH9w7jXbyLeiNdd8XM2w9U/t7y0Ff/9yi0GE44Za4rF2LN9d11TPA
mRGunUHBcnWEvgJBQl9nJEiU0Zsnvgc/ubhPgXRR4Xq37Z0j4r7g1SgEEzwxA57d
emyPxgcYxn/eR44/KJ4EBs+lVDR3veyJm+kXQ99b21/+jh5Xos1AnX5iItreGCc=
-----END CERTIFICATE-----
```

## 5. Overenie, že sa ukladá

1. **Devices**: obe zariadenia `online` (živé MQTT spojenie), stĺpec Last seen sa hýbe.
2. Detail zariadenia → **Messages**: každá správa s `decode_status = ok`; ak vidíš `unknown_key`, kľúč nie je priradený (krok 2.6).
3. Detail assetu → datastream: posledná hodnota a čas; **Dashboard** ukazuje počty a posledné hodnoty.
4. Odpoj Pi od siete na 3 minúty a pripoj späť: v Messages príde dávka s `bf: 1` a datastream nemá dieru (Raspberry skript má buffer); ESP32 knižnica buffruje rovnako.
5. **Exports**: CSV za posledných 6 hodín obsahuje riadky oboch zariadení; PDF report z observations.

## 6. Keď to nejde

| Prejav | Príčina | Riešenie |
|---|---|---|
| `not authorised` / rc=5 pri connect | zlé external id alebo token, alebo token zrušený | Devices → Issue / rotate token, prepíš do zariadenia |
| TLS chyba na ESP32 (`certificate verify failed`, `-0x2700`) | nesynchronizovaný čas alebo chýba koreňový certifikát | počkať na NTP, skontrolovať `ROOT_CA_PEM`; na skúšku dočasne `c.insecure = true` |
| online, ale datastream prázdny | kľúče nepriradené alebo iný názov kľúča | Messages → `unknown_key`; priradiť v detaile zariadenia |
| správy odmietnuté (`rate`, `too_large`) | príliš časté alebo veľké publikovanie | interval ≥ 10 s, dávka ≤ 64 KiB |
| ESP32 sa nepripojí na Wi‑Fi | 5 GHz sieť alebo WPA3-only | 2,4 GHz SSID, WPA2 |
| Pi hlási `ssl.SSLCertVerificationError` | staré CA na Pi | `sudo apt-get install -y ca-certificates && sudo update-ca-certificates` |

Poznámky: token drž mimo repozitára (env premenná / `Preferences` v ESP32); jedno zariadenie = jeden token; po výmene tokenu staré spojenie padne.
