package metrics

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	r := &Registry{}
	c := r.Counter("t_msgs_total", "messages", "status")
	c.Inc("ok")
	c.Inc("ok")
	c.Add("error", 3)
	u := r.Counter("t_plain_total", "plain", "")
	u.Inc("")
	s := r.Summary("t_lat_seconds", "latency")
	s.Observe(0.5)
	s.Observe(1.5)
	g := r.Gauge("t_gauge", `a "quoted" help\n`)
	g.Set(2.25)
	var buf bytes.Buffer
	Format(&buf, r.Snapshot())
	out := buf.String()
	for _, want := range []string{
		"# HELP t_msgs_total messages\n# TYPE t_msgs_total counter\n",
		`t_msgs_total{status="error"} 3`, `t_msgs_total{status="ok"} 2`, "t_plain_total 1\n",
		"t_lat_seconds_count 2", "t_lat_seconds_sum 2", "t_lat_seconds_max 1.5", "t_gauge 2.25",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Count(out, "# TYPE t_msgs_total") != 1 {
		t.Errorf("TYPE line must appear once")
	}
}
