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
		Org: "Sample organization", Product: "ctrl32 · telemetry", Locale: locale, GeneratedAt: now.Format(time.RFC3339), GeneratedBy: "preview",
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
	case "cameras":
		d.Title, d.Subtitle = "Camera inventory (preview)", "Sample organization · 2 cameras"
		d.Scope = map[string]any{"cameras": 2, "confidential": false}
		d.Summary = map[string]any{"cameras": 2, "enabled": 2}
		for _, k := range []string{"id", "name", "location", "group", "enabled", "connection", "make", "ip", "main_stream_url", "sub_stream_url", "transport", "username", "onvif_url", "recording", "retention_days", "note", "state"} {
			d.Columns = append(d.Columns, Column{k, k, "left"})
		}
		d.Rows = []map[string]any{
			{"id": "0199a000-0000-7000-8000-000000000001", "name": "Gate", "location": "Main entrance", "group": "Yard", "enabled": "yes", "connection": "pull", "make": "hikvision", "ip": "192.168.1.64",
				"main_stream_url": "rtsp://192.168.1.64:554/Streaming/Channels/101", "sub_stream_url": "rtsp://192.168.1.64:554/Streaming/Channels/102", "transport": "auto", "username": "admin",
				"onvif_url": "http://192.168.1.64/onvif/device_service", "recording": "continuous", "retention_days": "14", "note": "Mast by the gate", "state": "online"},
			{"id": "0199a000-0000-7000-8000-000000000002", "name": "Warehouse door", "location": "Hall B", "enabled": "yes", "connection": "pull", "make": "tapo", "ip": "192.168.1.70",
				"main_stream_url": "rtsp://192.168.1.70:554/stream1", "transport": "auto", "username": "camera-account", "recording": "events", "retention_days": "7", "state": "online"},
		}
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
