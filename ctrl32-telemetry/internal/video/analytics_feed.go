package video

// Object counting (spec 029) reads the same streams as viewers: the server's own subscription to the camera's
// sub-stream (the main stream when there is none) feeds the analytics worker. Works for direct and relayed cameras
// alike, and the camera is connected only once.

// AnalyticsFeed subscribes to the stream object counting analyses. Close the subscription when done.
func (m *Manager) AnalyticsFeed(id string) (*Sub, error) {
	src := m.Source(id, true)
	if src == nil {
		return nil, ErrNoCamera
	}
	return src.subscribe(true), nil
}

// AnnexB is a video sample as a byte stream for a decoder: start codes before the NAL units, the parameter sets in
// front of key frames; an MJPEG sample is its JPEG picture.
func AnnexB(t *Track, s *Sample) []byte {
	if s == nil || t == nil {
		return nil
	}
	if s.JPEG != nil {
		return s.JPEG
	}
	au := withParams(s.Key, t, s.AU)
	n := 0
	for _, u := range au {
		n += 4 + len(u)
	}
	out := make([]byte, 0, n)
	for _, u := range au {
		if len(u) == 0 {
			continue
		}
		out = append(out, 0, 0, 0, 1)
		out = append(out, u...)
	}
	return out
}
