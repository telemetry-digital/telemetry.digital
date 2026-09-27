// Package onvif is a small ONVIF client (spec 013): device capabilities, media profiles and stream addresses, events
// through a pull-point subscription (motion, tampering, line crossing, intrusion, digital inputs — detected by the
// camera, so the server never decodes video), PTZ and WS-Discovery. SOAP 1.2 with WS-Security UsernameToken digest;
// the camera's clock offset is measured first, since cameras reject tokens with a skewed creation time.
package onvif

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Namespaces used in requests.
const (
	nsEnv    = "http://www.w3.org/2003/05/soap-envelope"
	nsDevice = "http://www.onvif.org/ver10/device/wsdl"
	nsMedia  = "http://www.onvif.org/ver10/media/wsdl"
	nsEvents = "http://www.onvif.org/ver10/events/wsdl"
	nsPTZ    = "http://www.onvif.org/ver20/ptz/wsdl"
	nsSchema = "http://www.onvif.org/ver10/schema"
	nsWSNT   = "http://docs.oasis-open.org/wsn/b-2"
	nsWSA    = "http://www.w3.org/2005/08/addressing"
)

// Client talks to one camera.
type Client struct {
	Device   string // http://host[:port]/onvif/device_service
	Username string
	Password string
	HTTP     *http.Client

	mu     sync.Mutex
	offset time.Duration // camera clock minus ours
	synced bool
	caps   *Capabilities
}

// Capabilities are the service addresses of the camera.
type Capabilities struct {
	Media  string `json:"media"`
	Events string `json:"events"`
	PTZ    string `json:"ptz"`
}

// Profile is a media profile with its video stream.
type Profile struct {
	Token    string `json:"token"`
	Name     string `json:"name"`
	Encoding string `json:"encoding"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	PTZ      bool   `json:"ptz"`
	URI      string `json:"uri,omitempty"`
}

// Preset is a stored PTZ position.
type Preset struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// Fault is a SOAP fault or an HTTP error of the camera.
type Fault struct {
	Status int
	Reason string
}

func (f *Fault) Error() string {
	if f.Reason == "" {
		return fmt.Sprintf("camera answered HTTP %d", f.Status)
	}
	return "camera refused: " + f.Reason
}

// ErrAuth is returned when the camera rejects the credentials.
var ErrAuth = errors.New("the camera rejected the user name or password")

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// security builds the WS-Security header (UsernameToken with password digest).
func (c *Client) security() string {
	if c.Username == "" {
		return ""
	}
	c.mu.Lock()
	created := time.Now().Add(c.offset).UTC().Format("2006-01-02T15:04:05.000Z")
	c.mu.Unlock()
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(c.Password))
	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return `<wsse:Security s:mustUnderstand="1" xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">` +
		`<wsse:UsernameToken><wsse:Username>` + esc(c.Username) + `</wsse:Username>` +
		`<wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">` + digest + `</wsse:Password>` +
		`<wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">` + base64.StdEncoding.EncodeToString(nonce) + `</wsse:Nonce>` +
		`<wsu:Created>` + created + `</wsu:Created></wsse:UsernameToken></wsse:Security>`
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// call posts a SOAP request and decodes the body into out (by local element names).
func (c *Client) call(ctx context.Context, addr, action, body string, out any, extraHeader string, auth bool) error {
	hdr := extraHeader
	if auth {
		hdr = c.security() + hdr
	}
	env := `<?xml version="1.0" encoding="UTF-8"?><s:Envelope xmlns:s="` + nsEnv + `" xmlns:tds="` + nsDevice + `" xmlns:trt="` + nsMedia +
		`" xmlns:tev="` + nsEvents + `" xmlns:tptz="` + nsPTZ + `" xmlns:tt="` + nsSchema + `" xmlns:wsnt="` + nsWSNT + `" xmlns:wsa="` + nsWSA + `">` +
		`<s:Header>` + hdr + `</s:Header><s:Body>` + body + `</s:Body></s:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr, strings.NewReader(env))
	if err != nil {
		return err
	}
	ct := "application/soap+xml; charset=utf-8"
	if action != "" {
		ct += `; action="` + action + `"`
	}
	req.Header.Set("Content-Type", ct)
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	var e struct {
		Body struct {
			Fault *struct {
				Reason struct {
					Text string `xml:"Text"`
				} `xml:"Reason"`
				Code struct {
					Subcode struct {
						Value string `xml:"Value"`
					} `xml:"Subcode"`
				} `xml:"Code"`
			} `xml:"Fault"`
			Inner []byte `xml:",innerxml"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(raw, &e); err != nil {
		if resp.StatusCode == 401 {
			return ErrAuth
		}
		return &Fault{Status: resp.StatusCode}
	}
	if e.Body.Fault != nil {
		reason := strings.TrimSpace(e.Body.Fault.Reason.Text)
		if resp.StatusCode == 401 || strings.Contains(strings.ToLower(e.Body.Fault.Code.Subcode.Value), "notauthorized") ||
			strings.Contains(strings.ToLower(e.Body.Fault.Code.Subcode.Value), "failedauthentication") {
			return ErrAuth
		}
		return &Fault{Status: resp.StatusCode, Reason: reason}
	}
	if resp.StatusCode == 401 {
		return ErrAuth
	}
	if resp.StatusCode >= 300 {
		return &Fault{Status: resp.StatusCode}
	}
	if out == nil {
		return nil
	}
	return xml.Unmarshal(append(append([]byte("<Body>"), e.Body.Inner...), "</Body>"...), out)
}

// sync measures the camera's clock (GetSystemDateAndTime needs no authentication).
func (c *Client) sync(ctx context.Context) {
	c.mu.Lock()
	done := c.synced
	c.mu.Unlock()
	if done {
		return
	}
	var r struct {
		T struct {
			UTC struct {
				Date struct{ Year, Month, Day int }     `xml:"Date"`
				Time struct{ Hour, Minute, Second int } `xml:"Time"`
			} `xml:"SystemDateAndTime>UTCDateTime"`
		} `xml:"GetSystemDateAndTimeResponse"`
	}
	if err := c.call(ctx, c.Device, "", `<tds:GetSystemDateAndTime/>`, &r, "", false); err == nil && r.T.UTC.Date.Year > 2000 {
		d, t := r.T.UTC.Date, r.T.UTC.Time
		cam := time.Date(d.Year, time.Month(d.Month), d.Day, t.Hour, t.Minute, t.Second, 0, time.UTC)
		off := time.Until(cam)
		if off.Abs() < 2*time.Second {
			off = 0 // the camera reports whole seconds: smaller differences are noise
		}
		c.mu.Lock()
		c.offset = off
		c.mu.Unlock()
	}
	c.mu.Lock()
	c.synced = true
	c.mu.Unlock()
}

// DateTime asks the camera for its clock (GetSystemDateAndTime needs no authentication); the relay's self-check uses
// it to prove that the camera's ONVIF service answers.
func (c *Client) DateTime(ctx context.Context) (time.Time, error) {
	var r struct {
		T struct {
			UTC struct {
				Date struct{ Year, Month, Day int }     `xml:"Date"`
				Time struct{ Hour, Minute, Second int } `xml:"Time"`
			} `xml:"SystemDateAndTime>UTCDateTime"`
		} `xml:"GetSystemDateAndTimeResponse"`
	}
	if err := c.call(ctx, c.Device, "", `<tds:GetSystemDateAndTime/>`, &r, "", false); err != nil {
		return time.Time{}, err
	}
	d, t := r.T.UTC.Date, r.T.UTC.Time
	if d.Year < 2000 {
		return time.Time{}, errors.New("the camera did not report its time")
	}
	return time.Date(d.Year, time.Month(d.Month), d.Day, t.Hour, t.Minute, t.Second, 0, time.UTC), nil
}

// Capabilities returns the service addresses (cached).
func (c *Client) Capabilities(ctx context.Context) (*Capabilities, error) {
	c.mu.Lock()
	if c.caps != nil {
		defer c.mu.Unlock()
		return c.caps, nil
	}
	c.mu.Unlock()
	c.sync(ctx)
	var r struct {
		C struct {
			Media  string `xml:"Capabilities>Media>XAddr"`
			Events string `xml:"Capabilities>Events>XAddr"`
			PTZ    string `xml:"Capabilities>PTZ>XAddr"`
		} `xml:"GetCapabilitiesResponse"`
	}
	if err := c.call(ctx, c.Device, nsDevice+"/GetCapabilities", `<tds:GetCapabilities><tds:Category>All</tds:Category></tds:GetCapabilities>`, &r, "", true); err != nil {
		return nil, err
	}
	caps := &Capabilities{Media: c.fix(r.C.Media), Events: c.fix(r.C.Events), PTZ: c.fix(r.C.PTZ)}
	if caps.Media == "" {
		return nil, errors.New("the camera reports no media service")
	}
	c.mu.Lock()
	c.caps = caps
	c.mu.Unlock()
	return caps, nil
}

// fix keeps the scheme and host we reached the camera with: cameras behind NAT or port forwarding report internal
// addresses in their capabilities.
func (c *Client) fix(addr string) string {
	if addr == "" {
		return ""
	}
	u, err := url.Parse(addr)
	d, err2 := url.Parse(c.Device)
	if err != nil || err2 != nil {
		return addr
	}
	u.Scheme, u.Host = d.Scheme, d.Host
	return u.String()
}

// Profiles lists the media profiles with their stream addresses.
func (c *Client) Profiles(ctx context.Context) ([]Profile, error) {
	caps, err := c.Capabilities(ctx)
	if err != nil {
		return nil, err
	}
	var r struct {
		P []struct {
			Token string `xml:"token,attr"`
			Name  string `xml:"Name"`
			Enc   struct {
				Encoding string `xml:"Encoding"`
				W        int    `xml:"Resolution>Width"`
				H        int    `xml:"Resolution>Height"`
			} `xml:"VideoEncoderConfiguration"`
			PTZ *struct{} `xml:"PTZConfiguration"`
		} `xml:"GetProfilesResponse>Profiles"`
	}
	if err := c.call(ctx, caps.Media, nsMedia+"/GetProfiles", `<trt:GetProfiles/>`, &r, "", true); err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(r.P))
	for _, p := range r.P {
		pr := Profile{Token: p.Token, Name: p.Name, Encoding: p.Enc.Encoding, Width: p.Enc.W, Height: p.Enc.H, PTZ: p.PTZ != nil}
		var u struct {
			URI string `xml:"GetStreamUriResponse>MediaUri>Uri"`
		}
		body := `<trt:GetStreamUri><trt:StreamSetup><tt:Stream>RTP-Unicast</tt:Stream><tt:Transport><tt:Protocol>RTSP</tt:Protocol></tt:Transport></trt:StreamSetup><trt:ProfileToken>` + esc(p.Token) + `</trt:ProfileToken></trt:GetStreamUri>`
		if err := c.call(ctx, caps.Media, nsMedia+"/GetStreamUri", body, &u, "", true); err == nil {
			pr.URI = strings.TrimSpace(u.URI)
		}
		out = append(out, pr)
	}
	return out, nil
}

// ---- PTZ -------------------------------------------------------------------------------------------------------

func clamp(v float64) float64 { return max(-1, min(1, v)) }

func num(v float64) string { return strconv.FormatFloat(clamp(v), 'f', 3, 64) }

func (c *Client) ptz(ctx context.Context) (string, error) {
	caps, err := c.Capabilities(ctx)
	if err != nil {
		return "", err
	}
	if caps.PTZ == "" {
		return "", errors.New("the camera has no PTZ")
	}
	return caps.PTZ, nil
}

// Move starts a continuous movement: pan, tilt and zoom speeds from -1 to 1.
func (c *Client) Move(ctx context.Context, profile string, pan, tilt, zoom float64) error {
	addr, err := c.ptz(ctx)
	if err != nil {
		return err
	}
	body := `<tptz:ContinuousMove><tptz:ProfileToken>` + esc(profile) + `</tptz:ProfileToken><tptz:Velocity><tt:PanTilt x="` + num(pan) + `" y="` + num(tilt) + `"/><tt:Zoom x="` + num(zoom) + `"/></tptz:Velocity></tptz:ContinuousMove>`
	return c.call(ctx, addr, nsPTZ+"/ContinuousMove", body, nil, "", true)
}

// Stop ends a movement.
func (c *Client) Stop(ctx context.Context, profile string) error {
	addr, err := c.ptz(ctx)
	if err != nil {
		return err
	}
	return c.call(ctx, addr, nsPTZ+"/Stop", `<tptz:Stop><tptz:ProfileToken>`+esc(profile)+`</tptz:ProfileToken><tptz:PanTilt>true</tptz:PanTilt><tptz:Zoom>true</tptz:Zoom></tptz:Stop>`, nil, "", true)
}

// Presets lists the stored positions.
func (c *Client) Presets(ctx context.Context, profile string) ([]Preset, error) {
	addr, err := c.ptz(ctx)
	if err != nil {
		return nil, err
	}
	var r struct {
		P []struct {
			Token string `xml:"token,attr"`
			Name  string `xml:"Name"`
		} `xml:"GetPresetsResponse>Preset"`
	}
	if err := c.call(ctx, addr, nsPTZ+"/GetPresets", `<tptz:GetPresets><tptz:ProfileToken>`+esc(profile)+`</tptz:ProfileToken></tptz:GetPresets>`, &r, "", true); err != nil {
		return nil, err
	}
	out := make([]Preset, 0, len(r.P))
	for _, p := range r.P {
		out = append(out, Preset{Token: p.Token, Name: p.Name})
	}
	return out, nil
}

// GotoPreset moves to a stored position.
func (c *Client) GotoPreset(ctx context.Context, profile, preset string) error {
	addr, err := c.ptz(ctx)
	if err != nil {
		return err
	}
	return c.call(ctx, addr, nsPTZ+"/GotoPreset", `<tptz:GotoPreset><tptz:ProfileToken>`+esc(profile)+`</tptz:ProfileToken><tptz:PresetToken>`+esc(preset)+`</tptz:PresetToken></tptz:GotoPreset>`, nil, "", true)
}
