// Command camsim simulates IP cameras for development and tests (specs 010, 013).
//
//	go run ./tools/camsim -rtsp :8554 -http :8555 -fps 10 -onvif gate,yard -onvif-user admin -onvif-pass admin -motion-every 30s
//
// RTSP:  rtsp://127.0.0.1:8554/<label>/<h264|mjpeg>/<W>x<H>   e.g. rtsp://127.0.0.1:8554/gate/h264/640x480
// HTTP:  http://127.0.0.1:8555/<label>/<W>x<H>                 (MJPEG)
// ONVIF: http://127.0.0.1:8555/onvif/<label>/device_service   (profiles, events with motion, PTZ that moves the picture)
//
//	POST http://127.0.0.1:8555/sim/motion/<label>?on=1        switches motion on (on=0 off)
//	GET  http://127.0.0.1:8555/sim/ptz/<label>                 PTZ position {"pan","tilt","zoom"}
//	GET  http://127.0.0.1:8555/snapshot/<label>/<W>x<H>           one JPEG picture (snapshot address)
//
// Cameras labelled STILL… show a standing scene until motion is switched on (for motion detection on the server).
// With -clip <dir> cameras labelled clip… play the JPEG pictures of the directory in a loop (object counting, spec 029).
package main

import (
	"bytes"
	"flag"
	"image/jpeg"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/camsim"
)

func main() {
	rtsp := flag.String("rtsp", ":8554", "RTSP listen address")
	httpAddr := flag.String("http", ":8555", "HTTP listen address for MJPEG and ONVIF (empty = off)")
	fps := flag.Int("fps", 10, "frames per second")
	loop := flag.Int("loop", 0, "encode this many frames once and repeat them (load tests); 0 = draw every frame")
	onvifLabels := flag.String("onvif", "", "comma-separated camera labels announced over WS-Discovery (any label answers ONVIF)")
	user := flag.String("onvif-user", "admin", "ONVIF user name (empty = no authentication)")
	pass := flag.String("onvif-pass", "admin", "ONVIF password")
	codec := flag.String("onvif-codec", "h264", "codec of the ONVIF profiles: h264 or mjpeg")
	motion := flag.Duration("motion-every", 0, "toggle motion on the announced cameras with this period (0 = only /sim/motion)")
	disco := flag.String("discovery", ":3702", "UDP address answering WS-Discovery probes (empty = off)")
	host := flag.String("host", "127.0.0.1", "host name put into ONVIF addresses")
	clip := flag.String("clip", "", "directory of JPEG pictures played in a loop by cameras labelled clip… (object counting)")
	flag.Parse()

	_, rport, _ := net.SplitHostPort(*rtsp)
	_, hport, _ := net.SplitHostPort(*httpAddr)
	ov := &camsim.ONVIF{RTSPHost: net.JoinHostPort(*host, rport), HTTPHost: net.JoinHostPort(*host, hport), User: *user, Pass: *pass, Codec: *codec, MotionEvery: *motion}
	s := &camsim.Server{RTSPAddr: *rtsp, FPS: *fps, Loop: *loop, Log: slog.Default(), ONVIF: ov}
	if *clip != "" {
		files, _ := filepath.Glob(filepath.Join(*clip, "*.jpg"))
		sort.Strings(files)
		for _, f := range files {
			if b, err := os.ReadFile(f); err == nil {
				if _, err := jpeg.DecodeConfig(bytes.NewReader(b)); err == nil {
					s.Clip = append(s.Clip, b)
				}
			}
		}
		log.Printf("camsim: clip of %d pictures from %s (cameras labelled clip…)", len(s.Clip), *clip)
	}
	if err := s.Start(); err != nil {
		log.Fatal(err)
	}
	var labels []string
	for _, l := range strings.Split(*onvifLabels, ",") {
		if l = strings.TrimSpace(l); l != "" {
			labels = append(labels, l)
		}
	}
	go ov.Run(labels, nil)
	if *disco != "" && len(labels) > 0 {
		if a, err := net.ResolveUDPAddr("udp4", *disco); err == nil {
			if c, err := net.ListenUDP("udp4", a); err == nil {
				go ov.ServeDiscovery(c, labels)
			} else {
				log.Printf("discovery: %v", err)
			}
		}
		if g, err := net.ResolveUDPAddr("udp4", "239.255.255.250:3702"); err == nil {
			if c, err := net.ListenMulticastUDP("udp4", nil, g); err == nil {
				go ov.ServeDiscovery(c, labels)
			}
		}
	}
	log.Printf("camsim: rtsp %s, http %s, %d fps, onvif %v", *rtsp, *httpAddr, *fps, labels)
	if *httpAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("/onvif/", ov)
		mux.Handle("/sim/", ov)
		mux.Handle("/snapshot/", http.StripPrefix("/snapshot", camsim.SnapshotHandler(ov)))
		mux.Handle("/", camsim.MJPEGHandler(*fps))
		log.Fatal(http.ListenAndServe(*httpAddr, mux))
	}
	select {}
}
