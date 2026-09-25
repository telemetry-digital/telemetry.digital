// Built-in template: alarm log over a period.
#import "layout.typ": *
#let d = load()
#show: page-setup.with(d)

#title-block(d)
#summary-block(d)
#data-table(d)
#end-marker()
