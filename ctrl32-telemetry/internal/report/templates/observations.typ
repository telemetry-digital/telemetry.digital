// Built-in template: observations of one datastream over a period.
#import "layout.typ": *
#let d = load()
#show: page-setup.with(d)

#title-block(d)
#summary-block(d)
#chart-block(d)
#data-table(d)
#end-marker()
