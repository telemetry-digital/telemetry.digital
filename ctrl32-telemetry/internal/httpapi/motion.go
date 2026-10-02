package httpapi

// Motion detection on the server (spec 019): settings of a camera. The detection itself is internal/video/motion.go.

import (
	"encoding/json"
	"net/url"
	"strings"
)

// applyMotion validates the motion settings of the camera form onto a row.
func applyMotion(in *cameraIn, c *cameraRow) string {
	if in.Audio != nil {
		c.audio = *in.Audio
	}
	if in.AudioRecord != nil {
		c.audioRecord = *in.AudioRecord
	}
	if in.MotionDetect != nil {
		c.motion = *in.MotionDetect
	}
	if in.MotionSensitivity != nil {
		c.motionSens = *in.MotionSensitivity
	}
	if c.motionSens < 1 || c.motionSens > 100 {
		return "motion_sensitivity: 1 to 100"
	}
	if in.MotionZones != nil {
		z, msg := validateMasks(*in.MotionZones)
		if msg != "" {
			return strings.Replace(msg, "privacy_masks", "motion_zones", 1)
		}
		c.zones, _ = json.Marshal(z)
	}
	if in.SnapshotURL != nil {
		c.snapshotURL = strings.TrimSpace(*in.SnapshotURL)
	}
	if c.snapshotURL != "" {
		u, err := url.Parse(c.snapshotURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "snapshot_url: http(s)://camera/… address of one JPEG picture"
		}
		if u.User != nil {
			return "snapshot_url: without user name and password (the camera's are used)"
		}
		if c.source == "push" {
			return "snapshot_url: not for cameras that send their video through a relay"
		}
		if len(c.snapshotURL) > 500 {
			return "snapshot_url: at most 500 characters"
		}
	}
	return ""
}

// storeExtras writes the JSON, motion, sound and note columns of a camera (after its insert or update).
const storeExtras = `UPDATE core.camera SET privacy_masks = $2::jsonb, motion_detect = $3, motion_sensitivity = $4, motion_zones = $5::jsonb,
	snapshot_url = $6, audio = $7, audio_record = $8, note = $9 WHERE id = $1::uuid`

func extrasArgs(c cameraRow) []any {
	return []any{c.id, string(orEmpty(c.masks)), c.motion, c.motionSens, string(orEmpty(c.zones)), c.snapshotURL, c.audio, c.audioRecord, c.note}
}
