package httpapi

import (
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/camerainv"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/video"
)

func invServer(t *testing.T) *Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Server{video: video.New(nil, t.TempDir(), 1, "unit-test-secret-0123456789abcdef", log), log: log}
}

// stored cameras of the tests: a Hikvision camera with masks and a password, a remote (push) camera, and a removed one
func invStored(t *testing.T, s *Server) []invCamera {
	pw, _ := s.video.SealPassword("0123-secret")
	sec, _ := s.video.SealPassword("relay-secret-0123456789abcdef")
	tapo, _ := s.video.SealPassword("tapo-pass")
	return []invCamera{
		{cameraRow: cameraRow{id: "0199a000-0000-7000-8000-000000000001", name: "Gate", location: "Entrance", url: "rtsp://192.168.1.64:554/Streaming/Channels/101",
			subURL: "rtsp://192.168.1.64:554/Streaming/Channels/102", username: "admin", sealed: pw, transport: "tcp", record: "events", retention: 30, sort: 1, enabled: true,
			source: "pull", onvifURL: "http://192.168.1.64/onvif/device_service", eventPre: 5, eventPost: 30, eventKinds: []string{"motion", "line"}, group: "Yard",
			masks: []byte(`[[[0.1,0.1],[0.3,0.1],[0.3,0.2],[0.1,0.2]]]`), motion: true, motionSens: 70, zones: []byte(`[]`),
			snapshotURL: "http://192.168.1.64/ISAPI/Streaming/channels/101/picture", audio: true, audioRecord: false, note: "Mast by the gate"}},
		{cameraRow: cameraRow{id: "0199a000-0000-7000-8000-000000000002", name: "Remote shed", username: "tapo", sealed: tapo, transport: "auto", record: "continuous", retention: 7,
			enabled: true, source: "push", pushSealed: sec, onvifURL: "http://192.168.0.21:2020/onvif/device_service", eventPre: 10, eventPost: 20,
			eventKinds: []string{}, masks: []byte(`[]`), motionSens: 50, zones: []byte(`[]`), audioRecord: true}},
		{cameraRow: cameraRow{id: "0199a000-0000-7000-8000-000000000003", name: "Old dock", url: "rtsp://192.168.1.80/live", transport: "auto", record: "off", retention: 7,
			source: "pull", eventPre: 10, eventPost: 20, eventKinds: []string{"motion"}, motionSens: 50, audioRecord: true}, removed: true},
	}
}

func invMaps(cams []invCamera) (map[string]*invCamera, map[string][]*invCamera) {
	byID, byName := map[string]*invCamera{}, map[string][]*invCamera{}
	for i := range cams {
		c := &cams[i]
		byID[c.id] = c
		if !c.removed {
			k := strings.ToLower(c.name)
			byName[k] = append(byName[k], c)
		}
	}
	return byID, byName
}

func parseInv(t *testing.T, text string) []camerainv.Row {
	t.Helper()
	f, err := camerainv.Parse([]byte(text), 100)
	if err != nil {
		t.Fatal(err)
	}
	return f.Rows
}

var allOpt = InventoryOptions{Update: true, Create: true}

// An export imports back onto the same cameras without any change — with and without passwords, Excel and CSV.
func TestInventoryRoundTrip(t *testing.T) {
	s := invServer(t)
	stored := invStored(t, s)
	byID, byName := invMaps(stored)
	p := &iam.Principal{Username: "admin"}
	for _, creds := range []bool{false, true} {
		var cams []camerainv.Camera
		for _, c := range stored[:2] {
			cams = append(cams, s.inventoryCamera(c, []string{"Plan (90°)"}, creds))
		}
		if creds && (cams[0].Password != "0123-secret" || cams[1].RelaySecret != "relay-secret-0123456789abcdef") {
			t.Fatalf("credentials not opened: %+v", cams)
		}
		if !creds && (cams[0].Password != "" || cams[1].RelaySecret != "") {
			t.Fatal("credentials without asking")
		}
		if cams[0].Make != "hikvision" || cams[0].IP != "192.168.1.64" || cams[1].Make != "tapo" || cams[1].IP != "192.168.0.21" {
			t.Errorf("make and ip: %+v", cams)
		}
		m := camerainv.Meta{Org: "Demo", Confidential: creds}
		for _, format := range []string{"xlsx", "csv"} {
			var b []byte
			if format == "xlsx" {
				b, _ = camerainv.XLSX(m, cams)
			} else {
				b, _ = camerainv.CSV(m, cams)
			}
			f, err := camerainv.Parse(b, 100)
			if err != nil {
				t.Fatal(err)
			}
			for i := range f.Rows {
				it := s.planRow(p, &f.Rows[i], byID, byName, nil, allOpt)
				if it.Action != "unchanged" || len(it.Errors) > 0 || len(it.Warnings) > 0 {
					t.Errorf("%s creds=%v line %d: %s %v %v %v", format, creds, it.Line, it.Action, it.Changes, it.Errors, it.Warnings)
				}
				// what would be stored equals what is stored
				if want := stored[i].cameraRow; cameraChanges(want, it.row, false, false) != nil || it.row.sealed != want.sealed || it.row.pushSealed != want.pushSealed {
					t.Errorf("%s creds=%v line %d: stored row differs", format, creds, it.Line)
				}
			}
		}
	}
}

func TestInventoryPlanRows(t *testing.T) {
	s := invServer(t)
	stored := invStored(t, s)
	byID, byName := invMaps(stored)
	p := &iam.Principal{Username: "admin"}
	plan := func(text string, opt InventoryOptions, taken map[string]bool) []*invItem {
		var out []*invItem
		for _, r := range parseInv(t, text) {
			out = append(out, s.planRow(p, &r, byID, byName, taken, opt))
		}
		return out
	}

	// a new camera from make and ip only
	it := plan("name;make;ip;username;password\nWarehouse;reolink;10.0.0.7;cam;secret-1\n", allOpt, nil)[0]
	if it.Action != "create" || it.row.url != "rtsp://10.0.0.7:554/h264Preview_01_main" || it.row.subURL != "rtsp://10.0.0.7:554/h264Preview_01_sub" ||
		it.row.onvifURL != "http://10.0.0.7:8000/onvif/device_service" || s.video.OpenPassword(it.row.sealed) != "secret-1" || !it.pwChanged || it.row.retention != 7 || !it.Enabled {
		t.Errorf("create from make: %+v", it)
	}
	// a different ip moves every address of the camera; an empty password keeps the stored one
	it = plan("id;ip;password\n0199a000-0000-7000-8000-000000000001;10.20.0.64;\n", allOpt, nil)[0]
	if it.Action != "update" || !slices.Equal(it.Changes, []string{"onvif_url", "snapshot_url", "sub_url", "url"}) || it.row.url != "rtsp://10.20.0.64:554/Streaming/Channels/101" ||
		it.row.snapshotURL != "http://10.20.0.64/ISAPI/Streaming/channels/101/picture" || it.row.sealed != stored[0].sealed || it.pwChanged {
		t.Errorf("ip move: %s %v %+v", it.Action, it.Changes, it.row)
	}
	// the same password is no change, a new one is
	if it = plan("name;password\nGate;0123-secret\n", allOpt, nil)[0]; it.Action != "unchanged" || it.ID != stored[0].id {
		t.Errorf("same password: %s %v", it.Action, it.Changes)
	}
	if it = plan("name;password\nGate;new-one\n", allOpt, nil)[0]; it.Action != "update" || !slices.Equal(it.Changes, []string{"password"}) || s.video.OpenPassword(it.row.sealed) != "new-one" {
		t.Errorf("new password: %s %v", it.Action, it.Changes)
	}
	// a dash clears a text, an invalid value is an error with the form's message
	if it = plan("name;location;retention_days\nGate;-;0\n", allOpt, nil)[0]; it.Action != "error" || it.Errors[0] != "retention_days: 1 to 3650" {
		t.Errorf("invalid value: %+v", it)
	}
	if it = plan("name;location\nGate;-\n", allOpt, nil)[0]; it.Action != "update" || it.row.location != "" || !slices.Equal(it.Changes, []string{"location"}) {
		t.Errorf("clear: %+v", it)
	}
	// ids: a free id is kept, an id of another organization is not, a removed camera is restored
	items := plan("id;name;main_stream_url\n0199a000-0000-7000-8000-0000000000aa;New A;rtsp://10.0.0.1/a\n0199a000-0000-7000-8000-0000000000bb;New B;rtsp://10.0.0.2/b\n0199a000-0000-7000-8000-000000000003;Old dock;\nnot-an-id;X;rtsp://h/x\n",
		allOpt, map[string]bool{"0199a000-0000-7000-8000-0000000000bb": true})
	if items[0].Action != "create" || !items[0].keepID || items[0].ID != "0199a000-0000-7000-8000-0000000000aa" {
		t.Errorf("free id: %+v", items[0])
	}
	if items[1].Action != "create" || items[1].keepID || len(items[1].Warnings) != 1 {
		t.Errorf("foreign id: %+v", items[1])
	}
	if items[2].Action != "restore" || items[2].row.url != "rtsp://192.168.1.80/live" {
		t.Errorf("restore: %+v", items[2])
	}
	if items[3].Action != "error" {
		t.Errorf("bad id: %+v", items[3])
	}
	// options: nothing is added or changed when switched off
	items = plan("name;main_stream_url\nNew C;rtsp://10.0.0.3/c\nGate;rtsp://192.168.1.64:554/x\n", InventoryOptions{}, nil)
	if items[0].Action != "skip" || items[1].Action != "skip" {
		t.Errorf("options off: %s %s", items[0].Action, items[1].Action)
	}
	// a push camera from the file gets a new relay secret unless the file has one
	it = plan("name;connection;onvif_url\nRemote 2;push;http://192.168.0.30:2020/onvif/device_service\n", allOpt, nil)[0]
	if it.Action != "create" || !it.newSecret || it.row.pushSealed == "" || it.row.url != "" {
		t.Errorf("push: %+v", it)
	}
	it = plan("name;relay_secret\nRemote shed;short\n", allOpt, nil)[0]
	if it.Action != "error" {
		t.Errorf("short relay secret: %+v", it)
	}
	// a make without an address, an unknown make
	if it = plan("name;make\nX;tapo\n", allOpt, nil)[0]; it.Action != "error" {
		t.Errorf("make without ip: %+v", it)
	}
	if it = plan("name;make;ip\nX;acme;10.0.0.1\n", allOpt, nil)[0]; it.Action != "error" {
		t.Errorf("unknown make: %+v", it)
	}
	// camera groups: a limited user changes only cameras of their groups and adds only into them
	lim := &iam.Principal{Username: "guard", CameraLimited: true, CameraGroups: []string{"Lobby"}}
	r := parseInv(t, "name;group;main_stream_url\nGate;;\nNew;Yard;rtsp://10.0.0.9/a\nNew 2;Lobby;rtsp://10.0.0.9/b\n")
	if a, b, c := s.planRow(lim, &r[0], byID, byName, nil, allOpt), s.planRow(lim, &r[1], byID, byName, nil, allOpt), s.planRow(lim, &r[2], byID, byName, nil, allOpt); a.Action != "error" || b.Action != "error" || c.Action != "create" {
		t.Errorf("camera groups: %s %s %s", a.Action, b.Action, c.Action)
	}
}

// Without a licence: enabled cameras stay enabled, new ones take the free places in file order, the rest come disabled.
func TestInventoryCameraLimit(t *testing.T) {
	s := invServer(t)
	stored := invStored(t, s)
	byID, byName := invMaps(stored)
	p := &iam.Principal{Username: "admin"}
	rows := parseInv(t, "name;main_stream_url;enabled\nGate;;yes\nA;rtsp://10.0.0.1/a;yes\nB;rtsp://10.0.0.2/b;yes\nC;rtsp://10.0.0.3/c;no\nD;rtsp://10.0.0.4/d;\n")
	mk := func() *InventoryPlan {
		pl := &InventoryPlan{Counts: map[string]int{}}
		for i := range rows {
			pl.Rows = append(pl.Rows, s.planRow(p, &rows[i], byID, byName, nil, allOpt))
		}
		return pl
	}
	pl := mk()
	applyCameraLimit(pl, 2) // two other enabled cameras of the server; Gate is enabled and stays so
	got := []bool{}
	for _, it := range pl.Rows {
		got = append(got, it.Enabled)
	}
	if !slices.Equal(got, []bool{true, true, false, false, false}) || pl.EnabledAfter != 4 || pl.Counts["disabled_by_licence"] != 2 {
		t.Errorf("limit: %v after %d, disabled %d", got, pl.EnabledAfter, pl.Counts["disabled_by_licence"])
	}
	if !strings.Contains(strings.Join(pl.Rows[2].Warnings, " "), "imported disabled") || pl.Rows[3].Warnings != nil {
		t.Errorf("warnings: %v / %v", pl.Rows[2].Warnings, pl.Rows[3].Warnings)
	}
	pl = mk()
	pl.Licensed = true
	applyCameraLimit(pl, 2)
	if pl.EnabledAfter != 6 || pl.Counts["disabled_by_licence"] != 0 {
		t.Errorf("licensed: after %d", pl.EnabledAfter)
	}
	// a disabled camera switched on by the file takes a place like a new one
	rows = parseInv(t, "name;enabled\nGate;yes\n")
	stored[0].enabled = false
	byID, byName = invMaps(stored)
	pl = &InventoryPlan{Counts: map[string]int{}, Rows: []*invItem{s.planRow(p, &rows[0], byID, byName, nil, allOpt)}}
	applyCameraLimit(pl, 4)
	if pl.Rows[0].Enabled || pl.Rows[0].Action != "unchanged" || pl.Counts["disabled_by_licence"] != 1 {
		t.Errorf("enabling over the limit: %+v", pl.Rows[0])
	}
}
