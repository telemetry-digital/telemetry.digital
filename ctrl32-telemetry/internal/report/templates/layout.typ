// ctrl32 telemetry — shared report layout. Imported by every template: #import "layout.typ": *
// Data contract: see internal/report/report.go (Data). Templates read `data.json` themselves.
// The block designer (internal/report/spec.go) generates templates from these helpers only.

#let load() = json("data.json")

#let cell(v) = {
  if v == none { "" } else if type(v) == str { v } else if type(v) == bool { if v { "true" } else { "false" } } else if type(v) == int or type(v) == float { str(v) } else { repr(v) }
}

// tok(d, "scope.asset") looks up a dotted path in the data; missing = "".
#let tok(d, path) = {
  let cur = d
  for part in path.split(".") {
    if type(cur) == dictionary and part in cur { cur = cur.at(part) } else { return "" }
  }
  cell(cur)
}

#let page-setup(d, body, paper: "a4", landscape: false, font: "Libertinus Serif", size: 9.5pt) = {
  set page(
    paper: paper,
    flipped: landscape,
    margin: (x: 1.6cm, y: 1.8cm),
    header: [
      #set text(size: 8pt, fill: luma(90))
      #grid(columns: (auto, 1fr, auto), column-gutter: 8pt, align: horizon,
        if d.logo != none and d.logo != "" { image(d.logo, height: 18pt) } else { [] },
        [#d.org],
        [#d.title])
      #line(length: 100%, stroke: 0.5pt + luma(180))
    ],
    footer: context [
      #set text(size: 8pt, fill: luma(90))
      #line(length: 100%, stroke: 0.5pt + luma(180))
      #grid(columns: (1fr, auto),
        [#(if "product" in d and d.product != none and d.product != "" { d.product } else { "ctrl32 · telemetry" }) · generated #d.generated_at by #d.generated_by · audit head \##d.audit.seq #d.audit.hash.slice(0, 16)…],
        [Page #counter(page).display() of #counter(page).final().first()])
    ],
  )
  set text(font: font, size: size, lang: d.locale)
  set par(justify: false)
  body
}

#let title-block(d, title: none, subtitle: none, show-period: true, show-scope: true) = {
  let t = if title == none { d.title } else { title }
  let s = if subtitle == none { d.subtitle } else { subtitle }
  text(size: 18pt, weight: "bold")[#t]
  linebreak()
  if s != none and s != "" {
    text(size: 10pt, fill: luma(80))[#s]
    v(4pt)
  }
  let parts = ()
  if show-period { parts.push([Period: #d.period.from → #d.period.to]) }
  if show-scope { parts += d.scope.pairs().map(((k, v)) => [*#k*: #cell(v)]) }
  if parts.len() > 0 { text(size: 9pt)[#parts.join([ · ])] }
  v(8pt)
}

#let summary-block(d, keys: ()) = {
  let pairs = d.summary.pairs().filter(((k, v)) => keys.len() == 0 or k in keys)
  if pairs.len() > 0 {
    let items = pairs.map(((k, v)) => box(inset: (x: 8pt, y: 5pt), stroke: 0.5pt + luma(200), radius: 3pt)[#text(size: 8pt, fill: luma(90))[#k] #linebreak() #text(size: 11pt, weight: "bold")[#cell(v)]])
    stack(dir: ltr, spacing: 6pt, ..items)
    v(8pt)
  }
}

#let chart-block(d, caption: "") = {
  if d.chart != none and d.chart != "" {
    image(d.chart, width: 100%)
    if caption != "" { align(center, text(size: 8pt, fill: luma(90))[#caption]) }
    v(6pt)
  }
}

#let text-block(body, style: "normal") = {
  let paras = body.split("\n").filter(p => p.trim() != "").map(p => [#p])
  let content = paras.join(parbreak())
  if style == "note" {
    block(inset: 8pt, fill: luma(245), stroke: 0.5pt + luma(200), radius: 3pt, width: 100%, text(size: 9pt)[#content])
  } else if style == "warning" {
    block(inset: 8pt, fill: rgb("#fff4e5"), stroke: 0.5pt + rgb("#e0a800"), radius: 3pt, width: 100%, text(size: 9pt)[#content])
  } else {
    text(size: 9.5pt)[#content]
  }
  v(6pt)
}

#let data-table(d, max-rows: 3000, columns: (), font-size: 8pt) = {
  let cols = d.columns.filter(c => columns.len() == 0 or c.key in columns)
  if cols.len() == 0 { cols = d.columns }
  let aligns = cols.map(c => if c.align == "right" { right } else if c.align == "center" { center } else { left })
  let header = cols.map(c => text(weight: "bold", size: font-size + 0.5pt)[#c.label])
  let rows = d.rows.slice(0, calc.min(d.rows.len(), int(max-rows)))
  let cells = rows.map(r => cols.map(c => text(size: font-size)[#cell(r.at(c.key, default: none))])).flatten()
  table(
    columns: cols.len(),
    align: aligns,
    stroke: (x: none, y: 0.3pt + luma(200)),
    inset: (x: 5pt, y: 3pt),
    fill: (_, y) => if y == 0 { luma(235) } else { none },
    ..header, ..cells,
  )
  if d.rows.len() > int(max-rows) {
    v(4pt)
    text(size: 8pt, fill: red)[Table truncated to #int(max-rows) of #d.rows.len() rows — use the CSV/JSONL export for the complete data.]
  }
}

// Per-day samples / min / max / average from the rows (values may be numbers or numeric strings).
#let daily-table(d, value-key: "value", time-key: "measured_at", decimals: 2) = {
  let numre = regex("^-?[0-9]+(\\.[0-9]+)?$")
  let days = (:)
  for r in d.rows {
    let t = cell(r.at(time-key, default: none))
    let v = r.at(value-key, default: none)
    let f = none
    if type(v) == int or type(v) == float { f = float(v) } else if type(v) == str and v.match(numre) != none { f = float(v) }
    if t.len() >= 10 and f != none {
      let day = t.slice(0, 10)
      if day in days {
        let e = days.at(day)
        days.insert(day, (n: e.n + 1, min: calc.min(e.min, f), max: calc.max(e.max, f), sum: e.sum + f))
      } else {
        days.insert(day, (n: 1, min: f, max: f, sum: f))
      }
    }
  }
  let fmt(x) = str(calc.round(x, digits: int(decimals)))
  if days.len() == 0 {
    text(size: 8pt, fill: luma(90))[No numeric values in column "#value-key".]
  } else {
    let header = ([*Day*], [*Samples*], [*Min*], [*Max*], [*Average*])
    let cells = days.pairs().map(((day, e)) => ([#day], [#e.n], [#fmt(e.min)], [#fmt(e.max)], [#fmt(e.sum / e.n)])).flatten()
    table(columns: 5, align: (left, right, right, right, right), stroke: (x: none, y: 0.3pt + luma(200)), inset: (x: 5pt, y: 3pt),
      fill: (_, y) => if y == 0 { luma(235) } else { none }, ..header, ..cells)
  }
  v(6pt)
}

#let signatures-block(roles: ("Prepared by", "Reviewed by", "Approved by"), date: true) = {
  v(16pt)
  grid(columns: roles.len(), column-gutter: 14pt, ..roles.map(r => [
    #v(18pt)
    #line(length: 100%, stroke: 0.5pt + luma(120))
    #text(size: 8pt, fill: luma(90))[#r]
    #if date [ #linebreak() #text(size: 8pt, fill: luma(90))[Date: \_\_\_\_\_\_\_\_\_\_\_\_] ]
  ]))
  v(6pt)
}

#let end-marker() = {
  v(10pt)
  align(center, text(size: 8pt, fill: luma(90))[— end of report —])
}
