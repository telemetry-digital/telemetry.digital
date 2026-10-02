// Package camerainv is the camera inventory (spec 028): one file with every setting of the cameras of an
// organization, for the customer to keep after the installation company leaves, and to put the cameras back in a few
// minutes after a crash or a move to another server. The same columns are written (Excel, CSV, PDF) and read back
// (Excel or CSV): an exported file imports unchanged.
//
// The package only converts between files and rows; validation against the database, matching, the licence limit
// and storing are in internal/httpapi (camera_inventory.go), shared by the web interface and the command line.
package camerainv

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Column is one column of the inventory, in file order.
type Column struct {
	Key        string  // header in the file
	Help       string  // explanation for the "Read me" sheet and the documentation
	Values     string  // allowed values
	Example    string  // a value of the template's example row
	Credential bool    // only in a confidential export (passwords and relay secrets)
	ExportOnly bool    // written for information, ignored on import
	Width      float64 // column width in Excel (characters)
}

// Columns of the inventory. The keys are part of the file format: never rename one (add new columns at the end).
var Columns = []Column{
	{Key: "id", Help: "The camera's id. Empty for a new camera; an id from an export of another server is kept when it is free. Rows whose id starts with # are ignored (the template's examples).", Values: "id or empty", Example: "#example", Width: 38},
	{Key: "name", Help: "Name shown everywhere. Without an id, a camera of the same name (and the same ip, when several have the name) is updated.", Values: "1–80 characters", Example: "Gate", Width: 22},
	{Key: "location", Help: "Where the camera is.", Values: "up to 120 characters", Example: "Main entrance", Width: 20},
	{Key: "group", Help: "Camera group: users limited to some groups see only those cameras. A new group is created by using it.", Values: "up to 60 characters", Example: "Yard", Width: 14},
	{Key: "enabled", Help: "Whether the server runs the camera. Without a licence at most 4 cameras of the server are enabled; rows beyond are imported disabled.", Values: "yes, no", Example: "yes", Width: 9},
	{Key: "connection", Help: "pull: the server connects to the camera (local network or VPN); push: a relay at a remote site sends the video.", Values: "pull, push", Example: "pull", Width: 11},
	{Key: "make", Help: "Camera make. With ip it fills in the addresses of a camera whose main_stream_url is empty. In an export: recognised from the addresses.", Values: "tapo, hikvision, dahua, reolink, axis, uniview, ezviz, other", Example: "hikvision", Width: 11},
	{Key: "ip", Help: "IP address or host name of the camera (of the main stream; of the ONVIF address for push). A different value moves every address of the camera from the old one to the new one.", Values: "address", Example: "192.168.1.64", Width: 16},
	{Key: "main_stream_url", Help: "Main stream, without user name and password (those are separate columns).", Values: "rtsp://, rtsps://, http://, https://", Example: "rtsp://192.168.1.64:554/Streaming/Channels/101", Width: 46},
	{Key: "sub_stream_url", Help: "Smaller stream for tiles (optional).", Values: "as main_stream_url", Example: "rtsp://192.168.1.64:554/Streaming/Channels/102", Width: 46},
	{Key: "transport", Help: "RTSP transport.", Values: "auto, tcp, udp", Example: "auto", Width: 10},
	{Key: "username", Help: "The camera's user name (streams and ONVIF).", Values: "text", Example: "admin", Width: 12},
	{Key: "password", Help: "The camera's password — only in a confidential export. On import an empty cell keeps the stored password.", Values: "text", Example: "", Credential: true, Width: 16},
	{Key: "relay_secret", Help: "Secret of a push camera's relay — only in a confidential export. On import the relay keeps working with it; without it a new secret is made.", Values: "16–200 characters", Example: "", Credential: true, Width: 24},
	{Key: "onvif_url", Help: "ONVIF device service for events and PTZ (optional).", Values: "http(s)://camera/onvif/device_service", Example: "http://192.168.1.64/onvif/device_service", Width: 40},
	{Key: "ptz_profile", Help: "ONVIF profile token for PTZ; empty = the first profile with PTZ.", Values: "up to 64 characters", Example: "", Width: 12},
	{Key: "snapshot_url", Help: "Address of one JPEG picture, for motion detection of H.264/H.265 cameras.", Values: "http(s)://…", Example: "http://192.168.1.64/ISAPI/Streaming/channels/101/picture", Width: 40},
	{Key: "recording", Help: "Recording mode.", Values: "continuous, events, off", Example: "continuous", Width: 12},
	{Key: "pre_event_s", Help: "Seconds recorded before an event (recording = events).", Values: "0–60", Example: "10", Width: 11},
	{Key: "post_event_s", Help: "Seconds recorded after an event.", Values: "1–600", Example: "20", Width: 12},
	{Key: "event_kinds", Help: "Events that start recording, separated by commas; none for no event.", Values: "motion, tamper, line, intrusion, input, mark, other", Example: "motion,tamper,line,intrusion,input,mark", Width: 30},
	{Key: "retention_days", Help: "Days the recordings are kept.", Values: "1–3650", Example: "14", Width: 13},
	{Key: "motion_detection", Help: "Motion detection on the server (cameras without ONVIF events).", Values: "yes, no", Example: "no", Width: 15},
	{Key: "motion_sensitivity", Help: "Sensitivity of the motion detection.", Values: "1–100", Example: "50", Width: 16},
	{Key: "motion_zones", Help: "Areas where motion counts: polygons of [x, y] points from 0 to 1 (JSON). [] = the whole picture.", Values: "JSON", Example: "[]", Width: 24},
	{Key: "privacy_masks", Help: "Covered areas of the picture: polygons of [x, y] points from 0 to 1 (JSON). [] = none.", Values: "JSON", Example: "[[[0,0],[0.25,0],[0.25,0.2],[0,0.2]]]", Width: 30},
	{Key: "audio", Help: "Take the camera's sound.", Values: "yes, no", Example: "no", Width: 8},
	{Key: "audio_record", Help: "Record the sound too (no = heard live only).", Values: "yes, no", Example: "yes", Width: 12},
	{Key: "sort", Help: "Order in lists.", Values: "whole number", Example: "0", Width: 7},
	{Key: "note", Help: "Free note: mounting, cabling, NVR channel, warranty.", Values: "up to 500 characters", Example: "Mast by the gate, PoE switch port 3", Width: 30},
	{Key: "floor_plans", Help: "Floor plans and dashboards with a pin of the camera. Export only: pins are part of the dashboards.", Values: "text", ExportOnly: true, Width: 24},
	{Key: "state", Help: "State of the camera when the file was made. Export only.", Values: "text", ExportOnly: true, Width: 12},
}

// Camera is one camera as the inventory shows it.
type Camera struct {
	ID, Name, Location, Group         string
	Enabled                           bool
	Connection, Make, IP              string
	URL, SubURL, Transport, Username  string
	Password, RelaySecret             string // only in a confidential export
	OnvifURL, PTZProfile, SnapshotURL string
	Recording                         string
	PreEvent, PostEvent               int
	EventKinds                        []string
	RetentionDays                     int
	MotionDetect                      bool
	MotionSensitivity                 int
	MotionZones, PrivacyMasks         string // JSON
	Audio, AudioRecord                bool
	Sort                              int
	Note                              string
	FloorPlans, State                 string // export only
}

// Values returns the cells of the camera in column order; credentials only when withCredentials.
func (c Camera) Values(withCredentials bool) []string {
	out := make([]string, 0, len(Columns))
	for _, col := range Columns {
		if col.Credential && !withCredentials {
			continue
		}
		out = append(out, c.value(col.Key))
	}
	return out
}

// Headers returns the column keys in file order; credentials only when withCredentials.
func Headers(withCredentials bool) []string {
	var out []string
	for _, col := range Columns {
		if col.Credential && !withCredentials {
			continue
		}
		out = append(out, col.Key)
	}
	return out
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func (c Camera) value(key string) string {
	switch key {
	case "id":
		return c.ID
	case "name":
		return c.Name
	case "location":
		return c.Location
	case "group":
		return c.Group
	case "enabled":
		return yesNo(c.Enabled)
	case "connection":
		return c.Connection
	case "make":
		return c.Make
	case "ip":
		return c.IP
	case "main_stream_url":
		return c.URL
	case "sub_stream_url":
		return c.SubURL
	case "transport":
		return c.Transport
	case "username":
		return c.Username
	case "password":
		return c.Password
	case "relay_secret":
		return c.RelaySecret
	case "onvif_url":
		return c.OnvifURL
	case "ptz_profile":
		return c.PTZProfile
	case "snapshot_url":
		return c.SnapshotURL
	case "recording":
		return c.Recording
	case "pre_event_s":
		return strconv.Itoa(c.PreEvent)
	case "post_event_s":
		return strconv.Itoa(c.PostEvent)
	case "event_kinds":
		if len(c.EventKinds) == 0 {
			return "none" // an empty cell would keep the stored kinds
		}
		return strings.Join(c.EventKinds, ",")
	case "retention_days":
		return strconv.Itoa(c.RetentionDays)
	case "motion_detection":
		return yesNo(c.MotionDetect)
	case "motion_sensitivity":
		return strconv.Itoa(c.MotionSensitivity)
	case "motion_zones":
		return orEmptyJSON(c.MotionZones)
	case "privacy_masks":
		return orEmptyJSON(c.PrivacyMasks)
	case "audio":
		return yesNo(c.Audio)
	case "audio_record":
		return yesNo(c.AudioRecord)
	case "sort":
		return strconv.Itoa(c.Sort)
	case "note":
		return c.Note
	case "floor_plans":
		return c.FloorPlans
	case "state":
		return c.State
	}
	return ""
}

func orEmptyJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return "[]"
	}
	return s
}

// numeric reports whether a column holds whole numbers (written as numbers into Excel).
func numeric(key string) bool {
	switch key {
	case "pre_event_s", "post_event_s", "retention_days", "motion_sensitivity", "sort":
		return true
	}
	return false
}

// ---- camera makes --------------------------------------------------------------------------------------------------

// Preset is the usual addresses of a camera make ({ip} is replaced); the same table as the camera form.
type Preset struct {
	Key, Name, URL, Sub, Onvif, Snap string
}

// Presets are the camera makes the form and the import know.
var Presets = []Preset{
	{"tapo", "TP-Link Tapo", "rtsp://{ip}:554/stream1", "rtsp://{ip}:554/stream2", "http://{ip}:2020/onvif/device_service", ""},
	{"hikvision", "Hikvision (and HiLook)", "rtsp://{ip}:554/Streaming/Channels/101", "rtsp://{ip}:554/Streaming/Channels/102", "http://{ip}/onvif/device_service", "http://{ip}/ISAPI/Streaming/channels/101/picture"},
	{"dahua", "Dahua (and Imou, Amcrest)", "rtsp://{ip}:554/cam/realmonitor?channel=1&subtype=0", "rtsp://{ip}:554/cam/realmonitor?channel=1&subtype=1", "http://{ip}/onvif/device_service", "http://{ip}/cgi-bin/snapshot.cgi"},
	{"reolink", "Reolink", "rtsp://{ip}:554/h264Preview_01_main", "rtsp://{ip}:554/h264Preview_01_sub", "http://{ip}:8000/onvif/device_service", ""},
	{"axis", "Axis", "rtsp://{ip}/axis-media/media.amp", "rtsp://{ip}/axis-media/media.amp?resolution=640x360", "http://{ip}/onvif/device_service", "http://{ip}/axis-cgi/jpg/image.cgi"},
	{"uniview", "Uniview", "rtsp://{ip}:554/media/video1", "rtsp://{ip}:554/media/video2", "http://{ip}/onvif/device_service", ""},
	{"ezviz", "EZVIZ", "rtsp://{ip}:554/h264/ch1/main/av_stream", "rtsp://{ip}:554/h264/ch1/sub/av_stream", "", ""},
}

// PresetOf returns the preset of a make key ("" or "other": none).
func PresetOf(key string) (Preset, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, p := range Presets {
		if p.Key == key {
			return p, true
		}
	}
	return Preset{}, false
}

// Addresses fills a preset in for an IP address or host name.
func (p Preset) Addresses(ip string) (main, sub, onvif, snap string) {
	h := hostForURL(ip)
	at := func(s string) string { return strings.ReplaceAll(s, "{ip}", h) }
	return at(p.URL), at(p.Sub), at(p.Onvif), at(p.Snap)
}

// DetectMake recognises the make from the stream addresses ("other" when it cannot).
func DetectMake(mainURL, subURL, onvifURL string) string {
	all := strings.ToLower(mainURL + " " + subURL)
	switch {
	case strings.Contains(all, "/streaming/channels/"), strings.Contains(strings.ToLower(onvifURL), "/isapi/"):
		return "hikvision"
	case strings.Contains(all, "/cam/realmonitor"):
		return "dahua"
	case strings.Contains(all, "preview_01_"):
		return "reolink"
	case strings.Contains(all, "/axis-media/"):
		return "axis"
	case strings.Contains(all, "/media/video"):
		return "uniview"
	case strings.Contains(all, "/h264/ch1/"):
		return "ezviz"
	}
	if u, err := url.Parse(mainURL); err == nil && (u.Path == "/stream1" || u.Path == "/stream2") {
		return "tapo"
	}
	if u, err := url.Parse(onvifURL); err == nil && u.Port() == "2020" {
		return "tapo"
	}
	return "other"
}

// HostOf is the host name or IP address of an address ("" when there is none).
func HostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// MoveHost replaces the host of an address when it is oldHost, keeping the port, path and query.
func MoveHost(raw, oldHost, newHost string) (string, bool) {
	if raw == "" || oldHost == "" || newHost == "" {
		return raw, false
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Hostname(), oldHost) {
		return raw, false
	}
	h := hostForURL(newHost)
	if p := u.Port(); p != "" {
		h = net.JoinHostPort(strings.Trim(h, "[]"), p)
	}
	u.Host = h
	return u.String(), true
}

// hostForURL brackets an IPv6 address.
func hostForURL(h string) string {
	h = strings.TrimSpace(h)
	if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
		return "[" + h + "]"
	}
	return h
}

// ValidHost accepts an IP address or a host name.
func ValidHost(h string) bool {
	if net.ParseIP(strings.Trim(h, "[]")) != nil {
		return true
	}
	if h == "" || len(h) > 253 {
		return false
	}
	for _, part := range strings.Split(h, ".") {
		if part == "" || len(part) > 63 {
			return false
		}
		for _, r := range part {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// ---- reading cells -------------------------------------------------------------------------------------------------

// Row is one camera row of an imported file. A nil field is an empty cell: a new camera gets the default, an existing
// one keeps its value. "-" in a clearable text column clears it.
type Row struct {
	Line int // line (CSV) or row (Excel) number in the file, for messages
	ID   string

	Name, Location, Group              *string
	Enabled                            *bool
	Connection, Make, IP               *string
	URL, SubURL, Transport             *string
	Username, Password, RelaySecret    *string
	OnvifURL, PTZProfile, SnapshotURL  *string
	Recording                          *string
	PreEvent, PostEvent, RetentionDays *int
	MotionSensitivity, Sort            *int
	EventKinds                         *[]string
	MotionDetect, Audio, AudioRecord   *bool
	MotionZones, PrivacyMasks          *json.RawMessage
	Note                               *string
	Errors                             []string // cells that could not be read
	cellsSet                           int
}

// Empty reports whether no cell of the row has a value.
func (r *Row) Empty() bool { return r.cellsSet == 0 && r.ID == "" }

// clearable text columns accept "-" for "no value".
var clearable = map[string]bool{"location": true, "group": true, "sub_stream_url": true, "username": true, "onvif_url": true, "ptz_profile": true, "snapshot_url": true, "note": true}

// ParseBool reads yes/no in the forms people type.
func ParseBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "y", "true", "1", "on", "x", "áno", "ano", "a", "ja", "oui", "si", "sí", "tak", "igen", "da":
		return true, true
	case "no", "n", "false", "0", "off", "nie", "ne", "nein", "non", "nem":
		return false, true
	}
	return false, false
}

// set reads one cell into the row.
func (r *Row) set(key, raw string) {
	v := raw
	if key != "password" && key != "relay_secret" {
		v = strings.TrimSpace(raw)
	}
	if v == "" {
		return
	}
	r.cellsSet++
	str := func(dst **string) {
		if v == "-" && clearable[key] {
			e := ""
			*dst = &e
			return
		}
		x := v
		*dst = &x
	}
	boolean := func(dst **bool) {
		b, ok := ParseBool(v)
		if !ok {
			r.Errors = append(r.Errors, fmt.Sprintf("%s: yes or no, not %q", key, short(v)))
			return
		}
		*dst = &b
	}
	integer := func(dst **int) {
		n, err := strconv.Atoi(v)
		if err != nil {
			// Excel may write a whole number as 7.0 or 7,0
			f, ferr := strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
			if ferr != nil || f != float64(int(f)) {
				r.Errors = append(r.Errors, fmt.Sprintf("%s: a whole number, not %q", key, short(v)))
				return
			}
			n = int(f)
		}
		*dst = &n
	}
	polygons := func(dst **json.RawMessage) {
		if v == "-" {
			v = "[]"
		}
		if !json.Valid([]byte(v)) {
			r.Errors = append(r.Errors, fmt.Sprintf("%s: JSON polygons like [[[0,0],[0.5,0],[0.5,0.5]]], not %q", key, short(v)))
			return
		}
		x := json.RawMessage(v)
		*dst = &x
	}
	switch key {
	case "id":
		r.ID = v
	case "name":
		str(&r.Name)
	case "location":
		str(&r.Location)
	case "group":
		str(&r.Group)
	case "enabled":
		boolean(&r.Enabled)
	case "connection":
		x := strings.ToLower(v)
		r.Connection = &x
	case "make":
		x := strings.ToLower(v)
		r.Make = &x
	case "ip":
		str(&r.IP)
	case "main_stream_url":
		str(&r.URL)
	case "sub_stream_url":
		str(&r.SubURL)
	case "transport":
		x := strings.ToLower(v)
		r.Transport = &x
	case "username":
		str(&r.Username)
	case "password":
		str(&r.Password)
	case "relay_secret":
		str(&r.RelaySecret)
	case "onvif_url":
		str(&r.OnvifURL)
	case "ptz_profile":
		str(&r.PTZProfile)
	case "snapshot_url":
		str(&r.SnapshotURL)
	case "recording":
		x := strings.ToLower(v)
		r.Recording = &x
	case "pre_event_s":
		integer(&r.PreEvent)
	case "post_event_s":
		integer(&r.PostEvent)
	case "event_kinds":
		kinds := []string{}
		if v != "-" && !strings.EqualFold(v, "none") {
			for _, k := range strings.FieldsFunc(strings.ToLower(v), func(c rune) bool { return c == ',' || c == ';' || c == '|' || c == ' ' }) {
				kinds = append(kinds, k)
			}
		}
		r.EventKinds = &kinds
	case "retention_days":
		integer(&r.RetentionDays)
	case "motion_detection":
		boolean(&r.MotionDetect)
	case "motion_sensitivity":
		integer(&r.MotionSensitivity)
	case "motion_zones":
		polygons(&r.MotionZones)
	case "privacy_masks":
		polygons(&r.PrivacyMasks)
	case "audio":
		boolean(&r.Audio)
	case "audio_record":
		boolean(&r.AudioRecord)
	case "sort":
		integer(&r.Sort)
	case "note":
		str(&r.Note)
	default:
		r.cellsSet-- // export-only columns are not read
	}
}

func short(s string) string {
	if len([]rune(s)) > 40 {
		return string([]rune(s)[:40]) + "…"
	}
	return s
}
