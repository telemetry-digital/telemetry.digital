package httpapi

// Dashboard widgets of trends and batches (spec 043), rendered by static/widgets-trend.js.
//
// trend: the trend control of the Trends page inside a dashboard or a process picture — the bound datastreams as
// curves on axes by unit, a live window, dragging, the wheel and rulers. batches: the batch that runs on a unit and the
// last batches, each a link to its record.

var trendWidgets = []WidgetDef{
	{Type: "trend", Label: "Trend", Group: "charts", Help: "Curves of the bound datastreams on one time axis, with an axis per unit: drag to move in time, turn the wheel to zoom, click to place a ruler. For a fixed set of curves on a dashboard; the Trends page adds saved views, a second ruler and the export.",
		Binds: dsBind, MinBinds: 1, Default: [2]int{8, 4},
		Params: []ScreenParam{pTitle,
			{Name: "window", Label: "Time shown", Kind: "select", Default: "1h", Options: []string{"5m", "15m", "1h", "6h", "8h", "12h", "24h", "7d", "30d"}},
			{Name: "tools", Label: "Buttons for the time and Live", Kind: "bool", Default: true},
			{Name: "table", Label: "Table with the value at the ruler, minimum, maximum and average", Kind: "bool", Default: false},
			{Name: "limits", Label: "Alarm limits", Kind: "bool", Default: false}}},
	{Type: "batches", Label: "Batches", Group: "lists", Help: "The batch that runs on a unit now — its number, code, recipe and how long it has run — and the last batches. A click opens the record of a batch.",
		Default: [2]int{6, 3},
		Params: []ScreenParam{pTitle,
			{Name: "batch_unit", Label: "Unit (empty = all units)", Kind: "text"},
			{Name: "rows", Label: "Batches listed", Kind: "number", Default: 6, Min: fptr(0), Max: fptr(30)},
			{Name: "running", Label: "Show the running batch on top", Kind: "bool", Default: true}}},
}

// init runs before the catalog is completed in widget_params.go (files initialise in name order).
func init() { moreWidgets = append(moreWidgets, trendWidgets...) }
