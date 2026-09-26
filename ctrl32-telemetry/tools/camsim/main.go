// Command camsim simulates IP cameras for development and tests (spec 010).
//
//	go run ./tools/camsim -rtsp :8554 -http :8555 -fps 10
//
// RTSP:  rtsp://127.0.0.1:8554/<label>/<h264|mjpeg>/<W>x<H>   e.g. rtsp://127.0.0.1:8554/gate/h264/640x480
// HTTP:  http://127.0.0.1:8555/<label>/<W>x<H>                 (MJPEG)
package main

import (
	"flag"
	"log"
	"log/slog"
	"net/http"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/camsim"
)

func main() {
	rtsp := flag.String("rtsp", ":8554", "RTSP listen address")
	httpAddr := flag.String("http", ":8555", "HTTP MJPEG listen address (empty = off)")
	fps := flag.Int("fps", 10, "frames per second")
	flag.Parse()
	s := &camsim.Server{RTSPAddr: *rtsp, FPS: *fps, Log: slog.Default()}
	if err := s.Start(); err != nil {
		log.Fatal(err)
	}
	log.Printf("camsim: rtsp %s, http %s, %d fps", *rtsp, *httpAddr, *fps)
	if *httpAddr != "" {
		log.Fatal(http.ListenAndServe(*httpAddr, camsim.MJPEGHandler(*fps)))
	}
	select {}
}
