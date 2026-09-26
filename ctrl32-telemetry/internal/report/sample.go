package report

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// SampleData builds representative data for template previews and compile checks.
func SampleData(kind, locale string) Data {
	now := time.Now().UTC()
	d := Data{
		Kind: kind, Title: strings.ToUpper(kind[:1]) + kind[1:] + " report (preview)", Subtitle: "Sample data — not a record",
		Org: "Sample organization", Locale: locale, GeneratedAt: now.Format(time.RFC3339), GeneratedBy: "preview",
		Period: Period{From: now.Add(-24 * time.Hour).Format(time.RFC3339), To: now.Format(time.RFC3339)},
		Scope:  map[string]any{"asset": "Fridge 1", "datastream": "temp", "unit": "°C"}, Summary: map[string]any{}, Audit: AuditHead{Seq: 1234, Hash: strings.Repeat("ab", 32)},
		System: map[string]any{"version": "preview"},
	}
	switch kind {
	case "alarms":
		d.Columns = []Column{{"raised_at", "Raised", "left"}, {"severity", "Severity", "left"}, {"state", "State", "left"}, {"key", "Datastream", "left"}, {"value", "Value", "right"}, {"reason", "Ack reason", "left"}}
		d.Rows = []map[string]any{{"raised_at": now.Add(-3 * time.Hour).Format(time.RFC3339), "severity": "action", "state": "cleared", "key": "temp", "value": "8.4", "reason": "door open for restocking"}}
	case "audit":
		d.Columns = []Column{{"seq", "#", "right"}, {"ts", "Time", "left"}, {"actor_name", "Actor", "left"}, {"action", "Action", "left"}, {"entity_type", "Entity", "left"}, {"reason", "Reason", "left"}, {"hash", "Hash", "left"}}
		d.Rows = []map[string]any{{"seq": "1", "ts": now.Format(time.RFC3339), "actor_name": "admin", "action": "alarm_rule.update", "entity_type": "alarm_rule", "reason": "limits per storage class", "hash": strings.Repeat("cd", 8) + "…"}}
	default:
		d.Columns = []Column{{"measured_at", "Measured", "left"}, {"received_at", "Received", "left"}, {"value", "Value", "right"}, {"quality", "Quality", "left"}}
		for i := 0; i < 24; i++ {
			t := now.Add(time.Duration(i-24) * time.Hour)
			v := 4.5 + 0.8*math.Sin(float64(i)/4)
			q := "ok"
			if i == 5 {
				q = "backfilled"
			}
			d.Rows = append(d.Rows, map[string]any{"measured_at": t.Format(time.RFC3339), "received_at": t.Add(2 * time.Second).Format(time.RFC3339), "value": fmt.Sprintf("%.2f", v), "quality": q})
		}
		d.Summary = map[string]any{"count": 24, "min": "3.70", "max": "5.30", "avg": "4.50", "gaps": 0, "alarms": 0}
		d.Chart = "chart.svg"
	}
	return d
}

// SampleChartSVG returns a small chart for previews.
func SampleChartSVG() string {
	pts := make([]Point, 0, 24)
	now := time.Now()
	for i := 0; i < 24; i++ {
		pts = append(pts, Point{T: now.Add(time.Duration(i-24) * time.Hour), V: 4.5 + 0.8*math.Sin(float64(i)/4)})
	}
	return ChartSVG(pts, "°C", nil, nil)
}
