package config

import (
	"strings"
	"testing"
)

func base() *Config {
	c := Defaults()
	c.DB.URL = "postgres://x"
	c.Security.Pepper = "0123456789abcdef0"
	c.Security.SecretKey = "0123456789abcdef0"
	c.Report.StorageDir = "/var/lib/x/reports"
	c.MQTT.Listen = "127.0.0.1:1883"
	return c
}

// Remote cameras send video from other networks: on an internet server the push listener must use TLS (spec 011).
func TestPushListenerNeedsTLSOnInternet(t *testing.T) {
	c := base()
	c.Profile, c.HTTP.Listen, c.Video.PushListen = "internet", "127.0.0.1:8080", ":8322"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "push_listen") {
		t.Fatalf("want a TLS error, got %v", err)
	}
	c = base()
	c.Profile, c.HTTP.Listen, c.Video.PushListen = "internet", "127.0.0.1:8080", ":8322"
	c.Video.PushTLSCert, c.Video.PushTLSKey = "/etc/c.pem", "/etc/k.pem"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c = base()
	c.Video.PushListen = ":8322" // intranet: digest authentication without TLS is allowed
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Video.StorageDir != "/var/lib/x/video" || c.Video.MaxDiskGB != 50 {
		t.Fatalf("video defaults %+v", c.Video)
	}
}
