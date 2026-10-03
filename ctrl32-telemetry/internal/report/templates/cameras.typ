// Built-in template: camera inventory (spec 028) — one card per camera on A4 landscape, for the customer's file.
// A document with passwords (scope.confidential) is marked in the header, the footer and above the cards.
#import "layout.typ": *
#let d = load()
#let conf = type(d.scope) == dictionary and d.scope.at("confidential", default: false) == true
#let mark = "CONFIDENTIAL — contains passwords"
#let red = rgb("#b00020")
#let product = if "product" in d and d.product != none and d.product != "" { d.product } else { "ctrl32 · telemetry" }

#set page(
  paper: "a4",
  flipped: true,
  margin: (x: 1.4cm, y: 1.7cm),
  header: [
    #set text(size: 8pt, fill: luma(90))
    #grid(columns: (auto, 1fr, auto), column-gutter: 8pt, align: horizon,
      if d.logo != none and d.logo != "" { image(d.logo, height: 18pt) } else { [] },
      [#d.org],
      if conf { text(fill: red, weight: "bold")[#mark] } else { [#d.title] })
    #line(length: 100%, stroke: 0.5pt + luma(180))
  ],
  footer: context [
    #set text(size: 8pt, fill: luma(90))
    #line(length: 100%, stroke: 0.5pt + luma(180))
    #grid(columns: (1fr, auto),
      [#if conf [#text(fill: red, weight: "bold")[#mark] · ]#product · generated #d.generated_at by #d.generated_by],
      [Page #counter(page).display() of #counter(page).final().first()])
  ],
)
#set text(font: "Liberation Sans", size: 8.5pt, lang: d.locale)
#set par(justify: false)

#text(size: 18pt, weight: "bold")[#d.title]
#linebreak()
#if d.subtitle != none and d.subtitle != "" { text(size: 10pt, fill: luma(80))[#d.subtitle] }
#v(6pt)
#if conf {
  block(fill: rgb("#fdecea"), stroke: 0.6pt + red, inset: 8pt, radius: 3pt, width: 100%)[
    #text(fill: red, weight: "bold")[#mark.] Keep this document like the keys to the building and destroy copies you no longer need.
  ]
  v(4pt)
}
#summary-block(d)

// the head of a card: name, location, group, state; the rest as label–value pairs, three to a row
#let head = ("name", "location", "group", "enabled", "state")
#let secret = ("password", "relay_secret")
#let first-key = if d.columns.len() > 0 { d.columns.first().key } else { "" }
#for r in d.rows {
  let title-key = if "name" in r { "name" } else { first-key }
  let title = cell(r.at(title-key, default: none))
  let place = (cell(r.at("location", default: none)), cell(r.at("group", default: none))).filter(x => x != "").join(" · ")
  let status = (cell(r.at("enabled", default: none)), cell(r.at("state", default: none))).filter(x => x != "")
  let pairs = d.columns.filter(c => c.key != title-key and c.key not in head)
    .map(c => (c.label, cell(r.at(c.key, default: none)), c.key))
    .filter(p => p.at(1) != "")
  block(breakable: false, width: 100%, stroke: 0.6pt + luma(170), radius: 3pt, inset: 7pt, below: 6pt)[
    #grid(columns: (auto, 1fr, auto), column-gutter: 8pt, align: (left + horizon, left + horizon, right + horizon),
      text(size: 11pt, weight: "bold")[#title],
      text(fill: luma(70))[#place],
      text(size: 8pt, fill: luma(60))[#(if status.len() > 0 { "enabled: " + status.join(" · ") } else { "" })])
    #v(3pt)
    #grid(columns: (auto, 1fr, auto, 1fr, auto, 1fr), column-gutter: 5pt, row-gutter: 3.5pt,
      ..pairs.map(p => (text(fill: luma(90), size: 7.5pt)[#p.at(0)],
        if p.at(2) in secret { text(size: 8pt, font: "DejaVu Sans Mono", weight: "bold", fill: red)[#p.at(1)] } else { text(size: 8pt)[#p.at(1)] })).flatten())
  ]
}
#end-marker()
