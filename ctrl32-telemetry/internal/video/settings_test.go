package video

import "testing"

func TestSettingsBounds(t *testing.T) {
	d := DefaultSettings(50)
	if msg := d.Validate(); msg != "" {
		t.Fatalf("defaults invalid: %s", msg)
	}
	for _, c := range []struct {
		name string
		mod  func(*Settings)
	}{
		{"disk", func(s *Settings) { s.MaxDiskGB = 0 }},
		{"segment", func(s *Settings) { s.SegmentSeconds = 5 }},
		{"relay fails", func(s *Settings) { s.PushMaxFails = 2 }},
		{"relay block", func(s *Settings) { s.PushBlockMinutes = 0 }},
		{"discovery", func(s *Settings) { s.DiscoverySeconds = 60 }},
		{"ptz audit", func(s *Settings) { s.PTZAuditMinutes = -1 }},
	} {
		s := d
		c.mod(&s)
		if s.Validate() == "" {
			t.Errorf("%s: out of bounds accepted", c.name)
		}
	}
}
