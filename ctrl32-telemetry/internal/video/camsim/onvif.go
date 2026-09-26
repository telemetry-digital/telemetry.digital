package camsim

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ONVIF simulates the ONVIF side of the cameras (spec 013): device, media, events (motion) and PTZ services at
// http://host/onvif/<label>/…, WS-Security digest authentication and WS-Discovery answers. PTZ moves the simulated
// picture, motion can be switched on and off (POST /sim/motion/<label>?on=1) or toggled periodically.
type ONVIF struct {
	RTSPHost    string // host:port of the RTSP simulator, for stream addresses
	HTTPHost    string // host:port where this handler is reachable, for service addresses
	User, Pass  string
	Codec       string        // stream codec of the simulated profiles (h264 or mjpeg)
	MotionEvery time.Duration // > 0: motion for a third of this period, repeatedly

	mu    sync.Mutex
	cams  map[string]*simCam
	subID int
}

type simCam struct {
	motion        bool
	subs          map[string]chan string // subscription → queued notification XML
	pan, tilt, zm float64
	vp, vt, vz    float64
	moved         time.Time
}

func (o *ONVIF) cam(label string) *simCam {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cams == nil {
		o.cams = map[string]*simCam{}
	}
	c := o.cams[label]
	if c == nil {
		c = &simCam{subs: map[string]chan string{}, zm: 0, moved: time.Now()}
		o.cams[label] = c
	}
	return c
}

// View is the simulated PTZ position of a camera (pan, tilt in -1…1, zoom 0…1).
func (o *ONVIF) View(label string) (pan, tilt, zoom float64) {
	if o == nil {
		return 0, 0, 0
	}
	c := o.cam(label)
	o.mu.Lock()
	defer o.mu.Unlock()
	c.integrate()
	return c.pan, c.tilt, c.zm
}

func (c *simCam) integrate() {
	dt := time.Since(c.moved).Seconds()
	c.moved = time.Now()
	c.pan = max(-1, min(1, c.pan+c.vp*dt*0.5))
	c.tilt = max(-1, min(1, c.tilt+c.vt*dt*0.5))
	c.zm = max(0, min(1, c.zm+c.vz*dt*0.5))
}

// SetMotion switches the motion state of a camera and notifies its subscribers.
func (o *ONVIF) SetMotion(label string, on bool) {
	c := o.cam(label)
	o.mu.Lock()
	defer o.mu.Unlock()
	if c.motion == on {
		return
	}
	c.motion = on
	msg := motionMsg(on, "Changed")
	for _, ch := range c.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

func motionMsg(on bool, op string) string {
	return `<wsnt:NotificationMessage><wsnt:Topic Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">tns1:RuleEngine/CellMotionDetector/Motion</wsnt:Topic>` +
		`<wsnt:Message><tt:Message UtcTime="` + time.Now().UTC().Format("2006-01-02T15:04:05.000Z") + `" PropertyOperation="` + op + `">` +
		`<tt:Source><tt:SimpleItem Name="VideoSourceConfigurationToken" Value="vsc"/><tt:SimpleItem Name="Rule" Value="MyMotionDetectorRule"/></tt:Source>` +
		`<tt:Data><tt:SimpleItem Name="IsMotion" Value="` + fmt.Sprint(on) + `"/></tt:Data></tt:Message></wsnt:Message></wsnt:NotificationMessage>`
}

// Run toggles motion periodically for the given cameras (MotionEvery > 0).
func (o *ONVIF) Run(labels []string, stop <-chan struct{}) {
	if o.MotionEvery <= 0 {
		return
	}
	t := time.NewTicker(o.MotionEvery / 3)
	defer t.Stop()
	for n := 0; ; n++ {
		select {
		case <-stop:
			return
		case <-t.C:
			for _, l := range labels {
				o.SetMotion(l, n%3 == 0)
			}
		}
	}
}

func (o *ONVIF) authOK(body string) bool {
	if o.User == "" {
		return true
	}
	var env struct {
		Token struct {
			User     string `xml:"Username"`
			Password string `xml:"Password"`
			Nonce    string `xml:"Nonce"`
			Created  string `xml:"Created"`
		} `xml:"Header>Security>UsernameToken"`
	}
	if xml.Unmarshal([]byte(body), &env) != nil || env.Token.User != o.User {
		return false
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Token.Nonce)
	if err != nil {
		return false
	}
	created, err := time.Parse(time.RFC3339Nano, env.Token.Created)
	if err != nil || time.Since(created).Abs() > 5*time.Minute {
		return false
	}
	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(env.Token.Created))
	h.Write([]byte(o.Pass))
	return base64.StdEncoding.EncodeToString(h.Sum(nil)) == env.Token.Password
}

const envOpen = `<?xml version="1.0" encoding="UTF-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema" xmlns:tds="http://www.onvif.org/ver10/device/wsdl" xmlns:trt="http://www.onvif.org/ver10/media/wsdl" xmlns:tev="http://www.onvif.org/ver10/events/wsdl" xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2" xmlns:wsa="http://www.w3.org/2005/08/addressing"><s:Body>`
const envClose = `</s:Body></s:Envelope>`

func reply(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, envOpen+body+envClose)
}

func fault(w http.ResponseWriter, code int, sub, reason string) {
	reply(w, code, `<s:Fault><s:Code><s:Value>s:Sender</s:Value><s:Subcode><s:Value>`+sub+`</s:Value></s:Subcode></s:Code><s:Reason><s:Text xml:lang="en">`+reason+`</s:Text></s:Reason></s:Fault>`)
}

// op returns the local name of the first element in the SOAP body.
func op(body string) string {
	dec := xml.NewDecoder(strings.NewReader(body))
	inBody := false
	for {
		t, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := t.(xml.StartElement); ok {
			if inBody {
				return se.Name.Local
			}
			if se.Name.Local == "Body" {
				inBody = true
			}
		}
	}
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i < 0 {
		return ""
	}
	s = s[i+len(a):]
	if j := strings.Index(s, b); j >= 0 {
		return s[:j]
	}
	return ""
}

// ServeHTTP implements the services.
func (o *ONVIF) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/sim/motion/") {
		o.SetMotion(strings.TrimPrefix(r.URL.Path, "/sim/motion/"), r.URL.Query().Get("on") == "1")
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/") // onvif/<label>/<service>[/sub/<id>]
	if len(parts) < 3 || parts[0] != "onvif" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	label, service := parts[1], parts[2]
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	body := string(raw)
	name := op(body)
	if name != "GetSystemDateAndTime" && !o.authOK(body) {
		fault(w, 400, "ter:NotAuthorized", "Sender not authorized")
		return
	}
	base := "http://" + o.HTTPHost + "/onvif/" + label
	c := o.cam(label)
	switch name {
	case "GetSystemDateAndTime":
		n := time.Now().UTC()
		reply(w, 200, fmt.Sprintf(`<tds:GetSystemDateAndTimeResponse><tds:SystemDateAndTime><tt:DateTimeType>NTP</tt:DateTimeType><tt:UTCDateTime><tt:Time><tt:Hour>%d</tt:Hour><tt:Minute>%d</tt:Minute><tt:Second>%d</tt:Second></tt:Time><tt:Date><tt:Year>%d</tt:Year><tt:Month>%d</tt:Month><tt:Day>%d</tt:Day></tt:Date></tt:UTCDateTime></tds:SystemDateAndTime></tds:GetSystemDateAndTimeResponse>`,
			n.Hour(), n.Minute(), n.Second(), n.Year(), int(n.Month()), n.Day()))
	case "GetCapabilities":
		reply(w, 200, `<tds:GetCapabilitiesResponse><tds:Capabilities><tt:Device><tt:XAddr>`+base+`/device_service</tt:XAddr></tt:Device><tt:Events><tt:XAddr>`+base+`/events</tt:XAddr></tt:Events><tt:Media><tt:XAddr>`+base+`/media</tt:XAddr></tt:Media><tt:PTZ><tt:XAddr>`+base+`/ptz</tt:XAddr></tt:PTZ></tds:Capabilities></tds:GetCapabilitiesResponse>`)
	case "GetProfiles":
		enc := "H264"
		if o.Codec == "mjpeg" {
			enc = "JPEG"
		}
		prof := func(tok, n string, w, h int) string {
			return `<trt:Profiles token="` + tok + `" fixed="true"><tt:Name>` + n + `</tt:Name><tt:VideoEncoderConfiguration token="enc_` + tok + `"><tt:Name>` + n + `</tt:Name><tt:Encoding>` + enc + `</tt:Encoding><tt:Resolution><tt:Width>` + fmt.Sprint(w) + `</tt:Width><tt:Height>` + fmt.Sprint(h) + `</tt:Height></tt:Resolution></tt:VideoEncoderConfiguration><tt:PTZConfiguration token="ptz"><tt:Name>PTZ</tt:Name></tt:PTZConfiguration></trt:Profiles>`
		}
		reply(w, 200, `<trt:GetProfilesResponse>`+prof("main", "mainStream", 640, 352)+prof("sub", "subStream", 256, 144)+`</trt:GetProfilesResponse>`)
	case "GetStreamUri":
		size := "640x352"
		if strings.Contains(between(body, "ProfileToken>", "<"), "sub") {
			size = "256x144"
		}
		codec := o.Codec
		if codec == "" {
			codec = "h264"
		}
		reply(w, 200, `<trt:GetStreamUriResponse><trt:MediaUri><tt:Uri>rtsp://`+o.RTSPHost+`/`+label+`/`+codec+`/`+size+`</tt:Uri><tt:InvalidAfterConnect>false</tt:InvalidAfterConnect><tt:InvalidAfterReboot>false</tt:InvalidAfterReboot><tt:Timeout>PT0S</tt:Timeout></trt:MediaUri></trt:GetStreamUriResponse>`)
	case "CreatePullPointSubscription":
		o.mu.Lock()
		o.subID++
		id := fmt.Sprint(o.subID)
		ch := make(chan string, 64)
		ch <- motionMsg(c.motion, "Initialized")
		c.subs[id] = ch
		o.mu.Unlock()
		reply(w, 200, `<tev:CreatePullPointSubscriptionResponse><tev:SubscriptionReference><wsa:Address>`+base+`/events/sub/`+id+`</wsa:Address></tev:SubscriptionReference><wsnt:CurrentTime>`+time.Now().UTC().Format(time.RFC3339)+`</wsnt:CurrentTime><wsnt:TerminationTime>`+time.Now().Add(2*time.Minute).UTC().Format(time.RFC3339)+`</wsnt:TerminationTime></tev:CreatePullPointSubscriptionResponse>`)
	case "PullMessages", "Renew", "Unsubscribe":
		if len(parts) < 5 {
			fault(w, 400, "ter:InvalidArgVal", "no subscription")
			return
		}
		o.mu.Lock()
		ch := c.subs[parts[4]]
		if name == "Unsubscribe" {
			delete(c.subs, parts[4])
		}
		o.mu.Unlock()
		if ch == nil {
			fault(w, 400, "ter:InvalidArgVal", "unknown subscription")
			return
		}
		switch name {
		case "Renew":
			reply(w, 200, `<wsnt:RenewResponse><wsnt:TerminationTime>`+time.Now().Add(2*time.Minute).UTC().Format(time.RFC3339)+`</wsnt:TerminationTime></wsnt:RenewResponse>`)
		case "Unsubscribe":
			reply(w, 200, `<wsnt:UnsubscribeResponse/>`)
		default:
			var msgs []string
			select {
			case m := <-ch:
				msgs = append(msgs, m)
			case <-time.After(3 * time.Second):
			case <-r.Context().Done():
				return
			}
			for len(ch) > 0 && len(msgs) < 50 {
				msgs = append(msgs, <-ch)
			}
			reply(w, 200, `<tev:PullMessagesResponse><tev:CurrentTime>`+time.Now().UTC().Format(time.RFC3339)+`</tev:CurrentTime><tev:TerminationTime>`+time.Now().Add(time.Minute).UTC().Format(time.RFC3339)+`</tev:TerminationTime>`+strings.Join(msgs, "")+`</tev:PullMessagesResponse>`)
		}
	case "ContinuousMove", "Stop", "GotoPreset":
		o.mu.Lock()
		c.integrate()
		switch name {
		case "ContinuousMove":
			fmt.Sscan(between(between(body, "PanTilt", "/>")+`"`, `x="`, `"`), &c.vp)
			fmt.Sscan(between(between(body, "PanTilt", "/>")+`"`, `y="`, `"`), &c.vt)
			fmt.Sscan(between(between(body, "Zoom", "/>")+`"`, `x="`, `"`), &c.vz)
		case "Stop":
			c.vp, c.vt, c.vz = 0, 0, 0
		case "GotoPreset":
			switch between(body, "PresetToken>", "<") {
			case "1":
				c.pan, c.tilt, c.zm = 0, 0, 0
			case "2":
				c.pan, c.tilt, c.zm = -0.6, 0.2, 0.3
			case "3":
				c.pan, c.tilt, c.zm = 0.7, -0.3, 0.6
			}
		}
		o.mu.Unlock()
		reply(w, 200, `<tptz:`+name+`Response/>`)
	case "GetPresets":
		reply(w, 200, `<tptz:GetPresetsResponse><tptz:Preset token="1"><tt:Name>Home</tt:Name></tptz:Preset><tptz:Preset token="2"><tt:Name>Gate</tt:Name></tptz:Preset><tptz:Preset token="3"><tt:Name>Parking</tt:Name></tptz:Preset></tptz:GetPresetsResponse>`)
	default:
		fault(w, 400, "ter:ActionNotSupported", "not supported by the simulator: "+name+" at "+service)
	}
}

// ServeDiscovery answers WS-Discovery probes on conn for the given camera labels until conn is closed.
func (o *ONVIF) ServeDiscovery(conn *net.UDPConn, labels []string) {
	buf := make([]byte, 64<<10)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if !strings.Contains(string(buf[:n]), "Probe") {
			continue
		}
		var matches strings.Builder
		for _, l := range labels {
			matches.WriteString(`<d:ProbeMatch><w:EndpointReference><w:Address>urn:uuid:camsim-` + l + `</w:Address></w:EndpointReference><d:Types>dn:NetworkVideoTransmitter</d:Types>` +
				`<d:Scopes>onvif://www.onvif.org/type/video_encoder onvif://www.onvif.org/name/` + l + ` onvif://www.onvif.org/hardware/camsim</d:Scopes>` +
				`<d:XAddrs>http://` + o.HTTPHost + `/onvif/` + l + `/device_service</d:XAddrs><d:MetadataVersion>1</d:MetadataVersion></d:ProbeMatch>`)
		}
		resp := `<?xml version="1.0" encoding="UTF-8"?><e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl"><e:Body><d:ProbeMatches>` + matches.String() + `</d:ProbeMatches></e:Body></e:Envelope>`
		_, _ = conn.WriteToUDP([]byte(resp), from)
	}
}
