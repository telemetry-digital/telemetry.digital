# Builds the presentation dashboards of the ctrl32 wall (p4-nano, f7-disco, reterminal) from the real datastreams
# (screen format v1): creates them, or publishes a new version when the slug exists. Needs an API token with
# data.read + content.write in a file (never in the repository).
#   python tools/walldash/wall_dashboards.py [--base URL] [--token-file path] [--only slug,slug] [--dry]
import argparse, io, json, sys, urllib.request, urllib.error

import os
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'wall-dashboards.json')
ap = argparse.ArgumentParser()
ap.add_argument('--base', default='https://telemetry.digital')
ap.add_argument('--token-file', default='D:/Ai/projekty/ctrl32/build/api_token.txt')
ap.add_argument('--dry', action='store_true')
ap.add_argument('--only', default='', help='comma list of slugs')
a = ap.parse_args()
TOKEN = io.open(a.token_file, encoding='utf-8').read().strip()

def call(method, path, body=None, ok=(200, 201)):
    data = json.dumps(body).encode('utf-8') if body is not None else None
    req = urllib.request.Request(a.base + path, data=data, method=method)
    req.add_header('Authorization', 'Bearer ' + TOKEN)
    if data is not None:
        req.add_header('Content-Type', 'application/json')
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            raw = r.read().decode('utf-8')
            return r.status, (json.loads(raw) if raw.strip() else {})
    except urllib.error.HTTPError as e:
        raw = e.read().decode('utf-8', 'replace')
        try:
            return e.code, json.loads(raw)
        except Exception:
            return e.code, {'error': raw[:300]}

st, ds = call('GET', '/api/v1/datastreams')
if st != 200:
    sys.exit(f'datastreams: {st} {ds}')
st, devs = call('GET', '/api/v1/devices')
ASSET = {'p4': 'P4 home dashboard', 'f7': 'F7 Discovery', 'rt': 'reTerminal (RS-485 master, i550 EtherCAT)'}
DEVICE = {'p4': 'p4-nano', 'f7': 'f7-disco', 'rt': 'reterminal'}
DS = {}
for d in ds:
    for k, name in ASSET.items():
        if d['asset'] == name:
            DS[(k, d['key'])] = d['id']
DEV = {k: next(x['id'] for x in devs if x['external_id'] == ext) for k, ext in DEVICE.items()}
missing = []

def ids(*pairs):
    out = []
    for p in pairs:
        dev, key = p.split('.')
        i = DS.get((dev, key))
        if i is None:
            missing.append(p)
        else:
            out.append(i)
    return out

# ---- widget helpers -------------------------------------------------------------------------------
W = []
n = [0]
def add(type_, x, y, w, h, params=None, streams=(), devices=None):
    n[0] += 1
    b = {}
    if streams:
        b['datastreams'] = ids(*streams)
    if devices is not None:
        b['devices'] = devices
    W.append({'id': f'w{n[0]}', 'type': type_, 'geometry': {'x': x, 'y': y, 'w': w, 'h': h}, 'bindings': b, 'params': params or {}})

ON_OFF = ['0=Off:#94a3b8', '1=On:#059669']
OK_BAD = ['0=Error:#dc2626', '1=OK:#059669']
HEAT = ['0=Idle:#94a3b8', '1=Heating:#ea580c']
RUN = ['0=Stopped:#94a3b8', '1=Running:#059669']
FAULT = ['0=No fault:#059669', '1=Fault:#dc2626']
WARN = ['0=No warning:#059669', '1=Warning:#d97706']
LINK = ['0=Disconnected:#dc2626', '1=Connected:#059669']
DIR = ['0=Forward:#0284c7', '1=Reverse:#7c3aed']

def spec(kind='dashboard', range_='24h', refresh=15, layout=None):
    global W
    s = {'kind': kind, 'version': 1, 'layout': layout or {'type': 'grid', 'columns': 12, 'row_height': 96}, 'range': range_, 'refresh_s': refresh, 'widgets': W}
    W = []
    n[0] = 0
    return s

DASHBOARDS = []

# ---- 1. wall overview -----------------------------------------------------------------------------
add('text', 0, 0, 12, 1, {'title': 'ctrl32 wall', 'style': 'heading', 'text': 'Three real devices send live values to telemetry.digital: P4 (home and heating zones), reTerminal (i550 drive over EtherCAT, production, sensors, UPS) and F7 Discovery (PLC). Everything on this page is a real measurement — no demo data.'})
add('devices', 0, 1, 4, 3, {'title': 'Wall devices', 'limit': 10}, devices=[DEV['p4'], DEV['f7'], DEV['rt']])
add('alarms', 4, 1, 4, 3, {'title': 'Alarms', 'limit': 8})
add('table', 8, 1, 4, 3, {'title': 'Key values', 'decimals': 1}, ('p4.temp_a', 'p4.temp_b', 'p4.temp_c', 'rt.speed', 'rt.current', 'rt.pa_pieces', 'f7.plc_real'))
add('gauge', 0, 4, 2, 2, {'title': 'Zone A [°C]', 'decimals': 1, 'y_min': 0, 'y_max': 40, 'warn': 26, 'action': 30}, ('p4.temp_a',))
add('gauge', 2, 4, 2, 2, {'title': 'Zone B [°C]', 'decimals': 1, 'y_min': 0, 'y_max': 40, 'warn': 26, 'action': 30}, ('p4.temp_b',))
add('gauge', 4, 4, 2, 2, {'title': 'Zone C [°C]', 'decimals': 1, 'y_min': 0, 'y_max': 40, 'warn': 26, 'action': 30}, ('p4.temp_c',))
add('value', 6, 4, 2, 2, {'title': 'Drive — speed', 'decimals': 0, 'sparkline': True}, ('rt.speed',))
add('value', 8, 4, 2, 2, {'title': 'Drive — current', 'decimals': 2, 'sparkline': True}, ('rt.current',))
add('value', 10, 4, 2, 2, {'title': 'Production — pieces', 'decimals': 0, 'sparkline': True}, ('rt.pa_pieces',))
add('chart', 0, 6, 6, 3, {'title': 'P4 zone temperatures and setpoints (24 h)', 'chart': 'line', 'decimals': 1, 'band': False, 'legend': True}, ('p4.temp_a', 'p4.temp_b', 'p4.temp_c', 'p4.sp_a', 'p4.sp_b', 'p4.sp_c'))
add('chart', 6, 6, 6, 3, {'title': 'i550 drive: speed and setpoint (24 h)', 'chart': 'area', 'decimals': 0, 'band': False, 'legend': True}, ('rt.speed', 'rt.setpoint'))
add('status', 0, 9, 2, 1, {'title': 'P4 PLC', 'states': OK_BAD}, ('p4.plc_ok',))
add('status', 2, 9, 2, 1, {'title': 'P4 time valid', 'states': OK_BAD}, ('p4.sys_time_valid',))
add('status', 4, 9, 2, 1, {'title': 'F7 program running', 'states': RUN}, ('f7.run',))
add('status', 6, 9, 2, 1, {'title': 'Drive running', 'states': RUN}, ('rt.running',))
add('status', 8, 9, 2, 1, {'title': 'Drive fault', 'states': FAULT}, ('rt.fault',))
add('status', 10, 9, 2, 1, {'title': 'Production running', 'states': RUN}, ('rt.pa_running',))
add('bars', 0, 10, 4, 2, {'title': 'Runtime cycle [µs]', 'decimals': 0}, ('p4.sys_cycle_us', 'f7.sys_cycle_us', 'rt.sys_cycle_us'))
add('bars', 4, 10, 4, 2, {'title': 'Longest cycle [µs]', 'decimals': 0}, ('p4.sys_cycle_max_us', 'f7.sys_cycle_max_us', 'rt.sys_cycle_max_us'))
add('bars', 8, 10, 4, 2, {'title': 'Cycle overruns', 'decimals': 0, 'warn': 1}, ('p4.sys_overruns', 'f7.sys_overruns', 'rt.sys_overruns'))
add('map', 0, 12, 6, 4, {'title': 'Devices on the map', 'zoom': 7}, devices=[DEV['p4'], DEV['f7'], DEV['rt']])
add('heatmap', 6, 12, 6, 2, {'title': 'Zone A by hour (7 days)', 'range': '7d', 'decimals': 1}, ('p4.temp_a',))
add('chart', 6, 14, 6, 2, {'title': 'Light at the wall [lx]', 'chart': 'step', 'decimals': 0, 'band': False, 'legend': False}, ('rt.lux',))
add('text', 0, 16, 12, 1, {'title': 'Note', 'style': 'note', 'text': 'More screens: P4 — zones and home · reTerminal — drive · reTerminal — zones, production, sensors, UPS · F7 Discovery — PLC · System — runtime · Process picture of the zones and the drive.'})
DASHBOARDS.append(('wall-prehlad', 'ctrl32 wall — overview', spec(), True))

# ---- 2. P4 — zones and home ------------------------------------------------------------------------
add('text', 0, 0, 12, 1, {'title': 'P4 home dashboard', 'style': 'heading', 'text': 'Three heating zones with setpoints, efficiency and limits; home scenes; values from the Simatic PLC; net32 links; runtime state.'})
for i, z in enumerate('abc'):
    add('gauge', i * 4, 1, 2, 2, {'title': f'Zone {z.upper()} — temperature', 'decimals': 1, 'y_min': 0, 'y_max': 40, 'warn': 26, 'action': 30}, (f'p4.temp_{z}',))
    add('value', i * 4 + 2, 1, 2, 1, {'title': f'Setpoint {z.upper()}', 'decimals': 1, 'sparkline': False}, (f'p4.sp_{z}',))
    add('status', i * 4 + 2, 2, 2, 1, {'title': f'Heating {z.upper()}', 'states': HEAT}, (f'p4.heat_{z}',))
add('chart', 0, 3, 8, 3, {'title': 'Temperatures and setpoints (24 h)', 'chart': 'line', 'decimals': 1, 'band': False, 'legend': True}, ('p4.temp_a', 'p4.temp_b', 'p4.temp_c', 'p4.sp_a', 'p4.sp_b', 'p4.sp_c'))
add('bars', 8, 3, 4, 3, {'title': 'Zone efficiency', 'decimals': 1}, ('p4.eff_a', 'p4.eff_b', 'p4.eff_c'))
add('table', 0, 6, 6, 3, {'title': 'Zone limits (hi / lo) and night setpoint', 'decimals': 1}, ('p4.hi_a', 'p4.lo_a', 'p4.hi_b', 'p4.lo_b', 'p4.hi_c', 'p4.lo_c', 'p4.sp_night'))
add('status', 6, 6, 2, 1, {'title': 'Zone A warm', 'states': ON_OFF}, ('p4.warm_a',))
add('status', 8, 6, 2, 1, {'title': 'Zone B warm', 'states': ON_OFF}, ('p4.warm_b',))
add('status', 10, 6, 2, 1, {'title': 'Zone C warm', 'states': ON_OFF}, ('p4.warm_c',))
add('status', 6, 7, 2, 1, {'title': 'Night scene', 'states': ON_OFF}, ('p4.scene_night',))
add('status', 8, 7, 2, 1, {'title': 'Eco mode', 'states': ON_OFF}, ('p4.eco_mode',))
add('status', 10, 7, 2, 1, {'title': 'Heating boost', 'states': ON_OFF}, ('p4.heat_boost',))
add('status', 6, 8, 2, 1, {'title': 'Zones heartbeat', 'states': ON_OFF}, ('p4.z_beat',))
add('status', 8, 8, 2, 1, {'title': 'PLC OK', 'states': OK_BAD}, ('p4.plc_ok',))
add('status', 10, 8, 2, 1, {'title': 'PLC blink', 'states': ON_OFF}, ('p4.plc_blink',))
add('table', 0, 9, 6, 2, {'title': 'Values from the Simatic PLC (S7)', 'decimals': 2}, ('p4.plc_creal', 'p4.plc_cint', 'p4.plc_cdint', 'p4.plc_treal', 'p4.plc_bit_a'))
add('chart', 6, 9, 6, 2, {'title': 'S7: plc_creal and plc_treal (24 h)', 'chart': 'line', 'decimals': 2, 'band': False, 'legend': True}, ('p4.plc_creal', 'p4.plc_treal'))
links = [('link_zones', 'Link zones'), ('link_drive', 'Link drive'), ('link_house', 'Link house'), ('link_room', 'Link room'), ('link_util', 'Link util'), ('link_wall', 'Link wall')]
for i, (k, t) in enumerate(links):
    add('status', i * 2, 11, 2, 1, {'title': t, 'states': LINK}, (f'p4.{k}',))
add('status', 0, 12, 2, 1, {'title': 'Link energy', 'states': LINK}, ('p4.link_energy',))
add('status', 2, 12, 2, 1, {'title': 'Ethernet', 'states': LINK}, ('p4.sys_eth_ok',))
add('status', 4, 12, 2, 1, {'title': 'Wi-Fi', 'states': LINK}, ('p4.sys_wifi_ok',))
add('status', 6, 12, 2, 1, {'title': 'S7 connection', 'states': LINK}, ('p4.sys_s7_ok',))
add('status', 8, 12, 2, 1, {'title': 'Time valid', 'states': OK_BAD}, ('p4.sys_time_valid',))
add('value', 10, 12, 2, 1, {'title': 'net32 peers', 'decimals': 0, 'sparkline': False}, ('p4.sys_nvl_peers',))
add('value', 0, 13, 3, 2, {'title': 'Uptime [s]', 'decimals': 0, 'sparkline': False}, ('p4.sys_uptime_s',))
add('value', 3, 13, 3, 2, {'title': 'Cycle [µs]', 'decimals': 0, 'sparkline': True}, ('p4.sys_cycle_us',))
add('value', 6, 13, 3, 2, {'title': 'Longest cycle [µs]', 'decimals': 0, 'sparkline': True}, ('p4.sys_cycle_max_us',))
add('value', 9, 13, 3, 2, {'title': 'Overruns', 'decimals': 0, 'sparkline': False, 'warn': 1}, ('p4.sys_overruns',))
DASHBOARDS.append(('p4-zony', 'P4 — zones and home', spec(), False))

# ---- 3. reTerminal — pohon ------------------------------------------------------------------------
add('text', 0, 0, 12, 1, {'title': 'reTerminal — Lenze i550 drive over EtherCAT', 'style': 'heading', 'text': 'Speed, setpoint, current, torque, DC link voltage, frequency; status word, faults, direction; ramps.'})
add('gauge', 0, 1, 3, 2, {'title': 'Speed [rpm]', 'decimals': 0, 'y_min': 0, 'y_max': 3000}, ('rt.speed',))
add('gauge', 3, 1, 3, 2, {'title': 'Current [A]', 'decimals': 2, 'y_min': 0, 'y_max': 10, 'warn': 6, 'action': 8}, ('rt.current',))
add('gauge', 6, 1, 3, 2, {'title': 'Torque [Nm]', 'decimals': 2, 'y_min': 0, 'y_max': 10}, ('rt.torque',))
add('gauge', 9, 1, 3, 2, {'title': 'DC voltage [V]', 'decimals': 0, 'y_min': 0, 'y_max': 800, 'warn': 700, 'action': 760}, ('rt.dc_voltage',))
add('status', 0, 3, 2, 1, {'title': 'Running', 'states': RUN}, ('rt.running',))
add('status', 2, 3, 2, 1, {'title': 'Ready', 'states': ['0=Not ready:#94a3b8', '1=Ready:#059669']}, ('rt.ready',))
add('status', 4, 3, 2, 1, {'title': 'Fault', 'states': FAULT}, ('rt.fault',))
add('status', 6, 3, 2, 1, {'title': 'Warning', 'states': WARN}, ('rt.warning',))
add('status', 8, 3, 2, 1, {'title': 'Direction', 'states': DIR}, ('rt.direction',))
add('status', 10, 3, 2, 1, {'title': 'Command', 'states': ['0=Stop:#94a3b8', '1=Start:#059669']}, ('rt.command',))
add('chart', 0, 4, 8, 3, {'title': 'Speed and setpoint (24 h)', 'chart': 'line', 'decimals': 0, 'band': False, 'legend': True}, ('rt.speed', 'rt.setpoint'))
add('chart', 8, 4, 4, 3, {'title': 'Current and torque (24 h)', 'chart': 'area', 'decimals': 2, 'band': False, 'legend': True}, ('rt.current', 'rt.torque'))
add('value', 0, 7, 3, 2, {'title': 'Frequency [Hz]', 'decimals': 1, 'sparkline': True}, ('rt.frequency',))
add('value', 3, 7, 3, 2, {'title': 'Setpoint [rpm]', 'decimals': 0, 'sparkline': False}, ('rt.setpoint',))
add('value', 6, 7, 3, 2, {'title': 'Acceleration ramp [s]', 'decimals': 1, 'sparkline': False}, ('rt.acc',))
add('value', 9, 7, 3, 2, {'title': 'Deceleration ramp [s]', 'decimals': 1, 'sparkline': False}, ('rt.dec',))
add('table', 0, 9, 6, 2, {'title': 'Status word and error code', 'decimals': 0}, ('rt.drive_sw', 'rt.error_code'))
add('chart', 6, 9, 6, 2, {'title': 'DC link voltage (24 h)', 'chart': 'line', 'decimals': 0, 'band': False, 'legend': False}, ('rt.dc_voltage',))
add('text', 0, 11, 12, 1, {'title': 'Note', 'style': 'note', 'text': 'The gauge ranges (3000 rpm, 10 A, 10 Nm, 800 V) are defaults; adjust them in the editor to the drive nameplate.'})
DASHBOARDS.append(('reterminal-pohon', 'reTerminal — i550 drive', spec(), False))

# ---- 4. reTerminal — zones, production, sensors, UPS ---------------------------------------------------
add('text', 0, 0, 12, 1, {'title': 'reTerminal — zones, production, sensors and UPS', 'style': 'heading', 'text': 'Three temperature zones with heating, a production piece counter, accelerometer, light, tilt and shock, power supply and the UPS battery.'})
for i, z in enumerate('abc'):
    add('gauge', i * 4, 1, 2, 2, {'title': f'Zone {z.upper()} [°C]', 'decimals': 1, 'y_min': 0, 'y_max': 60, 'warn': 40, 'action': 50}, (f'rt.z_temp_{z}',))
    add('value', i * 4 + 2, 1, 2, 1, {'title': f'Setpoint {z.upper()}', 'decimals': 1, 'sparkline': False}, (f'rt.z_sp_{z}',))
    add('status', i * 4 + 2, 2, 2, 1, {'title': f'Heating {z.upper()}', 'states': HEAT}, (f'rt.z_heat_{z}',))
add('chart', 0, 3, 8, 3, {'title': 'Zones: temperatures and setpoints (24 h)', 'chart': 'line', 'decimals': 1, 'band': False, 'legend': True}, ('rt.z_temp_a', 'rt.z_temp_b', 'rt.z_temp_c', 'rt.z_sp_a', 'rt.z_sp_b', 'rt.z_sp_c'))
add('value', 8, 3, 2, 1, {'title': 'Zone average', 'decimals': 1, 'sparkline': True}, ('rt.avg_temp',))
add('value', 10, 3, 2, 1, {'title': 'Limit', 'decimals': 1, 'sparkline': False}, ('rt.limit',))
add('status', 8, 4, 4, 1, {'title': 'Overheating', 'states': ['0=Normal:#059669', '1=Hot:#dc2626']}, ('rt.hot',))
add('value', 8, 5, 2, 1, {'title': 'Pieces', 'decimals': 0, 'sparkline': True}, ('rt.pa_pieces',))
add('status', 10, 5, 2, 1, {'title': 'Production', 'states': RUN}, ('rt.pa_running',))
add('chart', 0, 6, 6, 3, {'title': 'Accelerometer X / Y / Z [g] (6 h)', 'chart': 'line', 'range': '6h', 'decimals': 3, 'band': False, 'legend': True}, ('rt.ax', 'rt.ay', 'rt.az'))
add('chart', 6, 6, 6, 3, {'title': 'Production pieces (7 days)', 'chart': 'bar', 'range': '7d', 'decimals': 0, 'band': False, 'legend': False}, ('rt.pa_pieces',))
add('value', 0, 9, 3, 2, {'title': 'Light [lx]', 'decimals': 0, 'sparkline': True}, ('rt.lux',))
add('value', 3, 9, 3, 2, {'title': 'Tilt [°]', 'decimals': 1, 'sparkline': True, 'warn': 10, 'action': 20}, ('rt.sys_tilt',))
add('value', 6, 9, 3, 2, {'title': 'Shock [g]', 'decimals': 2, 'sparkline': True, 'warn': 1, 'action': 2}, ('rt.sys_shock_g',))
add('value', 9, 9, 3, 2, {'title': 'Acceleration Z [g]', 'decimals': 3, 'sparkline': True}, ('rt.sys_accel_z',))
add('text', 0, 11, 12, 1, {'title': 'UPS and power supply', 'style': 'heading', 'text': 'Bus and battery voltage, battery current, chip temperature, UPS state and the runtime power state.'})
add('gauge', 0, 12, 3, 2, {'title': 'Bus [V]', 'decimals': 2, 'y_min': 0, 'y_max': 6, 'warn': 5.5}, ('rt.ups_vbus',))
add('gauge', 3, 12, 3, 2, {'title': 'Battery [V]', 'decimals': 2, 'y_min': 2.5, 'y_max': 4.5}, ('rt.ups_vbat',))
add('value', 6, 12, 3, 2, {'title': 'Battery current [A]', 'decimals': 3, 'sparkline': True}, ('rt.ups_ibat',))
add('value', 9, 12, 3, 2, {'title': 'UPS temperature [°C]', 'decimals': 1, 'sparkline': True, 'warn': 60, 'action': 70}, ('rt.ups_tdie',))
add('status', 0, 14, 3, 1, {'title': 'UPS state', 'states': ['0=Off:#94a3b8', '1=Mains:#059669', '2=Battery:#d97706', '3=Charging:#0284c7', '4=Error:#dc2626']}, ('rt.ups_state',))
add('status', 3, 14, 3, 1, {'title': 'Battery present', 'states': ['0=No battery:#dc2626', '1=Present:#059669']}, ('rt.sys_battery_present',))
add('status', 6, 14, 3, 1, {'title': 'On battery', 'states': ['0=Mains:#059669', '1=On battery:#d97706']}, ('rt.sys_on_battery',))
add('status', 9, 14, 3, 1, {'title': 'Charging', 'states': ['0=Not charging:#94a3b8', '1=Charging:#0284c7']}, ('rt.sys_charging',))
add('chart', 0, 15, 12, 2, {'title': 'Battery and input voltage (24 h)', 'chart': 'line', 'decimals': 2, 'band': False, 'legend': True}, ('rt.sys_battery_voltage', 'rt.sys_input_voltage', 'rt.ups_vbat'))
DASHBOARDS.append(('reterminal-vyroba', 'reTerminal — zones, production, sensors, UPS', spec(), False))

# ---- 5. F7 Discovery — PLC -------------------------------------------------------------------------
add('text', 0, 0, 12, 1, {'title': 'F7 Discovery — PLC', 'style': 'heading', 'text': 'STM32F746 Discovery: values of the PLC program, lamp state, run and mode, runtime.'})
add('value', 0, 1, 3, 2, {'title': 'plc_real', 'decimals': 2, 'sparkline': True}, ('f7.plc_real',))
add('value', 3, 1, 3, 2, {'title': 'plc_int', 'decimals': 0, 'sparkline': True}, ('f7.plc_int',))
add('status', 6, 1, 3, 1, {'title': 'PLC OK', 'states': OK_BAD}, ('f7.plc_ok',))
add('status', 9, 1, 3, 1, {'title': 'Lamp', 'states': ON_OFF}, ('f7.lamp',))
add('status', 6, 2, 3, 1, {'title': 'Program running', 'states': RUN}, ('f7.run',))
add('status', 9, 2, 3, 1, {'title': 'Mode', 'states': ['0=Manual:#94a3b8', '1=Auto:#059669', '2=Service:#d97706']}, ('f7.mode',))
add('chart', 0, 3, 8, 3, {'title': 'plc_real (24 h)', 'chart': 'area', 'decimals': 2, 'band': True, 'legend': False}, ('f7.plc_real',))
add('chart', 8, 3, 4, 3, {'title': 'plc_int (24 h)', 'chart': 'step', 'decimals': 0, 'band': False, 'legend': False}, ('f7.plc_int',))
add('status', 0, 6, 3, 1, {'title': 'Ethernet', 'states': LINK}, ('f7.sys_eth_ok',))
add('status', 3, 6, 3, 1, {'title': 'S7 connection', 'states': LINK}, ('f7.sys_s7_ok',))
add('status', 6, 6, 3, 1, {'title': 'Time valid', 'states': OK_BAD}, ('f7.sys_time_valid',))
add('value', 9, 6, 3, 1, {'title': 'Uptime [s]', 'decimals': 0, 'sparkline': False}, ('f7.sys_uptime_s',))
add('bars', 0, 7, 6, 2, {'title': 'Cycle and longest cycle [µs]', 'decimals': 0}, ('f7.sys_cycle_us', 'f7.sys_cycle_max_us'))
add('value', 6, 7, 3, 2, {'title': 'Overruns', 'decimals': 0, 'sparkline': False, 'warn': 1}, ('f7.sys_overruns',))
add('heatmap', 9, 7, 3, 2, {'title': 'plc_real by hour', 'range': '7d', 'decimals': 2}, ('f7.plc_real',))
DASHBOARDS.append(('f7-plc', 'F7 Discovery — PLC', spec(), False))

# ---- 6. System — runtime --------------------------------------------------------------------------
add('text', 0, 0, 12, 1, {'title': 'System — runtime of all boards', 'style': 'heading', 'text': 'Cycle, longest cycle, overruns, uptime and link state of every wall device side by side.'})
add('devices', 0, 1, 4, 3, {'title': 'Devices', 'limit': 10}, devices=[DEV['p4'], DEV['f7'], DEV['rt']])
add('bars', 4, 1, 4, 3, {'title': 'Cycle [µs]', 'decimals': 0}, ('p4.sys_cycle_us', 'f7.sys_cycle_us', 'rt.sys_cycle_us'))
add('bars', 8, 1, 4, 3, {'title': 'Longest cycle [µs]', 'decimals': 0}, ('p4.sys_cycle_max_us', 'f7.sys_cycle_max_us', 'rt.sys_cycle_max_us'))
add('chart', 0, 4, 12, 3, {'title': 'Runtime cycle of the three boards (24 h)', 'chart': 'line', 'decimals': 0, 'band': True, 'legend': True}, ('p4.sys_cycle_us', 'f7.sys_cycle_us', 'rt.sys_cycle_us'))
add('value', 0, 7, 4, 2, {'title': 'P4 uptime [s]', 'decimals': 0, 'sparkline': False}, ('p4.sys_uptime_s',))
add('value', 4, 7, 4, 2, {'title': 'F7 uptime [s]', 'decimals': 0, 'sparkline': False}, ('f7.sys_uptime_s',))
add('value', 8, 7, 4, 2, {'title': 'reTerminal uptime [s]', 'decimals': 0, 'sparkline': False}, ('rt.sys_uptime_s',))
add('bars', 0, 9, 4, 2, {'title': 'Overruns', 'decimals': 0, 'warn': 1}, ('p4.sys_overruns', 'f7.sys_overruns', 'rt.sys_overruns'))
add('status', 4, 9, 2, 1, {'title': 'P4 Ethernet', 'states': LINK}, ('p4.sys_eth_ok',))
add('status', 6, 9, 2, 1, {'title': 'P4 Wi-Fi', 'states': LINK}, ('p4.sys_wifi_ok',))
add('status', 8, 9, 2, 1, {'title': 'P4 S7', 'states': LINK}, ('p4.sys_s7_ok',))
add('status', 10, 9, 2, 1, {'title': 'P4 time', 'states': OK_BAD}, ('p4.sys_time_valid',))
add('status', 4, 10, 2, 1, {'title': 'F7 Ethernet', 'states': LINK}, ('f7.sys_eth_ok',))
add('status', 6, 10, 2, 1, {'title': 'F7 S7', 'states': LINK}, ('f7.sys_s7_ok',))
add('status', 8, 10, 2, 1, {'title': 'F7 time', 'states': OK_BAD}, ('f7.sys_time_valid',))
add('status', 10, 10, 2, 1, {'title': 'reTerminal Modbus', 'states': LINK}, ('rt.sys_mb_ok',))
add('value', 0, 11, 4, 1, {'title': 'P4 net32 peers', 'decimals': 0, 'sparkline': False}, ('p4.sys_nvl_peers',))
add('alarms', 4, 11, 8, 2, {'title': 'Alarms', 'limit': 8})
DASHBOARDS.append(('system-runtime', 'System — runtime of all boards', spec(), False))

# ---- 7. Mimic: zones and drive (ISA-101 canvas) ---------------------------------------------------
add('text', 20, 20, 900, 60, {'title': 'Process picture — reTerminal zones and the i550 drive', 'style': 'normal', 'text': 'Grey process, colour only for abnormal states. Zone heating shown as pumps, temperatures as instruments, the drive as a motor with speed and current.'})
for i, z in enumerate('abc'):
    x = 120 + i * 420
    add('tank', x, 120, 180, 300, {'title': f'Zone {z.upper()}', 'decimals': 1, 'y_min': 0, 'y_max': 60, 'warn': 40, 'action': 50, 'frame': False}, (f'rt.z_temp_{z}',))
    add('instrument', x + 200, 150, 130, 130, {'tag': f'TT-{z.upper()}', 'decimals': 1, 'warn': 40, 'action': 50, 'frame': False}, (f'rt.z_temp_{z}',))
    add('pipe', x + 190, 380, 150, 36, {'orientation': 'horizontal', 'thickness': 40, 'flow_min': 0}, (f'rt.z_heat_{z}',))
    add('pump', x + 210, 300, 120, 120, {'title': f'H-{z.upper()}', 'on_value': 1, 'on_label': 'HEATING', 'off_label': 'IDLE', 'frame': False}, (f'rt.z_heat_{z}',))
add('pipe', 120, 520, 1200, 36, {'orientation': 'horizontal', 'thickness': 40, 'flow_min': 0}, ('rt.running',))
add('pump', 1340, 470, 160, 160, {'title': 'M-550', 'on_value': 1, 'fault_value': 2, 'on_label': 'RUNNING', 'off_label': 'STOPPED', 'frame': False}, ('rt.running',))
add('instrument', 1000, 600, 140, 140, {'tag': 'SI-550', 'decimals': 0, 'frame': False}, ('rt.speed',))
add('instrument', 1160, 600, 140, 140, {'tag': 'II-550', 'decimals': 2, 'warn': 6, 'action': 8, 'frame': False}, ('rt.current',))
add('instrument', 1320, 660, 140, 140, {'tag': 'JI-550', 'decimals': 2, 'frame': False}, ('rt.torque',))
add('value', 120, 600, 800, 140, {'title': 'Latest values', 'decimals': 1, 'sparkline': False}, ('rt.z_temp_a', 'rt.z_temp_b', 'rt.z_temp_c', 'rt.avg_temp', 'rt.speed', 'rt.current', 'rt.pa_pieces'))
add('text', 120, 800, 1380, 70, {'title': 'Note', 'style': 'note', 'text': 'Pipes animate while the bound value is above the threshold (heating on, drive running). Move symbols in the editor by dragging on a 10 px grid.'})
DASHBOARDS.append(('mimika-zony-pohon', 'Process picture — zones and drive (ISA-101)', spec(kind='mimic', layout={'type': 'canvas', 'width': 1600, 'height': 900}), False))

if missing:
    print('MISSING datastreams:', sorted(set(missing)))
    sys.exit(1)
print('devices:', DEV)
for slug, name, sp, default in DASHBOARDS:
    print(f'{slug}: {name} — {len(sp["widgets"])} widgets, kind {sp["kind"]}')
if a.dry:
    io.open(OUT, 'w', encoding='utf-8').write(json.dumps(DASHBOARDS, ensure_ascii=False, indent=1))
    print('dry run: specs written to wall-dashboards.json')
    sys.exit(0)
st, existing = call('GET', '/api/v1/dashboards')
have = {d['slug'] for d in (existing if isinstance(existing, list) else existing.get('items', []))}
only = [x for x in a.only.split(',') if x]
for slug, name, sp, default in DASHBOARDS:
    if only and slug not in only:
        continue
    body = {'name': name, 'spec': sp, 'is_default': default, 'reason': 'presentation dashboards of the ctrl32 wall in English (real devices)'}
    if slug in have:
        st, js = call('PUT', f'/api/v1/dashboards/{slug}', body)
        print(f'{slug}: updated → {st} version {js.get("version")}' if st == 200 else f'{slug}: PUT {st} {js}')
    else:
        body['slug'] = slug
        st, js = call('POST', '/api/v1/dashboards', body)
        print(f'{slug}: created → {st} version {js.get("version")}' if st == 201 else f'{slug}: POST {st} {js}')
