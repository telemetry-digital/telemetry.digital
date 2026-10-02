package httpapi

// Dashboard widgets of object counting (spec 029), rendered by static/widgets-counting.js. Each binds one datastream
// of a counting area (of the area, an entrance or a speed section): the widget shows the whole area from the
// summary API, so the user picks the area with the usual datastream picker. The occupancy alone also works with the
// ordinary value, gauge and chart widgets on <group>_occupancy.

var (
	pCountGroup = ScreenParam{Name: "group", Label: "Counts", Kind: "select", Default: "people", Options: []string{"people", "vehicles", "cars", "trucks", "buses", "motorcycles", "bicycles"}}
	pCountDay   = ScreenParam{Name: "day", Label: "Day", Kind: "select", Default: "today", Options: []string{"today", "yesterday"}}
)

var countingWidgets = []WidgetDef{
	{Type: "counting_occupancy", Label: "Occupancy", Group: "values", Help: "Occupancy of a counting area now with its capacity, and today's arrivals and departures. Bind any datastream of the area.", Binds: dsBind, MinBinds: 1, Default: [2]int{3, 2},
		Params: []ScreenParam{pTitle, pCountGroup, {Name: "show_today", Label: "Show today's in and out", Kind: "bool", Default: true}}},
	{Type: "counting_entrances", Label: "In and out per entrance", Group: "charts", Help: "Today's arrivals and departures of each entrance of a counting area as bars. Bind any datastream of the area.", Binds: dsBind, MinBinds: 1, Default: [2]int{6, 3},
		Params: []ScreenParam{pTitle, pCountGroup, pCountDay}},
	{Type: "counting_share", Label: "Share per entrance", Group: "charts", Help: "Today's share of each entrance in the arrivals (or departures) of a counting area. Bind any datastream of the area.", Binds: dsBind, MinBinds: 1, Default: [2]int{4, 3},
		Params: []ScreenParam{pTitle, pCountGroup, pCountDay, {Name: "direction", Label: "Direction", Kind: "select", Default: "in", Options: []string{"in", "out"}}}},
	{Type: "vehicle_speeds", Label: "Vehicle speeds", Group: "lists", Help: "The last vehicles through a speed section with their speed, today's count, average, maximum, vehicles over the limit and a histogram. Bind the section's vehicle_speed datastream.", Binds: dsBind, MinBinds: 1, Default: [2]int{6, 4},
		Params: []ScreenParam{pTitle, pCountDay, {Name: "rows", Label: "Vehicles listed", Kind: "number", Default: 6, Min: fptr(1), Max: fptr(10)}, {Name: "histogram", Label: "Histogram", Kind: "bool", Default: true}}},
}

// init runs before the catalog is completed in widget_params.go (files initialise in name order).
func init() { moreWidgets = append(moreWidgets, countingWidgets...) }
