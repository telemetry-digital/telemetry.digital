// ctrl32 telemetry — renderer of the public front page (spec 012). The page is data (blocks), never HTML: every text
// goes in through textContent, links and colours were validated by the server, icons come from the vendored catalogue.
//
//   ctrl32Landing.render(el, spec, {edit, selected, onSelect(id), onEdit(id, path, value)})
// In edit mode texts are editable in place (plain text) and clicks select blocks instead of following links.
(function () {
  const h = (tag, cls, text) => { const e = document.createElement(tag); if (cls) e.className = cls; if (text !== undefined && text !== null) e.textContent = text; return e; };
  const icon = (name) => {
    const d = window.ctrl32Icons && window.ctrl32Icons[name];
    if (!d) return null;
    const s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    s.setAttribute('viewBox', '0 0 24 24'); s.setAttribute('fill', 'none'); s.setAttribute('stroke', 'currentColor'); s.setAttribute('stroke-width', '2');
    s.setAttribute('stroke-linecap', 'round'); s.setAttribute('stroke-linejoin', 'round'); s.setAttribute('class', 'lp-icon');
    s.innerHTML = d; // vendored markup keyed by a name the server checked against the catalogue
    return s;
  };
  const imgURL = (id) => id === 'logo' ? ((window.ctrl32 && ctrl32.brand && ctrl32.brand.logo_custom) ? ctrl32.brand.logo_url : '/logo.svg') : '/api/v1/landing/assets/' + encodeURIComponent(id);

  // inline text: **bold** only; everything else literal
  function inline(el, s) {
    const parts = String(s || '').split(/(\*\*[^*]+\*\*)/);
    for (const p of parts) { if (/^\*\*[^*]+\*\*$/.test(p)) el.appendChild(h('b', '', p.slice(2, -2))); else if (p) el.appendChild(document.createTextNode(p)); }
    return el;
  }
  // paragraphs separated by blank lines; lines starting with "- " form a list
  function richText(el, s) {
    for (const para of String(s || '').split(/\n\s*\n/)) {
      const lines = para.split('\n');
      if (lines.every((l) => /^\s*-\s+/.test(l))) { const ul = h('ul'); for (const l of lines) ul.appendChild(inline(h('li'), l.replace(/^\s*-\s+/, ''))); el.appendChild(ul); }
      else if (para.trim()) el.appendChild(inline(h('p'), para));
    }
    return el;
  }

  function render(root, spec, opts) {
    opts = opts || {};
    const edit = !!opts.edit;
    root.innerHTML = '';
    const page = h('div', 'lp lp-w-' + ((spec.theme && spec.theme.width) || 'normal'));
    const th = spec.theme || {};
    if (th.background) page.style.setProperty('--lp-bg', th.background);
    if (th.text) page.style.setProperty('--lp-fg', th.text);
    if (th.accent) page.style.setProperty('--lp-accent', th.accent);
    root.appendChild(page);

    // an editable text element: plain text only, reported back with its path in the block
    const txt = (tag, cls, b, path, value, rich) => {
      if (!edit) {
        if (rich) return richText(h('div', cls), value);
        return inline(h(tag, cls), value);
      }
      const e = h(tag, cls + ' lp-edit', value || '');
      e.contentEditable = 'plaintext-only';
      if (e.contentEditable !== 'plaintext-only') e.contentEditable = 'true';
      const PH = { title: 'Title', subtitle: 'Subtitle', text: 'Text', label: 'Label', value: 'Value' };
      const key = path.split('.').pop();
      e.dataset.placeholder = window.ctrl32 && ctrl32.t ? ctrl32.t(PH[key] || key) : key;
      if (rich) e.style.whiteSpace = 'pre-wrap';
      e.addEventListener('input', () => opts.onEdit && opts.onEdit(b.id, path, e.innerText.replace(/\n$/, '')));
      e.addEventListener('keydown', (ev) => { if (!rich && ev.key === 'Enter') { ev.preventDefault(); e.blur(); } });
      return e;
    };
    const buttons = (b) => {
      const row = h('div', 'lp-buttons');
      (b.buttons || []).forEach((bt, i) => {
        const a = h('a', 'btn ' + (bt.style === 'secondary' ? '' : 'primary') + ' lp-btn');
        a.href = bt.href;
        if (/^https:/.test(bt.href)) { a.target = '_blank'; a.rel = 'noopener noreferrer'; }
        if (edit) { a.appendChild(txt('span', '', b, `buttons.${i}.label`, bt.label)); a.addEventListener('click', (e) => e.preventDefault()); }
        else a.textContent = bt.label;
        row.appendChild(a);
      });
      return row;
    };
    const image = (b, cls) => {
      if (!b.image) return edit ? h('div', 'lp-img-empty ' + (cls || ''), '🖼') : null;
      const im = h('img', cls || ''); im.src = imgURL(b.image); im.alt = b.title || ''; im.loading = 'lazy';
      if (b.image === 'logo') im.setAttribute('data-logo', '');
      return im;
    };
    const bg = (el, b) => {
      if (b.background && b.background2) el.style.background = `linear-gradient(135deg, ${b.background}, ${b.background2})`;
      else if (b.background) el.style.background = b.background;
      if (b.text_color) el.style.color = b.text_color;
    };

    for (const b of spec.blocks || []) {
      const sec = h('section', `lp-block lp-${b.type} lp-size-${b.size || 'medium'}${b.align === 'center' ? ' lp-center' : ''}`);
      sec.dataset.id = b.id;
      bg(sec, b);
      const inner = h('div', 'lp-inner');
      sec.appendChild(inner);
      switch (b.type) {
        case 'hero': {
          const img = image(b, 'lp-hero-img');
          const body = h('div', 'lp-hero-body');
          body.appendChild(txt('h1', 'lp-title', b, 'title', b.title));
          if (b.subtitle || edit) body.appendChild(txt('p', 'lp-sub', b, 'subtitle', b.subtitle));
          body.appendChild(txt('div', 'lp-text', b, 'text', b.text, true));
          body.appendChild(buttons(b));
          if (b.align === 'center') { if (img) inner.appendChild(img); inner.appendChild(body); }
          else { inner.classList.add('lp-cols'); inner.appendChild(body); if (img) inner.appendChild(img); }
          break;
        }
        case 'features': case 'stats': {
          if (b.title || edit) inner.appendChild(txt('h2', 'lp-h2', b, 'title', b.title));
          const grid = h('div', `lp-grid lp-c${b.columns || 3}`);
          (b.items || []).forEach((it, i) => {
            const card = h('div', b.type === 'stats' ? 'lp-stat' : 'lp-card');
            if (b.type === 'stats') {
              card.appendChild(txt('div', 'lp-value', b, `items.${i}.value`, it.value));
              card.appendChild(txt('div', 'lp-label', b, `items.${i}.title`, it.title));
            } else {
              const ic = icon(it.icon); if (ic) card.appendChild(ic);
              card.appendChild(txt('b', 'lp-card-title', b, `items.${i}.title`, it.title));
              card.appendChild(txt('p', 'lp-card-text', b, `items.${i}.text`, it.text));
            }
            grid.appendChild(card);
          });
          inner.appendChild(grid);
          break;
        }
        case 'text':
          if (b.title || edit) inner.appendChild(txt('h2', 'lp-h2', b, 'title', b.title));
          inner.appendChild(txt('div', 'lp-text', b, 'text', b.text, true));
          break;
        case 'image': {
          const fig = h('figure', 'lp-figure');
          const img = image(b); if (img) fig.appendChild(img);
          if (b.title || edit) fig.appendChild(txt('figcaption', '', b, 'title', b.title));
          inner.appendChild(fig);
          break;
        }
        case 'split': {
          inner.classList.add('lp-cols');
          if (b.image_side === 'left') inner.classList.add('lp-img-left');
          const body = h('div', 'lp-split-body');
          body.appendChild(txt('h2', 'lp-h2', b, 'title', b.title));
          body.appendChild(txt('div', 'lp-text', b, 'text', b.text, true));
          body.appendChild(buttons(b));
          inner.appendChild(body);
          const img = image(b, 'lp-split-img'); if (img) inner.appendChild(img);
          break;
        }
        case 'cta':
          inner.appendChild(txt('h2', 'lp-h2', b, 'title', b.title));
          inner.appendChild(txt('div', 'lp-text', b, 'text', b.text, true));
          inner.appendChild(buttons(b));
          break;
        case 'contact': {
          if (b.title || edit) inner.appendChild(txt('h2', 'lp-h2', b, 'title', b.title));
          const list = h('div', 'lp-contact');
          if (b.email) { const a = h('a', '', b.email); a.href = 'mailto:' + b.email; list.appendChild(a); }
          if (b.phone) { const a = h('a', '', b.phone); a.href = 'tel:' + b.phone.replace(/[^0-9+]/g, ''); list.appendChild(a); }
          if (b.address) list.appendChild(h('span', 'lp-address', b.address));
          inner.appendChild(list);
          if (b.text || edit) inner.appendChild(txt('div', 'lp-text', b, 'text', b.text, true));
          break;
        }
        case 'spacer':
          inner.appendChild(h('hr', 'lp-hr'));
          break;
      }
      if (edit) {
        if (b.id === opts.selected) sec.classList.add('lp-selected');
        sec.addEventListener('click', () => opts.onSelect && opts.onSelect(b.id));
      }
      page.appendChild(sec);
    }
    return page;
  }
  window.ctrl32Landing = { render };
})();
