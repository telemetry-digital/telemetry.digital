package camerainv

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/xlsx"
)

func sample() []Camera {
	return []Camera{
		{ID: "0199a000-0000-7000-8000-000000000001", Name: "Gate; north", Location: "Main \"entrance\"", Group: "Yard", Enabled: true, Connection: "pull", Make: "hikvision", IP: "192.168.1.64",
			URL: "rtsp://192.168.1.64:554/Streaming/Channels/101", SubURL: "rtsp://192.168.1.64:554/Streaming/Channels/102", Transport: "tcp", Username: "admin", Password: "0123;pass\"word",
			OnvifURL: "http://192.168.1.64/onvif/device_service", PTZProfile: "Profile_1", SnapshotURL: "http://192.168.1.64/ISAPI/Streaming/channels/101/picture", Recording: "events",
			PreEvent: 5, PostEvent: 30, EventKinds: []string{"motion", "line"}, RetentionDays: 30, MotionDetect: true, MotionSensitivity: 70,
			MotionZones: `[[[0,0],[1,0],[1,0.5]]]`, PrivacyMasks: `[[[0.1,0.1],[0.2,0.1],[0.2,0.2],[0.1,0.2]]]`, Audio: true, AudioRecord: false, Sort: 3,
			Note: "Mast by the gate, PoE port 3\nsecond line", FloorPlans: "Plan (90°)", State: "online"},
		{ID: "0199a000-0000-7000-8000-000000000002", Name: "Remote shed", Enabled: false, Connection: "push", Make: "tapo", IP: "192.168.0.21", Transport: "auto",
			Username: "tapo-user", RelaySecret: "abcdefghijklmnopqrstuvwxyz012345", OnvifURL: "http://192.168.0.21:2020/onvif/device_service", Recording: "continuous",
			PreEvent: 10, PostEvent: 20, EventKinds: []string{}, RetentionDays: 7, MotionSensitivity: 50, AudioRecord: true},
	}
}

// back turns a parsed row into a Camera again (what the import would store), for comparison.
func back(t *testing.T, r Row) Camera {
	t.Helper()
	if len(r.Errors) > 0 {
		t.Fatalf("line %d: %v", r.Line, r.Errors)
	}
	s := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	i := func(p *int) int {
		if p == nil {
			return -1
		}
		return *p
	}
	b := func(p *bool) bool { return p != nil && *p }
	j := func(p *json.RawMessage) string {
		if p == nil {
			return ""
		}
		return string(*p)
	}
	c := Camera{ID: r.ID, Name: s(r.Name), Location: s(r.Location), Group: s(r.Group), Enabled: b(r.Enabled), Connection: s(r.Connection), Make: s(r.Make), IP: s(r.IP),
		URL: s(r.URL), SubURL: s(r.SubURL), Transport: s(r.Transport), Username: s(r.Username), Password: s(r.Password), RelaySecret: s(r.RelaySecret),
		OnvifURL: s(r.OnvifURL), PTZProfile: s(r.PTZProfile), SnapshotURL: s(r.SnapshotURL), Recording: s(r.Recording), PreEvent: i(r.PreEvent), PostEvent: i(r.PostEvent),
		RetentionDays: i(r.RetentionDays), MotionDetect: b(r.MotionDetect), MotionSensitivity: i(r.MotionSensitivity), MotionZones: j(r.MotionZones), PrivacyMasks: j(r.PrivacyMasks),
		Audio: b(r.Audio), AudioRecord: b(r.AudioRecord), Sort: i(r.Sort), Note: s(r.Note)}
	if r.EventKinds != nil {
		c.EventKinds = *r.EventKinds
	}
	return c
}

func TestRoundTrip(t *testing.T) {
	for _, format := range []string{"xlsx", "csv"} {
		for _, creds := range []bool{false, true} {
			m := Meta{Org: "Demo; s.r.o.", Server: "https://cams.example.com", User: "admin", Version: "0.70.0", ExportedAt: "2026-10-02 21:00 CEST", Confidential: creds}
			var b []byte
			var err error
			if format == "xlsx" {
				b, err = XLSX(m, sample())
			} else {
				b, err = CSV(m, sample())
			}
			if err != nil {
				t.Fatal(err)
			}
			f, err := Parse(b, 100)
			if err != nil {
				t.Fatalf("%s: %v", format, err)
			}
			if f.Format != format || len(f.Rows) != 2 {
				t.Fatalf("%s: format %s, %d rows", format, f.Format, len(f.Rows))
			}
			if f.Has("password") != creds || f.Has("relay_secret") != creds {
				t.Errorf("%s creds=%v: columns %v", format, creds, f.Columns)
			}
			if len(f.Ignored) != 2 { // floor_plans and state are export only
				t.Errorf("%s: ignored %v", format, f.Ignored)
			}
			for k, want := range sample() {
				got := back(t, f.Rows[k])
				want.FloorPlans, want.State = "", ""
				want.MotionZones, want.PrivacyMasks = orEmptyJSON(want.MotionZones), orEmptyJSON(want.PrivacyMasks)
				if !creds {
					want.Password, want.RelaySecret = "", ""
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("%s creds=%v row %d:\n got %+v\nwant %+v", format, creds, k, got, want)
				}
			}
		}
	}
}

func TestExportMarkings(t *testing.T) {
	m := Meta{Org: "Demo", Confidential: true}
	b, _ := XLSX(m, sample())
	sheets, err := xlsx.Read(b)
	if err != nil {
		t.Fatal(err)
	}
	if sheets[0].Name != "CONFIDENTIAL cameras" || sheets[1].Name != "Read me" || sheets[0].Rows[0][0] != Confidential {
		t.Errorf("confidential workbook: %q %q %q", sheets[0].Name, sheets[1].Name, sheets[0].Rows[0])
	}
	m.Confidential = false
	b, _ = XLSX(m, sample())
	sheets, _ = xlsx.Read(b)
	if sheets[0].Name != "Cameras" || strings.Contains(string(b), "0123;pass") {
		t.Error("a workbook without passwords must not contain them")
	}
	c, _ := CSV(m, sample())
	if !bytes.HasPrefix(c, []byte("\xEF\xBB\xBF# Camera inventory: Demo")) || bytes.Contains(c, []byte("0123;pass")) || !bytes.Contains(c, []byte("\r\nid;name;location;group;enabled")) {
		t.Errorf("csv: %q", c[:120])
	}
	c, _ = CSV(Meta{Org: "Demo", Confidential: true}, sample())
	if !bytes.Contains(c, []byte("# "+Confidential)) || !bytes.Contains(c, []byte(`"0123;pass""word"`)) {
		t.Error("confidential csv")
	}
}

func TestTemplate(t *testing.T) {
	for _, format := range []string{"xlsx", "csv"} {
		b, err := Template(format)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Parse(b, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(f.Rows) != 0 || !f.Has("password") || !f.Has("make") {
			t.Errorf("%s template: %d rows, columns %v", format, len(f.Rows), f.Columns)
		}
		// the example rows import once their id is removed
		s := strings.Replace(string(b), "#example", "", 1)
		if format == "csv" {
			f, err = Parse([]byte(s), 10)
			if err != nil || len(f.Rows) != 1 || *f.Rows[0].Name != "Gate" || *f.Rows[0].Make != "hikvision" {
				t.Errorf("csv example row: %v %+v", err, f)
			}
		}
	}
}

func TestParseForms(t *testing.T) {
	// comma separated, Windows-1250 (Excel "CSV" in Slovak), Excel-style numbers and yes/no in other words
	text := "Name,IP,Make,Enabled,Retention days,Pre-event-s,Location,Sub stream url,Event kinds,Unknown\r\n" +
		"Brána,192.168.1.10,dahua,áno,14.0,0,Vrátnica,-,\"motion, line\",x\r\n" +
		",,,,,,,,,\r\n" +
		"Sklad,192.168.1.11,,nie,7,5,,,none,\r\n"
	enc, err := charmap.Windows1250.NewEncoder().String(text)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse([]byte(enc), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Rows) != 2 || len(f.Ignored) != 1 || f.Ignored[0] != "Unknown" {
		t.Fatalf("rows %d, ignored %v", len(f.Rows), f.Ignored)
	}
	r := f.Rows[0]
	if *r.Name != "Brána" || *r.IP != "192.168.1.10" || !*r.Enabled || *r.RetentionDays != 14 || *r.PreEvent != 0 || *r.Location != "Vrátnica" || *r.SubURL != "" ||
		strings.Join(*r.EventKinds, "|") != "motion|line" || r.URL != nil || r.Line != 2 {
		t.Errorf("row 1: %+v", r)
	}
	if r2 := f.Rows[1]; *r2.Enabled || len(*r2.EventKinds) != 0 || r2.Make != nil || r2.Location != nil || r2.Line != 4 {
		t.Errorf("row 2: %+v", r2)
	}
	// errors stay with their row
	f, err = Parse([]byte("id;name;enabled;sort;privacy_masks\n;A;maybe;1.5;[[\n"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if e := f.Rows[0].Errors; len(e) != 3 {
		t.Errorf("errors: %v", e)
	}
	// limits and missing header
	if _, err := Parse([]byte("a;b\n1;2\n"), 10); err == nil {
		t.Error("a file without the name column")
	}
	if _, err := Parse([]byte("name;ip\na;1\nb;2\nc;3\n"), 2); err == nil {
		t.Error("row limit")
	}
	if _, err := Parse(nil, 2); err == nil {
		t.Error("empty file")
	}
}

func TestAddresses(t *testing.T) {
	if DetectMake("rtsp://h:554/Streaming/Channels/101", "", "") != "hikvision" || DetectMake("rtsp://h/stream1", "", "") != "tapo" ||
		DetectMake("", "", "http://10.0.0.5:2020/onvif/device_service") != "tapo" || DetectMake("rtsp://h/live", "", "") != "other" ||
		DetectMake("rtsp://h:554/cam/realmonitor?channel=1&subtype=0", "", "") != "dahua" {
		t.Error("make detection")
	}
	p, ok := PresetOf("Reolink")
	if !ok {
		t.Fatal("preset")
	}
	main, sub, onvif, snap := p.Addresses("10.1.2.3")
	if main != "rtsp://10.1.2.3:554/h264Preview_01_main" || sub != "rtsp://10.1.2.3:554/h264Preview_01_sub" || onvif != "http://10.1.2.3:8000/onvif/device_service" || snap != "" {
		t.Errorf("preset: %s %s %s %s", main, sub, onvif, snap)
	}
	for _, c := range []struct{ in, old, new, want string }{
		{"rtsp://192.168.1.64:554/a?b=1", "192.168.1.64", "10.0.0.9", "rtsp://10.0.0.9:554/a?b=1"},
		{"http://192.168.1.64/onvif/device_service", "192.168.1.64", "cam.local", "http://cam.local/onvif/device_service"},
		{"rtsp://other:554/a", "192.168.1.64", "10.0.0.9", "rtsp://other:554/a"},
		{"rtsp://192.168.1.64:554/a", "192.168.1.64", "fd00::5", "rtsp://[fd00::5]:554/a"},
	} {
		if got, _ := MoveHost(c.in, c.old, c.new); got != c.want {
			t.Errorf("MoveHost(%s) = %s, want %s", c.in, got, c.want)
		}
	}
	if !ValidHost("192.168.1.5") || !ValidHost("cam-1.site.local") || !ValidHost("fd00::1") || ValidHost("bad host") || ValidHost("") {
		t.Error("ValidHost")
	}
	for in, want := range map[string]bool{"YES": true, "áno": true, "x": true, "0": false, "Nie": false} {
		if got, ok := ParseBool(in); !ok || got != want {
			t.Errorf("ParseBool(%q)", in)
		}
	}
	if _, ok := ParseBool("maybe"); ok {
		t.Error("ParseBool(maybe)")
	}
}
