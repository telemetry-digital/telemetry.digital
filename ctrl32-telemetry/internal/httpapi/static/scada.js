// ctrl32 telemetry — SCADA process pictures: the drawing layer of screens of kind "mimic" (screen_elements.go).
// Elements are vector objects on the canvas — rect, ellipse, line, polyline, polygon, pipe, text and industrial
// symbols — with dynamics (properties driven by datastream values: colour, state, level, blink, rotation, flow,
// visibility, text, opacity, angle, movement) and click events (faceplate, trend, screen change, command).
// Rendering is plain SVG; only elements whose appearance changed are replaced, so animations keep running.
(function () {
  'use strict';
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const NS = 'http://www.w3.org/2000/svg';
  const colRe = /^(#[0-9a-fA-F]{3,8}|none)$/;
  const col = (v, fb) => (typeof v === 'string' && colRe.test(v)) ? v : fb;
  const n = (v, fb) => (typeof v === 'number' && isFinite(v)) ? v : fb;
  const r2 = (v) => Math.round(v * 100) / 100;

  // ---- colours ----------------------------------------------------------------------------------------
  // State colours: classic follows the WinCC convention (green running, grey stopped, red fault); isa101 is the
  // high-performance HMI palette (grey process, colour reserved for abnormal situations).
  const SCHEMES = {
    classic: { run: '#22c55e', stop: '#9ca3af', fault: '#ef4444', manual: '#3b82f6', open: '#22c55e', closed: '#9ca3af', transit: '#facc15', off: '#d1d5db', warn: '#f59e0b' },
    isa101: { run: '#475569', stop: '#e2e8f0', fault: '#dc2626', manual: '#60a5fa', open: '#475569', closed: '#e2e8f0', transit: '#94a3b8', off: '#f1f5f9', warn: '#f59e0b' },
  };
  const STATES = Object.keys(SCHEMES.classic);
  const DEF = { body: '#cbd5e1', line: '#334155', medium: '#3b82f6', pipe: '#94a3b8', text: 'var(--fg)' };
  function rgb(h) { let s = String(h || '').replace('#', ''); if (s.length === 3 || s.length === 4) s = s.split('').map((c) => c + c).join(''); const v = parseInt(s.slice(0, 6), 16); return isNaN(v) ? [203, 213, 225] : [(v >> 16) & 255, (v >> 8) & 255, v & 255]; }
  function tone(h, k) { const c = rgb(h).map((x) => Math.round(k >= 0 ? x + (255 - x) * k : x * (1 + k))); return '#' + c.map((x) => Math.max(0, Math.min(255, x)).toString(16).padStart(2, '0')).join(''); }
  // Metallic shading: a horizontal (or vertical) gradient around the base colour.
  const shadeDef = (id, base, vertical) => `<linearGradient id="${id}" x1="0" y1="0" x2="${vertical ? 0 : 1}" y2="${vertical ? 1 : 0}"><stop offset="0" stop-color="${tone(base, -0.35)}"/><stop offset="0.38" stop-color="${tone(base, 0.55)}"/><stop offset="0.62" stop-color="${base}"/><stop offset="1" stop-color="${tone(base, -0.45)}"/></linearGradient>`;

  // ---- symbol library (viewBox 0 0 100 100) ------------------------------------------------------------
  // c = {body, base, line, sw, state, level (0..1|null), medium, spin, label, value, unit, uid}
  const S = {};
  function sym(id, name, cat, w, h, fit, draw) { S[id] = { id, name, cat, w, h, fit, draw }; }
  const CATS = { vessels: 'Vessels', pumps: 'Pumps and drives', valves: 'Valves', heat: 'Heat transfer', process: 'Process equipment', instruments: 'Instruments', electrical: 'Electrical', misc: 'Other' };
  const P = (c, d, fill, extra) => `<path d="${d}" fill="${fill === undefined ? c.body : fill}" stroke="${c.line}" stroke-width="${c.sw}" stroke-linejoin="round" ${extra || ''}/>`;
  const L = (c, d, w, color) => `<path d="${d}" fill="none" stroke="${color || c.line}" stroke-width="${w || c.sw}" stroke-linecap="round" stroke-linejoin="round"/>`;
  const C = (c, cx, cy, r, fill, extra) => `<circle cx="${cx}" cy="${cy}" r="${r}" fill="${fill === undefined ? c.body : fill}" stroke="${c.line}" stroke-width="${c.sw}" ${extra || ''}/>`;
  const T = (x, y, s, size, fill, weight) => `<text x="${x}" y="${y}" text-anchor="middle" font-size="${size}" font-weight="${weight || 600}" fill="${fill || '#111827'}" font-family="var(--mono, monospace)">${esc(s)}</text>`;
  // A liquid level inside a clip path between y0 (full) and y1 (empty).
  function level(c, clipD, y0, y1) {
    if (c.level == null) return '';
    const f = Math.max(0, Math.min(1, c.level)), top = y1 - (y1 - y0) * f, id = c.uid + 'lv';
    return `<clipPath id="${id}"><path d="${clipD}"/></clipPath><g clip-path="url(#${id})"><rect x="0" y="${top.toFixed(2)}" width="100" height="${(y1 - top + 4).toFixed(2)}" fill="${c.medium}" opacity="0.88"/><line x1="0" x2="100" y1="${top.toFixed(2)}" y2="${top.toFixed(2)}" stroke="${tone(c.medium, 0.5)}" stroke-width="1.2"/></g>`;
  }
  const spinCls = (c, cls) => c.spin ? ` class="${cls || 'sc-spin'}" style="animation-duration:${(1 / (c.speed || 1)).toFixed(2)}s"` : '';

  // Vessels
  sym('tank_vertical', 'Vertical tank', 'vessels', 120, 200, 'none', (c) => { const d = 'M8,14 C8,2 92,2 92,14 L92,86 C92,98 8,98 8,86 Z'; return P(c, d) + level(c, d, 3, 97) + P(c, d, 'none') + L(c, 'M8,14 H92 M8,86 H92', 0.8); });
  sym('tank_horizontal', 'Horizontal tank', 'vessels', 220, 110, 'none', (c) => { const d = 'M16,8 L84,8 C100,8 100,92 84,92 L16,92 C0,92 0,8 16,8 Z'; return P(c, d) + level(c, d, 8, 92) + P(c, d, 'none') + L(c, 'M16,8 V92 M84,8 V92', 0.8); });
  sym('tank_spherical', 'Spherical tank', 'vessels', 130, 140, 'meet', (c) => { const d = 'M50,4 A40,40 0 1 1 49.9,4 Z'; return L(c, 'M22,70 L14,98 M78,70 L86,98 M50,84 V98', c.sw) + P(c, d) + level(c, d, 4, 84) + P(c, d, 'none'); });
  sym('silo', 'Silo', 'vessels', 110, 200, 'none', (c) => { const d = 'M10,4 L90,4 L90,62 L58,90 L42,90 L10,62 Z'; return L(c, 'M14,64 L10,99 M86,64 L90,99') + P(c, d) + level(c, d, 4, 90) + P(c, d, 'none') + P(c, 'M43,90 H57 V97 H43 Z', c.body); });
  sym('hopper', 'Hopper', 'vessels', 120, 110, 'none', (c) => { const d = 'M4,6 L96,6 L62,82 L38,82 Z'; return P(c, d) + level(c, d, 6, 82) + P(c, d, 'none') + P(c, 'M40,82 H60 V96 H40 Z'); });
  sym('basin', 'Open basin', 'vessels', 200, 100, 'none', (c) => { const d = 'M4,8 L4,94 L96,94 L96,8 Z'; return `<path d="${d}" fill="${c.body}" opacity="0.5"/>` + level(c, d, 8, 94) + L(c, 'M4,8 V94 H96 V8'); });
  sym('reactor', 'Reactor with agitator', 'vessels', 140, 210, 'none', (c) => { const d = 'M12,30 C12,19 88,19 88,30 L88,86 C88,97 12,97 12,86 Z'; return P(c, d) + level(c, d, 21, 96) + P(c, d, 'none') + L(c, 'M5,38 V84 M95,38 V84 M5,38 H12 M5,84 H12 M95,38 H88 M95,84 H88', c.sw * 0.8) + P(c, 'M38,2 H62 V15 H38 Z', c.state ? c.body : DEF.body) + L(c, 'M50,15 V82') + `<g${spinCls(c, 'sc-agit')}>${P(c, 'M30,79 C38,73 62,73 70,79 C62,85 38,85 30,79 Z', c.line)}</g>`; });
  sym('column', 'Column', 'vessels', 70, 240, 'none', (c) => { const d = 'M18,7 C18,1 82,1 82,7 L82,93 C82,99 18,99 18,93 Z'; let t = ''; for (let y = 18, k = 0; y < 88; y += 10, k++) t += `M${k % 2 ? 36 : 18},${y} H${k % 2 ? 82 : 64} `; return P(c, d) + level(c, d, 2, 98) + P(c, d, 'none') + L(c, t, 0.9); });
  sym('stack', 'Stack / chimney', 'vessels', 60, 200, 'none', (c) => P(c, 'M34,2 H66 L80,98 H20 Z') + L(c, 'M30,20 H70 M27,40 H73', 0.8));

  // Pumps and drives
  sym('pump_centrifugal', 'Centrifugal pump', 'pumps', 90, 84, 'meet', (c) => P(c, 'M30,76 L20,97 L80,97 L70,76 Z', DEF.body) + P(c, 'M50,20 H95 V38 H72') + C(c, 50, 52, 32) + C(c, 50, 52, 7, c.line));
  sym('pump_gear', 'Positive displacement pump', 'pumps', 90, 84, 'meet', (c) => P(c, 'M30,78 L20,97 L80,97 L70,78 Z', DEF.body) + C(c, 50, 50, 34) + C(c, 38, 50, 12, 'none') + C(c, 62, 50, 12, 'none'));
  sym('pump_vacuum', 'Vacuum pump', 'pumps', 90, 84, 'meet', (c) => P(c, 'M30,78 L20,97 L80,97 L70,78 Z', DEF.body) + C(c, 50, 50, 34) + L(c, 'M24,36 L76,50 L24,64'));
  sym('motor', 'Motor', 'pumps', 120, 84, 'meet', (c) => P(c, 'M30,4 H54 V18 H30 Z', DEF.body) + P(c, 'M8,18 H78 A4,4 0 0 1 82,22 V78 A4,4 0 0 1 78,82 H8 A4,4 0 0 1 4,78 V22 A4,4 0 0 1 8,18 Z') + L(c, 'M18,24 V76 M28,24 V76 M38,24 V76 M48,24 V76 M58,24 V76 M68,24 V76', 0.7) + P(c, 'M82,44 H97 V56 H82 Z', DEF.body) + L(c, 'M12,82 L8,97 M74,82 L78,97') + T(43, 58, 'M', 20, '#111827', 700));
  sym('fan', 'Fan', 'pumps', 90, 90, 'meet', (c) => { const b = 'M50,50 C42,32 50,12 62,14 C66,30 60,42 50,50 Z'; return C(c, 50, 50, 46) + `<g${spinCls(c)}>${[0, 120, 240].map((a) => `<path d="${b}" transform="rotate(${a} 50 50)" fill="${tone(c.base, -0.35)}" stroke="${c.line}" stroke-width="${c.sw * 0.6}"/>`).join('')}<circle cx="50" cy="50" r="6" fill="${c.line}"/></g>`; });
  sym('blower', 'Blower', 'pumps', 100, 90, 'meet', (c) => P(c, 'M44,16 H97 V34 H72') + C(c, 44, 54, 38) + `<g${spinCls(c)}>${[0, 60, 120, 180, 240, 300].map((a) => `<line x1="44" y1="54" x2="44" y2="26" transform="rotate(${a} 44 54)" stroke="${c.line}" stroke-width="${c.sw}"/>`).join('')}<circle cx="44" cy="54" r="5" fill="${c.line}"/></g>`);
  sym('compressor', 'Compressor', 'pumps', 90, 90, 'meet', (c) => C(c, 50, 50, 44) + L(c, 'M12,28 L88,40 M12,72 L88,60'));
  sym('agitator', 'Agitator', 'pumps', 60, 150, 'none', (c) => P(c, 'M28,2 H72 V16 H28 Z') + L(c, 'M50,16 V86', c.sw * 1.3) + `<g${spinCls(c, 'sc-agit')}>${P(c, 'M14,88 C30,80 70,80 86,88 C70,96 30,96 14,88 Z', c.line)}</g>`);
  sym('conveyor', 'Belt conveyor', 'pumps', 240, 60, 'none', (c) => P(c, 'M13,30 H87 A13,13 0 0 1 87,56 H13 A13,13 0 0 1 13,30 Z') + C(c, 13, 43, 9, DEF.body) + C(c, 87, 43, 9, DEF.body) + (c.spin ? `<line x1="13" x2="87" y1="30" y2="30" stroke="${tone(c.base, -0.5)}" stroke-width="3" stroke-dasharray="6 6" class="sc-flow" style="--sc-per:-12px;animation-duration:${(1 / (c.speed || 1)).toFixed(2)}s"/>` : '') + L(c, 'M20,57 L14,98 M80,57 L86,98'));
  sym('screw_conveyor', 'Screw conveyor', 'pumps', 240, 60, 'none', (c) => { let h = ''; for (let x = 16; x < 92; x += 8) h += `M${x},28 L${x + 6},60 `; return P(c, 'M8,26 H96 V62 H8 Z') + L(c, h, 0.9) + P(c, 'M0,34 H8 V54 H0 Z', DEF.body); });
  sym('mixer_static', 'Static mixer', 'pumps', 200, 50, 'none', (c) => { let z = 'M4,50'; for (let k = 0, x = 8; x <= 92; x += 7, k++) z += ` L${x},${k % 2 ? 24 : 76}`; return P(c, 'M4,20 H96 V80 H4 Z') + L(c, z, 0.9); });

  // Valves (the bow-tie body carries the state colour)
  const bow = (c, y) => P(c, `M8,${y - 18} L8,${y + 18} L50,${y} Z M92,${y - 18} L92,${y + 18} L50,${y} Z`);
  sym('valve_gate', 'Gate valve', 'valves', 80, 70, 'meet', (c) => bow(c, 62) + L(c, 'M50,62 V26 M32,24 H68'));
  sym('valve_ball', 'Ball valve', 'valves', 80, 70, 'meet', (c) => bow(c, 62) + C(c, 50, 62, 9) + L(c, 'M50,53 V26 M36,26 H64'));
  sym('valve_butterfly', 'Butterfly valve', 'valves', 80, 70, 'meet', (c) => bow(c, 62) + L(c, 'M50,44 V80 M50,44 V26 M36,26 H64') + C(c, 50, 62, 4, c.line));
  sym('valve_control', 'Control valve', 'valves', 80, 90, 'meet', (c) => bow(c, 72) + L(c, 'M50,72 V32') + P(c, 'M26,32 C26,6 74,6 74,32 Z'));
  sym('valve_solenoid', 'Solenoid valve', 'valves', 80, 90, 'meet', (c) => bow(c, 72) + L(c, 'M50,72 V34') + P(c, 'M34,10 H66 V34 H34 Z', DEF.body) + T(50, 28, 'S', 16));
  sym('valve_3way', 'Three-way valve', 'valves', 80, 80, 'meet', (c) => P(c, 'M8,26 L8,62 L50,44 Z M92,26 L92,62 L50,44 Z M32,96 L68,96 L50,44 Z'));
  sym('valve_check', 'Check valve', 'valves', 80, 60, 'meet', (c) => P(c, 'M8,40 L8,76 L50,58 Z') + P(c, 'M92,40 L92,76 L50,58 Z', 'none') + L(c, 'M50,40 V76') + P(c, 'M28,16 H62 L62,8 L76,19 L62,30 L62,22 H28 Z', c.line));
  sym('valve_safety', 'Safety valve', 'valves', 70, 100, 'meet', (c) => P(c, 'M32,98 L68,98 L50,62 Z M94,44 L94,80 L50,62 Z') + L(c, 'M50,62 V54 L38,50 L62,44 L38,38 L62,32 L38,26 L50,22 V6'));
  sym('valve_manual', 'Hand valve', 'valves', 80, 70, 'meet', (c) => bow(c, 62) + L(c, 'M50,62 V30') + `<ellipse cx="50" cy="26" rx="20" ry="6" fill="none" stroke="${c.line}" stroke-width="${c.sw}"/>`);
  sym('damper', 'Damper', 'valves', 80, 80, 'meet', (c) => { const a = c.level != null ? 90 * (1 - Math.max(0, Math.min(1, c.level))) : ({ open: 0, run: 0, transit: 45 }[c.state] ?? 90); return P(c, 'M8,8 H92 V92 H8 Z', 'none') + `<g transform="rotate(${a} 50 50)"><rect x="16" y="45" width="68" height="10" rx="3" fill="${c.body}" stroke="${c.line}" stroke-width="${c.sw}"/></g>` + C(c, 50, 50, 3, c.line); });

  // Heat transfer
  sym('heat_exchanger', 'Heat exchanger', 'heat', 90, 90, 'meet', (c) => C(c, 50, 50, 44) + L(c, 'M6,50 H28 L36,28 L50,72 L64,28 L72,50 H94'));
  sym('shell_tube', 'Shell and tube exchanger', 'heat', 220, 90, 'none', (c) => P(c, 'M14,14 H86 C99,14 99,86 86,86 H14 C1,86 1,14 14,14 Z') + L(c, 'M14,32 H86 M14,50 H86 M14,68 H86', 0.8) + L(c, 'M14,14 V86 M86,14 V86', 0.8));
  sym('plate_hx', 'Plate exchanger', 'heat', 100, 110, 'meet', (c) => { let p = ''; for (let x = 22; x <= 78; x += 7) p += `M${x},18 V92 `; return P(c, 'M8,8 H92 V18 H8 Z', c.line) + P(c, 'M12,18 H88 V92 H12 Z') + L(c, p, 0.8); });
  sym('heater', 'Electric heater', 'heat', 90, 70, 'meet', (c) => P(c, 'M6,20 H94 V76 H6 Z', DEF.body) + L(c, 'M12,48 L20,30 L28,66 L36,30 L44,66 L52,30 L60,66 L68,30 L76,66 L84,30 L90,48', c.sw * 1.4, c.state === 'run' ? '#ef4444' : c.line));
  sym('air_cooler', 'Air cooler', 'heat', 110, 90, 'meet', (c) => { let f = ''; for (let x = 12; x < 92; x += 6) f += `M${x},8 V38 `; return P(c, 'M6,6 H94 V40 H6 Z') + L(c, f, 0.7) + C(c, 50, 68, 22, DEF.body) + `<g${spinCls(c)}>${[0, 120, 240].map((a) => `<path d="M50,68 L44,50 L56,50 Z" transform="rotate(${a} 50 68)" fill="${c.line}"/>`).join('')}</g>`; });
  sym('boiler', 'Boiler', 'heat', 110, 170, 'none', (c) => { const d = 'M12,18 C12,6 88,6 88,18 L88,70 L12,70 Z'; return P(c, d) + level(c, d, 6, 70) + P(c, d, 'none') + P(c, 'M12,70 H88 V97 H12 Z', DEF.body) + (c.state === 'run' ? '<path d="M50,95 C38,88 42,79 48,72 C50,80 60,80 60,88 C60,92 56,95 50,95 Z" fill="#f97316" class="sc-flame"/>' : ''); });
  sym('cooling_tower', 'Cooling tower', 'heat', 120, 150, 'none', (c) => P(c, 'M20,8 H80 C68,40 68,60 88,96 H12 C32,60 32,40 20,8 Z') + L(c, 'M28,96 L50,70 L72,96', 0.8));
  sym('furnace', 'Furnace', 'heat', 120, 120, 'meet', (c) => P(c, 'M10,20 H90 V96 H10 Z') + P(c, 'M40,4 H60 V20 H40 Z', DEF.body) + (c.state === 'run' ? '<path d="M50,88 C34,78 40,64 48,54 C50,66 64,64 62,78 C61,84 56,88 50,88 Z" fill="#f97316" class="sc-flame"/>' : L(c, 'M30,86 H70', 1)));

  // Process equipment
  sym('filter', 'Filter', 'process', 80, 100, 'meet', (c) => P(c, 'M16,6 H84 V94 H16 Z') + `<path d="M16,94 L84,6" stroke="${c.line}" stroke-width="${c.sw}" stroke-dasharray="6 4" fill="none"/>`);
  sym('cyclone', 'Cyclone', 'process', 90, 170, 'none', (c) => { const d = 'M14,6 H86 V40 L58,92 H42 L14,40 Z'; return P(c, d) + level(c, d, 6, 92) + P(c, d, 'none') + P(c, 'M86,12 H98 V26 H86 Z', DEF.body) + P(c, 'M42,92 H58 V99 H42 Z', DEF.body); });
  sym('separator', 'Separator', 'process', 220, 100, 'none', (c) => { const d = 'M16,8 L84,8 C100,8 100,92 84,92 L16,92 C0,92 0,8 16,8 Z'; return P(c, d) + level(c, d, 8, 92) + P(c, d, 'none') + L(c, 'M64,56 V92'); });
  sym('flow_meter', 'Flow meter', 'process', 100, 70, 'meet', (c) => L(c, 'M2,50 H98', c.sw * 3, DEF.body) + P(c, 'M26,22 H74 V78 H26 Z') + C(c, 50, 50, 16, '#fff') + T(50, 56, 'FE', 14));
  sym('orifice', 'Orifice plate', 'process', 50, 80, 'meet', (c) => P(c, 'M34,6 H44 V94 H34 Z M56,6 H66 V94 H56 Z'));
  sym('scale', 'Weighing scale', 'process', 110, 80, 'meet', (c) => P(c, 'M4,58 H96 V70 H4 Z') + L(c, 'M14,70 V92 M86,70 V92') + P(c, 'M30,8 H70 V38 H30 Z', '#f8fafc') + L(c, 'M50,38 V58'));
  sym('nozzle', 'Spray nozzle', 'process', 60, 80, 'meet', (c) => P(c, 'M36,4 H64 V36 L50,58 L36,36 Z') + L(c, 'M50,62 L34,94 M50,62 L50,96 M50,62 L66,94', 1.2, c.medium));
  sym('dryer', 'Rotary dryer', 'process', 240, 80, 'none', (c) => P(c, 'M6,20 H94 V80 H6 Z') + L(c, 'M20,20 V80 M80,20 V80', 1.6) + `<g${spinCls(c, 'sc-agit')}>${L(c, 'M30,50 H70', 3)}</g>`);

  // Instruments (ISA 5.1 bubbles; the label is the element text, the value the bound tag)
  const bubbleText = (c) => T(50, 44, c.label || '', 15) + T(50, 70, c.value == null ? '' : c.value + (c.unit ? ' ' + c.unit : ''), 13, c.state === 'fault' ? '#dc2626' : c.state === 'warn' ? '#b45309' : '#111827', 700);
  sym('instr_field', 'Instrument (field)', 'instruments', 80, 80, 'meet', (c) => C(c, 50, 50, 44, '#ffffff', c.state === 'fault' || c.state === 'warn' ? `stroke="${c.body}"` : '') + bubbleText(c));
  sym('instr_panel', 'Instrument (panel)', 'instruments', 80, 80, 'meet', (c) => C(c, 50, 50, 44, '#ffffff') + L(c, 'M6,50 H94', 1.2) + bubbleText(c));
  sym('instr_dcs', 'Instrument (DCS)', 'instruments', 80, 80, 'meet', (c) => P(c, 'M6,6 H94 V94 H6 Z', '#ffffff') + C(c, 50, 50, 44, 'none') + L(c, 'M6,50 H94', 1.2) + bubbleText(c));
  sym('instr_plc', 'Instrument (PLC)', 'instruments', 80, 80, 'meet', (c) => P(c, 'M6,6 H94 V94 H6 Z', '#ffffff') + P(c, 'M50,6 L94,50 L50,94 L6,50 Z', 'none') + L(c, 'M6,50 H94', 1.2) + bubbleText(c));
  sym('gauge_dial', 'Dial gauge', 'instruments', 100, 100, 'meet', (c) => { const f = c.level == null ? 0 : Math.max(0, Math.min(1, c.level)); let ticks = ''; for (let k = 0; k <= 10; k++) { const a = (-210 + 240 * k / 10) * Math.PI / 180, r0 = k % 5 ? 36 : 32; ticks += `M${(50 + r0 * Math.cos(a)).toFixed(1)},${(50 + r0 * Math.sin(a)).toFixed(1)} L${(50 + 40 * Math.cos(a)).toFixed(1)},${(50 + 40 * Math.sin(a)).toFixed(1)} `; } return C(c, 50, 50, 46, '#ffffff') + L(c, ticks, 1) + `<g transform="rotate(${(-120 + 240 * f).toFixed(1)} 50 50)"><path d="M48,52 L50,16 L52,52 Z" fill="#dc2626"/></g>` + C(c, 50, 50, 4, c.line) + T(50, 80, c.value == null ? '—' : c.value, 12, '#111827', 700) + T(50, 91, c.unit || '', 8, '#475569', 400); });
  sym('lamp', 'Indicator lamp', 'instruments', 50, 50, 'meet', (c) => `<radialGradient id="${c.uid}rg" cx="0.38" cy="0.35" r="0.7"><stop offset="0" stop-color="${tone(c.base, 0.7)}"/><stop offset="0.6" stop-color="${c.base}"/><stop offset="1" stop-color="${tone(c.base, -0.4)}"/></radialGradient>` + C(c, 50, 50, 44, DEF.body) + C(c, 50, 50, 34, `url(#${c.uid}rg)`));
  sym('pushbutton', 'Push button', 'instruments', 90, 56, 'none', (c) => `<linearGradient id="${c.uid}pb" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="${tone(c.base, 0.45)}"/><stop offset="1" stop-color="${tone(c.base, -0.25)}"/></linearGradient><rect x="4" y="6" width="92" height="88" rx="14" fill="url(#${c.uid}pb)" stroke="${c.line}" stroke-width="${c.sw}"/>` + T(50, 58, c.label || '', 22, '#111827', 700));
  sym('selector', 'Selector switch', 'instruments', 70, 70, 'meet', (c) => { const on = c.state === 'run' || c.state === 'open' || c.state === 'manual'; return C(c, 50, 50, 44, DEF.body) + `<g transform="rotate(${on ? 45 : -45} 50 50)"><rect x="42" y="14" width="16" height="72" rx="6" fill="${c.body}" stroke="${c.line}" stroke-width="${c.sw}"/><circle cx="50" cy="22" r="3" fill="${c.line}"/></g>`; });
  sym('bargraph', 'Bar graph', 'instruments', 40, 160, 'none', (c) => { let t = ''; for (let k = 0; k <= 10; k++) t += `M4,${4 + 9.2 * k} H${k % 5 ? 12 : 18} `; const f = c.level == null ? 0 : Math.max(0, Math.min(1, c.level)); return L(c, t, 0.8) + P(c, 'M24,4 H96 V96 H24 Z', '#f8fafc') + `<rect x="24" y="${(4 + 92 * (1 - f)).toFixed(1)}" width="72" height="${(92 * f).toFixed(1)}" fill="${c.state === 'fault' ? '#ef4444' : c.state === 'warn' ? '#f59e0b' : c.medium}"/>` + P(c, 'M24,4 H96 V96 H24 Z', 'none'); });
  sym('analyzer', 'Analyzer', 'instruments', 80, 80, 'meet', (c) => P(c, 'M6,6 H94 V94 H6 Z', '#ffffff') + T(50, 36, c.label || 'AT', 15) + T(50, 70, c.value == null ? '' : c.value + (c.unit ? ' ' + c.unit : ''), 13, '#111827', 700));
  sym('horn', 'Alarm horn', 'instruments', 70, 60, 'meet', (c) => P(c, 'M8,34 H28 L70,8 V92 L28,66 H8 Z') + (c.state === 'fault' || c.state === 'warn' || c.state === 'run' ? L(c, 'M80,30 Q90,50 80,70 M88,20 Q102,50 88,80', 2.2, '#ef4444') : ''));

  // Electrical
  sym('breaker', 'Circuit breaker', 'electrical', 60, 90, 'meet', (c) => { const closed = c.state === 'closed' || c.state === 'run'; return L(c, 'M50,4 V28 M50,72 V96', c.sw * 1.4) + C(c, 50, 72, 4, c.line) + L(c, closed ? 'M50,72 L50,28' : 'M50,72 L28,34', c.sw * 1.8, c.state ? c.body : c.line) + L(c, 'M42,28 H58'); });
  sym('transformer', 'Transformer', 'electrical', 60, 90, 'meet', (c) => L(c, 'M50,2 V14 M50,86 V98') + C(c, 50, 36, 22, 'none') + C(c, 50, 64, 22, 'none') + `<circle cx="50" cy="50" r="0" fill="${c.body}"/>`);
  sym('generator', 'Generator', 'electrical', 80, 80, 'meet', (c) => C(c, 50, 50, 44) + T(50, 46, 'G', 22, '#111827', 700) + L(c, 'M32,64 Q41,54 50,64 T68,64', 1.6));
  sym('battery', 'Battery', 'electrical', 100, 60, 'meet', (c) => { const f = c.level == null ? 0 : Math.max(0, Math.min(1, c.level)); const fc = f < 0.2 ? '#ef4444' : f < 0.4 ? '#f59e0b' : '#22c55e'; return P(c, 'M4,14 H86 V86 H4 Z', '#f8fafc') + P(c, 'M86,34 H96 V66 H86 Z', c.line) + `<rect x="9" y="19" width="${(72 * f).toFixed(1)}" height="62" fill="${fc}"/>`; });
  sym('solar_panel', 'Solar panel', 'electrical', 110, 70, 'meet', (c) => P(c, 'M22,8 H96 L78,72 H4 Z', '#1e3a8a') + L(c, 'M40,8 L22,72 M59,8 L41,72 M78,8 L60,72 M13,40 H87', 0.8, '#93c5fd') + L(c, 'M40,72 V96 M30,96 H50'));
  sym('wind_turbine', 'Wind turbine', 'electrical', 80, 150, 'meet', (c) => P(c, 'M47,40 L43,99 H57 L53,40 Z', DEF.body) + `<g${spinCls(c)}>${[0, 120, 240].map((a) => `<path d="M50,36 L47,6 Q50,2 53,6 Z" transform="rotate(${a} 50 36)" fill="#f8fafc" stroke="${c.line}" stroke-width="${c.sw * 0.6}"/>`).join('')}</g>` + C(c, 50, 36, 5));
  sym('socket', 'Load / consumer', 'electrical', 60, 60, 'meet', (c) => P(c, 'M10,10 H90 V90 H10 Z') + L(c, 'M10,10 L90,90 M90,10 L10,90', 1));

  // Other
  sym('arrow', 'Flow arrow', 'misc', 60, 30, 'meet', (c) => P(c, 'M4,42 H62 V26 L96,50 L62,74 V58 H4 Z', c.state ? c.body : c.line));
  sym('junction', 'Pipe junction', 'misc', 16, 16, 'meet', (c) => C(c, 50, 50, 40, c.line));
  sym('flange', 'Flange', 'misc', 20, 50, 'none', (c) => P(c, 'M30,4 H70 V96 H30 Z', c.line));
  sym('building', 'Building', 'misc', 120, 110, 'meet', (c) => P(c, 'M8,40 L50,6 L92,40 V96 H8 Z') + P(c, 'M40,62 H60 V96 H40 Z', DEF.body));

  const SYMBOL_IDS = Object.keys(S);

  // ---- values, formatting, dynamics ------------------------------------------------------------------
  function fmtVal(v, dec) {
    if (v == null) return null;
    if (typeof v === 'boolean') return v ? 'ON' : 'OFF';
    if (typeof v === 'number') return v.toFixed(dec == null ? 1 : dec);
    return String(v);
  }
  const cmp = (a, op, b) => a != null && ({ '>': a > b, '>=': a >= b, '<': a < b, '<=': a <= b, '==': a == b, '!=': a != b }[op] || false);
  function lin(v, l) { const f = (v - l[0]) / (l[1] - l[0]); const out = l[2] + f * (l[3] - l[2]); const lo = Math.min(l[2], l[3]), hi = Math.max(l[2], l[3]); return Math.max(lo, Math.min(hi, out)); }
  function numVal(x) { if (!x) return null; if (typeof x.value === 'number') return x.value; if (x.value_json === true) return 1; if (x.value_json === false) return 0; if (typeof x.value_json === 'number') return x.value_json; return null; }
  // Evaluate an element's dynamics against the latest values: {prop: result}.
  function evalDyn(e, values) {
    const out = {};
    for (const d of e.dyn || []) {
      const x = values[d.tag || e.tag]; const v = numVal(x);
      if (v == null) continue; // no value: no state, nothing hidden, texts show "—"
      switch (d.prop) {
        case 'fill': case 'stroke': case 'color': case 'state': case 'text': {
          const m = (d.map || []).find((m) => cmp(v, m.op, m.v));
          if (m) out[d.prop] = m.to; else if (d.prop === 'text') out.text = fmtVal(v, e.decimals);
          break;
        }
        case 'visible': out.visible = cmp(v, d.op, d.v); break;
        case 'blink': if (cmp(v, d.op, d.v)) out.blink = { color: d.color || '' }; break;
        case 'flow': if (cmp(v, d.op, d.v)) out.flow = { speed: d.speed || 1 }; break;
        case 'rotate': if (cmp(v, d.op, d.v)) out.spin = { speed: d.speed || 1 }; break;
        case 'level': out.level = lin(v, d.lin); break;
        case 'opacity': out.opacity = lin(v, d.lin); break;
        case 'angle': out.angle = lin(v, d.lin); break;
        case 'move_x': out.dx = lin(v, d.lin); break;
        case 'move_y': out.dy = lin(v, d.lin); break;
      }
    }
    return out;
  }

  // ---- element rendering -------------------------------------------------------------------------------
  const dashArr = (d, sw) => ({ dash: `${sw * 4} ${sw * 3}`, dot: `${sw} ${sw * 2}`, dashdot: `${sw * 5} ${sw * 2} ${sw} ${sw * 2}` }[d] || '');
  const ptsAttr = (pts) => (pts || []).map((p) => r2(p[0]) + ',' + r2(p[1])).join(' ');
  function bbox(e) { if (e.points && e.points.length) { const xs = e.points.map((p) => p[0]), ys = e.points.map((p) => p[1]); const x = Math.min(...xs), y = Math.min(...ys); return { x, y, w: Math.max(...xs) - x, h: Math.max(...ys) - y }; } return { x: n(e.x, 0), y: n(e.y, 0), w: n(e.w, 0), h: n(e.h, 0) }; }
  function clickable(e) { return !!((e.on && e.on.length) || (e.tag && (e.type === 'symbol' || e.type === 'text'))); }
  // env: {values, ds (id → datastream), scheme, uid, edit}
  function renderEl(e, env) {
    const dv = evalDyn(e, env.values); const st = e.style || {}; const sch = SCHEMES[env.scheme] || SCHEMES.classic;
    if (dv.visible === false && !env.edit) return `<g data-eid="${esc(e.id)}" display="none"></g>`;
    const b = bbox(e); const cx = b.x + b.w / 2, cy = b.y + b.h / 2; const uid = env.uid + esc(e.id) + '-';
    const x = values(env, e.tag); const d = e.tag ? env.ds[e.tag] : null;
    const badQ = !!(x && x.quality && x.quality !== 'ok');
    let val = dv.text !== undefined ? dv.text : fmtVal(numVal(x) ?? (x && x.value_json != null ? x.value_json : null), e.decimals);
    if (val != null && badQ) val += ' ?';
    const unit = e.unit ?? (d ? d.unit : '');
    const state = dv.state || null;
    const stateCol = state ? sch[state] : null;
    const baseFill = col(dv.fill, null) || stateCol || col(st.fill, null);
    const stroke = col(dv.stroke, null) || col(st.stroke, null);
    const sw = n(st.sw, null);
    const op = dv.opacity != null ? dv.opacity : n(st.opacity, 1);
    let inner = '', defs = '';
    switch (e.type) {
      case 'rect': case 'ellipse': {
        let fill = baseFill || (e.type === 'rect' ? 'none' : 'none');
        if (st.shade && fill !== 'none') { defs += shadeDef(uid + 'g', fill, b.h < b.w * 0.6); fill = `url(#${uid}g)`; }
        const common = `fill="${fill}" stroke="${stroke || 'none'}" stroke-width="${sw ?? 1}" ${st.dash ? `stroke-dasharray="${dashArr(st.dash, sw ?? 1)}"` : ''}`;
        inner = e.type === 'rect' ? `<rect x="${r2(b.x)}" y="${r2(b.y)}" width="${r2(b.w)}" height="${r2(b.h)}" rx="${n(st.radius, 0)}" ${common}/>` : `<ellipse cx="${r2(cx)}" cy="${r2(cy)}" rx="${r2(b.w / 2)}" ry="${r2(b.h / 2)}" ${common}/>`;
        if (dv.level != null && e.type === 'rect') { const f = Math.max(0, Math.min(1, dv.level)); inner += `<rect x="${r2(b.x)}" y="${r2(b.y + b.h * (1 - f))}" width="${r2(b.w)}" height="${r2(b.h * f)}" rx="${n(st.radius, 0)}" fill="${col(st.medium, DEF.medium)}" opacity="0.9"/>`; }
        break;
      }
      case 'line': case 'polyline': case 'polygon': {
        const w = sw ?? 2; const fill = e.type === 'polygon' ? (baseFill || 'none') : 'none';
        const tag = e.type === 'polygon' ? 'polygon' : 'polyline';
        inner = `<${tag} points="${ptsAttr(e.points)}" fill="${fill}" stroke="${stroke || DEF.line}" stroke-width="${w}" stroke-linecap="round" stroke-linejoin="round" ${st.dash ? `stroke-dasharray="${dashArr(st.dash, w)}"` : ''}/>`;
        if (dv.flow) inner += `<polyline points="${ptsAttr(e.points)}" fill="none" stroke="${tone(stroke || DEF.line, 0.6)}" stroke-width="${Math.max(1, w * 0.6)}" stroke-dasharray="${w * 3} ${w * 3}" class="sc-flow" style="--sc-per:-${w * 6}px;animation-duration:${(1 / dv.flow.speed).toFixed(2)}s"/>`;
        break;
      }
      case 'pipe': {
        const w = sw ?? 12; const med = col(dv.fill, null) || col(st.medium, null) || stateCol || col(st.fill, null) || DEF.pipe; const pts = ptsAttr(e.points);
        inner = `<polyline points="${pts}" fill="none" stroke="${stroke || tone(med, -0.55)}" stroke-width="${w + 2}" stroke-linejoin="round" stroke-linecap="butt"/><polyline points="${pts}" fill="none" stroke="${med}" stroke-width="${w}" stroke-linejoin="round" stroke-linecap="butt"/>`;
        if (st.shade !== false) inner += `<polyline points="${pts}" fill="none" stroke="#ffffff" stroke-opacity="0.35" stroke-width="${Math.max(1, w * 0.3)}" stroke-linejoin="round"/>`;
        if (dv.flow) { const dl = Math.max(4, w * 0.9); inner += `<polyline points="${pts}" fill="none" stroke="${tone(med, 0.75)}" stroke-width="${Math.max(1.5, w * 0.34)}" stroke-dasharray="${dl} ${dl * 1.3}" stroke-linecap="round" class="sc-flow" style="--sc-per:-${(dl * 2.3).toFixed(1)}px;animation-duration:${(1 / dv.flow.speed).toFixed(2)}s"/>`; }
        break;
      }
      case 'text': {
        const size = n(st.font, 14); const color = col(dv.color, null) || col(st.color, null) || DEF.text;
        const txt = String(e.text ?? '{value}').replace(/\{value\}/g, val ?? '—').replace(/\{unit\}/g, unit || '').replace(/\{name\}/g, e.name || '').replace(/\{time\}/g, x && x.measured_at ? new Date(x.measured_at).toLocaleTimeString(document.documentElement.lang || undefined) : '');
        const lines = txt.split('\n'); const al = st.align || 'start'; const va = st.valign || 'middle';
        const tx = al === 'middle' ? cx : al === 'end' ? b.x + b.w - 4 : b.x + 4; const lh = size * 1.2; const total = lh * lines.length;
        const ty0 = va === 'top' ? b.y + size + 2 : va === 'bottom' ? b.y + b.h - total + size - 2 : cy - total / 2 + size * 0.85;
        const bg = col(dv.fill, null) || stateCol || col(st.bg, null);
        if (bg || stroke) inner += `<rect x="${r2(b.x)}" y="${r2(b.y)}" width="${r2(b.w)}" height="${r2(b.h)}" rx="${n(st.radius, 0)}" fill="${bg || 'none'}" stroke="${stroke || 'none'}" stroke-width="${sw ?? 1}"/>`;
        inner += `<text x="${r2(tx)}" text-anchor="${al}" font-size="${size}" font-weight="${st.bold ? 700 : 400}" ${st.italic ? 'font-style="italic"' : ''} fill="${color}">${lines.map((l, k) => `<tspan x="${r2(tx)}" y="${r2(ty0 + k * lh)}">${esc(l)}</tspan>`).join('')}</text>`;
        break;
      }
      case 'image': {
        const WG = window.ctrl32Widgets; const url = (WG && WG.apiPath ? WG.apiPath('/api/v1/hmi/objects/') : '/api/v1/hmi/objects/') + encodeURIComponent(e.src || '') + '/svg';
        inner = `<image href="${esc(url)}" x="${r2(b.x)}" y="${r2(b.y)}" width="${r2(Math.max(1, b.w))}" height="${r2(Math.max(1, b.h))}" preserveAspectRatio="xMidYMid meet"/>`;
        if (stroke) inner += `<rect x="${r2(b.x)}" y="${r2(b.y)}" width="${r2(b.w)}" height="${r2(b.h)}" fill="none" stroke="${stroke}" stroke-width="${sw ?? 1}"/>`;
        break;
      }
      case 'symbol': {
        const def = S[e.symbol];
        if (!def) { inner = `<rect x="${r2(b.x)}" y="${r2(b.y)}" width="${r2(b.w)}" height="${r2(b.h)}" fill="none" stroke="#dc2626" stroke-dasharray="4 3"/><text x="${r2(cx)}" y="${r2(cy)}" text-anchor="middle" font-size="11" fill="#dc2626">${esc(e.symbol)}</text>`; break; }
        const base = baseFill || DEF.body; let body = base;
        if (st.shade) { defs += shadeDef(uid + 'g', base, def.fit === 'none' && b.w > b.h * 1.4); body = `url(#${uid}g)`; }
        const c = { body, base, line: stroke || DEF.line, sw: sw ?? 1.5, state, level: dv.level ?? null, medium: col(st.medium, DEF.medium), spin: !!dv.spin, speed: dv.spin ? dv.spin.speed : 1, label: e.text || e.name || '', value: val, unit, uid };
        inner = `<svg x="${r2(b.x)}" y="${r2(b.y)}" width="${r2(Math.max(1, b.w))}" height="${r2(Math.max(1, b.h))}" viewBox="0 0 100 100" preserveAspectRatio="${def.fit === 'none' ? 'none' : 'xMidYMid meet'}" overflow="visible" class="sc-sym" style="width:${r2(Math.max(1, b.w))}px;height:${r2(Math.max(1, b.h))}px;max-width:none">${def.draw(c)}</svg>`;
        break;
      }
    }
    if (st.shadow) inner = `<g filter="drop-shadow(2px 3px 2px rgba(0,0,0,.35))">${inner}</g>`;
    const tf = []; if (dv.dx || dv.dy) tf.push(`translate(${r2(dv.dx || 0)} ${r2(dv.dy || 0)})`); const rot = n(e.r, 0) + (dv.angle || 0); if (rot) tf.push(`rotate(${r2(rot)} ${r2(cx)} ${r2(cy)})`);
    let cls = 'sc-el'; let style = '';
    const spinInner = dv.spin && e.type !== 'symbol' ? ` class="sc-spin" style="animation-duration:${(1 / dv.spin.speed).toFixed(2)}s"` : '';
    if (dv.blink && !dv.blink.color) cls += ' sc-blink';
    if (!env.edit && clickable(e)) cls += ' sc-click';
    if (e.hidden && env.edit) style += ';opacity:.35';
    let frame = '';
    if (dv.blink && dv.blink.color) frame = `<rect x="${r2(b.x - 4)}" y="${r2(b.y - 4)}" width="${r2(b.w + 8)}" height="${r2(b.h + 8)}" rx="4" fill="none" stroke="${dv.blink.color}" stroke-width="3" class="sc-blink"/>`;
    const title = e.name || (d ? `${d.asset} / ${d.key}` : '');
    const tip = title || val != null ? `<title>${esc(title)}${val != null ? ': ' + esc(val) + (unit ? ' ' + esc(unit) : '') : ''}${badQ ? ' (quality: ' + esc(x.quality) + ')' : ''}</title>` : '';
    return `<g data-eid="${esc(e.id)}" class="${cls}"${tf.length ? ` transform="${tf.join(' ')}"` : ''}${op !== 1 ? ` opacity="${r2(op)}"` : ''}${style ? ` style="${style}"` : ''}${e.hidden && !env.edit ? ' display="none"' : ''}>${defs ? `<defs>${defs}</defs>` : ''}${tip}<g${spinInner}>${inner}</g>${frame}</g>`;
  }
  const values = (env, tag) => (tag ? env.values[tag] : null);

  // A standalone symbol preview (palette, catalogue).
  let previewSeq = 0;
  function symbolSVG(id, o) {
    const def = S[id]; if (!def) return '';
    o = o || {}; const base = o.fill || SCHEMES.classic[o.state] || DEF.body;
    const c = { body: base, base, line: DEF.line, sw: 1.5, state: o.state || null, level: o.level ?? null, medium: DEF.medium, spin: false, speed: 1, label: o.label || '', value: o.value ?? null, unit: o.unit || '', uid: 'pv' + (++previewSeq) + '-' };
    return `<svg viewBox="0 0 100 100" preserveAspectRatio="xMidYMid meet" class="sc-sym" style="width:100%;height:100%;display:block">${def.draw(c)}</svg>`;
  }

  // ---- mount: live layer on a process picture ------------------------------------------------------------
  let mountSeq = 0;
  function tagsOf(elements) { const set = new Set(); for (const e of elements || []) { if (e.tag) set.add(e.tag); for (const d of e.dyn || []) if (d.tag) set.add(d.tag); } return set; }
  // mount(host, spec, {edit}) → controller {refresh(), draw(), setElements(list), tags(), destroy()}
  function mount(host, spec, opts) {
    opts = opts || {}; const W0 = (spec.layout && spec.layout.width) || 1600, H0 = (spec.layout && spec.layout.height) || 900;
    const uid = 's' + (++mountSeq) + '-';
    host.innerHTML = `<svg class="sc-layer notranslate" xmlns="${NS}" width="${W0}" height="${H0}" viewBox="0 0 ${W0} ${H0}" style="width:${W0}px;height:${H0}px;max-width:none"><rect class="sc-bg" x="0" y="0" width="${W0}" height="${H0}" fill="${col(spec.layout && spec.layout.bg, 'none')}"/><g class="sc-root"></g></svg>`;
    const root = host.querySelector('.sc-root'); let elements = spec.elements || []; let cache = []; let vals = {}; let dsMap = {};
    const env = () => ({ values: vals, ds: dsMap, scheme: spec.scheme || 'classic', uid, edit: !!opts.edit });
    function draw() {
      const E = env(); const html = elements.map((e) => renderEl(e, E));
      if (cache.length !== html.length || root.childElementCount !== html.length) { root.innerHTML = html.join(''); cache = html; return; }
      for (let i = 0; i < html.length; i++) {
        if (html[i] === cache[i]) continue;
        const tmp = document.createElementNS(NS, 'g'); tmp.innerHTML = html[i]; const node = tmp.firstElementChild; if (node) root.children[i].replaceWith(node);
      }
      cache = html;
    }
    async function refresh() {
      const tags = [...tagsOf(elements)]; const WG = window.ctrl32Widgets;
      if (tags.length && WG) {
        try { const all = await WG.datastreams(); dsMap = Object.fromEntries(all.map((d) => [d.id, d])); } catch (_) { }
        try { vals = await WG.latest(tags); } catch (_) { }
      }
      draw();
    }
    if (!opts.edit) {
      root.addEventListener('click', (ev) => { const g = ev.target.closest('[data-eid]'); if (!g) return; const e = elements.find((x) => x.id === g.dataset.eid); if (e && clickable(e)) act(e, { values: vals, ds: dsMap }); });
    }
    draw();
    return { refresh, draw, root, svg: host.querySelector('svg'), setElements(list) { elements = list; }, setSpec(s) { spec = s; host.querySelector('.sc-bg').setAttribute('fill', col(spec.layout && spec.layout.bg, 'none')); }, tags: () => tagsOf(elements), values: () => vals, datastreams: () => dsMap };
  }

  // ---- events: faceplate, trend, screen change, command ------------------------------------------------
  const isPublic = () => !!(window.ctrl32Widgets && window.ctrl32Widgets.isPublic && window.ctrl32Widgets.isPublic());
  const canCmd = () => !isPublic() && window.ctrl32 && typeof ctrl32.can === 'function' && ctrl32.can('device.command');
  function act(e, live) {
    const list = e.on && e.on.length ? e.on : [{ act: 'faceplate' }];
    const ev = list[0];
    if (ev.act === 'screen') { if (!isPublic()) location.href = '/dashboards/' + encodeURIComponent(ev.target); return; }
    if (ev.act === 'command') { commandDialog(e, ev); return; }
    faceplate(e, live, ev.act === 'trend');
  }
  function dialog() {
    let d = document.querySelector('dialog.sc-dlg');
    if (!d) { d = document.createElement('dialog'); d.className = 'sc-dlg'; document.body.appendChild(d); d.addEventListener('click', (ev) => { if (ev.target === d) d.close(); }); }
    return d;
  }
  async function sendCommand(out, device, method, key, value, reason) {
    out.innerHTML = '<span class="muted">sending…</span>';
    try {
      const r = await ctrl32.api('POST', '/api/v1/devices/' + device + '/commands', { method: method || 'write', args: { key, value }, reason });
      out.innerHTML = r.status === 'acked' ? '<span class="badge ok">acked</span>' : r.status === 'pending' ? '<span class="badge warn">pending</span> <span class="muted">waiting for approval by another user</span>' : `<span class="badge">${esc(r.status)}</span>`;
    } catch (x) { out.innerHTML = `<span class="badge bad">${esc(x.message)}</span>`; }
  }
  function commandDialog(e, ev) {
    const d = dialog();
    if (!e.device) { d.innerHTML = `<div class="form"><h2 style="margin-top:0">${esc(e.name || 'Command')}</h2><p class="muted">No device is bound to this object.</p><div class="actions"><button class="btn" type="button" data-close>Close</button></div></div>`; }
    else if (!canCmd()) { d.innerHTML = `<div class="form"><h2 style="margin-top:0">${esc(e.name || 'Command')}</h2><p class="muted">${isPublic() ? 'read-only' : 'no permission to send commands'}</p><div class="actions"><button class="btn" type="button" data-close>Close</button></div></div>`; }
    else {
      d.innerHTML = `<form class="form" method="dialog"><h2 style="margin-top:0">${esc(e.name || 'Command')}</h2><p>${esc(ev.key)} → <b>${esc(String(ev.value ?? ''))}</b></p><label>Reason (audit trail) <input name="reason" required maxlength="200"></label><div class="sc-out" style="min-height:18px"></div><div class="actions"><button class="btn primary" type="submit">Send</button><button class="btn" type="button" data-close>Close</button></div></form>`;
      d.querySelector('form').onsubmit = (x) => { x.preventDefault(); if (ev.confirm !== false && !window.confirm(`${ev.key} → ${ev.value}?`)) return; sendCommand(d.querySelector('.sc-out'), e.device, ev.method, ev.key, ev.value, x.target.reason.value.trim()); };
    }
    for (const b of d.querySelectorAll('[data-close]')) b.onclick = () => d.close();
    d.showModal();
  }
  async function faceplate(e, live, trendOnly) {
    const d = dialog(); const tag = e.tag; const ds = tag ? live.ds[tag] : null; const x = tag ? live.values[tag] : null;
    const unit = e.unit ?? (ds ? ds.unit : ''); const v = x ? (numVal(x) ?? x.value_json) : null;
    const title = e.name || (ds ? `${ds.asset} / ${ds.key}` : e.symbol || e.type);
    const ctl = !trendOnly && e.device && e.key;
    d.innerHTML = `<div class="sc-fp"><div class="sc-fp-head"><b>${esc(title)}</b>${ds ? `<span class="muted">${esc(ds.asset)} / ${esc(ds.key)}</span>` : ''}<span class="grow"></span><button class="btn small ghost" type="button" data-close title="Close">✕</button></div>
      ${tag ? `<div class="sc-fp-val"><span class="sc-fp-big">${v == null ? '—' : esc(fmtVal(v, e.decimals))}</span> <span class="muted">${esc(unit)}</span> ${x ? `<span class="badge ${x.quality === 'ok' ? 'ok' : 'warn'}">${esc(x.quality || '')}</span>` : ''}<div class="muted" style="font-size:12px">${x && x.measured_at ? esc(new Date(x.measured_at).toLocaleString(document.documentElement.lang || undefined)) : 'no value yet'}</div></div>
      <div class="row sc-fp-range">${['1h', '6h', '24h', '7d'].map((r) => `<button class="btn small${r === '6h' ? ' primary' : ''}" type="button" data-r="${r}">${r}</button>`).join('')}</div>
      <div class="sc-fp-chart"></div>` : '<p class="muted">No datastream is bound to this object.</p>'}
      ${ctl ? `<div class="sc-fp-ctl"><h3>Operation</h3>${canCmd() ? `<div class="row" style="align-items:center;gap:6px;flex-wrap:wrap"><button class="btn small" type="button" data-v="true">Start / On</button><button class="btn small" type="button" data-v="false">Stop / Off</button><input class="sc-sp" type="number" step="any" placeholder="value" style="width:110px"><button class="btn small primary" type="button" data-sp>Set</button></div><input class="sc-reason" placeholder="Reason (audit trail)" maxlength="200" style="width:100%;margin-top:6px"><div class="sc-out" style="min-height:18px;font-size:12px"></div>` : `<p class="muted">${isPublic() ? 'read-only' : 'no permission to send commands'}</p>`}</div>` : ''}</div>`;
    for (const b of d.querySelectorAll('[data-close]')) b.onclick = () => d.close();
    const chartBox = d.querySelector('.sc-fp-chart');
    const drawTrend = async (range) => {
      const WG = window.ctrl32Widgets; if (!chartBox || !WG) return; chartBox.innerHTML = '<span class="muted">loading…</span>';
      try { const pts = await WG.series(tag, range); const t1 = Date.now(), t0 = t1 - WG.RANGE_MS[range]; WG.renderChart(chartBox, [{ label: title, unit, points: pts }], { t0, t1, decimals: e.decimals ?? 1, height: 220 }); } catch (x) { chartBox.innerHTML = `<span class="muted">${esc(x.message)}</span>`; }
    };
    for (const b of d.querySelectorAll('[data-r]')) b.onclick = () => { d.querySelectorAll('[data-r]').forEach((k) => k.classList.toggle('primary', k === b)); drawTrend(b.dataset.r); };
    if (ctl && canCmd()) {
      const send = (value) => { const reason = d.querySelector('.sc-reason').value.trim(); const out = d.querySelector('.sc-out'); if (!reason) { out.innerHTML = '<span style="color:#d97706">reason is required</span>'; d.querySelector('.sc-reason').focus(); return; } if (!window.confirm(`${e.key} → ${value}?`)) return; sendCommand(out, e.device, 'write', e.key, value, reason); };
      for (const b of d.querySelectorAll('[data-v]')) b.onclick = () => send(b.dataset.v === 'true');
      d.querySelector('[data-sp]').onclick = () => { const v = parseFloat(d.querySelector('.sc-sp').value); if (isNaN(v)) { d.querySelector('.sc-out').innerHTML = '<span style="color:#d97706">enter a number</span>'; return; } send(v); };
    }
    d.showModal();
    if (tag) drawTrend('6h');
  }

  // openTag: faceplate or trend of one datastream outside a picture (widget click actions, spec 002).
  async function openTag(tag, o) {
    o = o || {}; const WG = window.ctrl32Widgets; const [all, lv] = await Promise.all([WG.datastreams(), WG.latest([tag]).catch(() => ({}))]);
    const d = all.find((x) => x.id === tag); faceplate({ tag, name: o.name, device: o.device, key: o.key, decimals: o.decimals, type: 'widget' }, { ds: d ? { [tag]: d } : {}, values: lv }, !!o.trendOnly);
  }

  window.ctrl32Scada = { openTag, mount, renderEl, bbox, symbolSVG, symbols: S, SYMBOL_IDS, CATS, SCHEMES, STATES, tagsOf, evalDyn, fmtVal, clickable, DEF };
})();
