package agent

// Read-only host health for the server screen (ADR-0025): software RAID from /proc/mdstat, the physical network links
// from /sys/class/net, the disks' own health (SMART, when smartctl is installed; cached for 10 minutes) and a pending
// reboot. Nothing here changes the host.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var smartCache struct {
	sync.Mutex
	at    time.Time
	disks []SMARTDisk
	state string
}

func (a *Server) hostHealth(ctx context.Context) *HostHealth {
	if isWindows() && !a.simulate {
		return nil
	}
	h := &HostHealth{RAID: []RAIDArray{}, Links: []NetLink{}, Disks: []SMARTDisk{}, Problems: []string{}}
	if a.simulate {
		h.RAID = parseMdstat(simMdstat)
		h.Links = []NetLink{{Name: "eth0", State: "up", SpeedMbps: 1000, Duplex: "full"}}
		h.SMART = "unavailable"
	} else {
		h.RAID = parseMdstat(readFile("/proc/mdstat"))
		h.Links = netLinks("/sys/class/net")
		h.Disks, h.SMART = smartDisks(ctx)
	}
	h.RebootRequired, _ = a.rebootRequired()
	for _, r := range h.RAID {
		if r.Health != "ok" {
			h.Problems = append(h.Problems, "RAID "+r.Name+": "+r.Health+strings.TrimSpace(" "+r.Progress))
		}
	}
	up := 0
	for _, l := range h.Links {
		if l.State == "up" {
			up++
		}
	}
	if len(h.Links) > 0 && up == 0 {
		h.Problems = append(h.Problems, "no network link is up")
	}
	for _, d := range h.Disks {
		if d.Health == "FAILED" {
			h.Problems = append(h.Problems, "disk "+d.Device+" reports FAILED (SMART)")
		}
	}
	if h.RebootRequired {
		h.Problems = append(h.Problems, "reboot required")
	}
	return h
}

const simMdstat = `Personalities : [raid0]
md0 : active raid0 sdb[1] sda[0]
      7813772288 blocks super 1.2 512k chunks

unused devices: <none>
`

var (
	mdLineRe   = regexp.MustCompile(`^(md\S*) : (\S+)(?: \((auto-)?read-only\))? ?(\S+)? ?(.*)$`)
	mdStatusRe = regexp.MustCompile(`\[(\d+)/(\d+)\] \[([U_]+)\]`)
	mdSyncRe   = regexp.MustCompile(`(recovery|resync|reshape|check)\s*=\s*([\d.]+%)(?:.*finish=(\S+))?`)
)

// parseMdstat reads the arrays of /proc/mdstat:
//
//	md0 : active raid1 sdb1[1] sda1[0]
//	      976630464 blocks super 1.2 [2/1] [U_]
//	      [=>...................]  recovery =  7.5% (73654336/976630464) finish=95.3min speed=157828K/sec
func parseMdstat(s string) []RAIDArray {
	out := []RAIDArray{}
	var cur *RAIDArray
	for _, l := range strings.Split(s, "\n") {
		if m := mdLineRe.FindStringSubmatch(l); m != nil {
			out = append(out, RAIDArray{Name: m[1], State: m[2], Level: m[4], Devices: strings.Fields(m[5]), Health: "ok"})
			cur = &out[len(out)-1]
			if cur.State != "active" {
				cur.Health = "failed"
			}
			for _, d := range cur.Devices {
				if strings.HasSuffix(d, "(F)") {
					cur.Health = "degraded"
				}
			}
			if !strings.HasPrefix(cur.Level, "raid") && cur.Level != "linear" { // "md0 : inactive sdb[1](S)"
				cur.Devices = strings.Fields(m[4] + " " + m[5])
				cur.Level = ""
			}
			continue
		}
		if cur == nil {
			continue
		}
		if !strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "\t") {
			cur = nil
			continue
		}
		if m := mdStatusRe.FindStringSubmatch(l); m != nil {
			cur.Status = m[0]
			if strings.Contains(m[3], "_") && cur.Health == "ok" {
				cur.Health = "degraded"
			}
		}
		if m := mdSyncRe.FindStringSubmatch(l); m != nil {
			cur.Progress = m[1] + " " + m[2]
			if m[3] != "" {
				cur.Progress += ", finish " + m[3]
			}
			if m[1] == "recovery" || m[1] == "resync" || m[1] == "reshape" {
				cur.Health = "rebuilding"
			}
		}
	}
	return out
}

// netLinks lists the physical interfaces (those with a device link; not lo, bridges, docker or WireGuard).
func netLinks(root string) []NetLink {
	out := []NetLink{}
	ents, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range ents {
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "device")); err != nil {
			continue
		}
		l := NetLink{Name: e.Name(), State: strings.TrimSpace(readFile(filepath.Join(dir, "operstate"))), MAC: strings.TrimSpace(readFile(filepath.Join(dir, "address")))}
		if l.State == "up" {
			if v, err := strconv.Atoi(strings.TrimSpace(readFile(filepath.Join(dir, "speed")))); err == nil && v > 0 {
				l.SpeedMbps = v
			}
			l.Duplex = strings.TrimSpace(readFile(filepath.Join(dir, "duplex")))
		}
		out = append(out, l)
	}
	return out
}

var (
	smartModelRe  = regexp.MustCompile(`(?m)^(?:Device Model|Model Number|Product):\s+(.+)$`)
	smartSerialRe = regexp.MustCompile(`(?m)^Serial [Nn]umber:\s+(.+)$`)
	smartHealthRe = regexp.MustCompile(`(?m)(?:SMART overall-health self-assessment test result|SMART Health Status):\s+(\S+)`)
	smartTempRe   = regexp.MustCompile(`(?m)^(?:194 Temperature_Celsius.*\s(\d+)(?:\s\(.*\))?|Temperature:\s+(\d+) Celsius)$`)
)

// smartDisks asks smartctl about every disk it finds (cached: reading SMART wakes sleeping disks and takes a moment).
func smartDisks(ctx context.Context) ([]SMARTDisk, string) {
	smartCache.Lock()
	defer smartCache.Unlock()
	if !smartCache.at.IsZero() && time.Since(smartCache.at) < 10*time.Minute {
		return smartCache.disks, smartCache.state
	}
	smartCache.at = time.Now()
	smartCache.disks, smartCache.state = []SMARTDisk{}, "unavailable"
	scan, err := run(ctx, 15*time.Second, "smartctl", "--scan")
	if err != nil {
		return smartCache.disks, smartCache.state
	}
	smartCache.state = "ok"
	for _, l := range strings.Split(scan, "\n") {
		f := strings.Fields(l)
		if len(f) == 0 || !strings.HasPrefix(f[0], "/dev/") {
			continue
		}
		smartCache.disks = append(smartCache.disks, smartDisk(ctx, f))
	}
	return smartCache.disks, smartCache.state
}

// smartDisk reads one disk of `smartctl --scan` (/dev/sdb -d scsi # …). The scan names SATA disks behind a SAS/RAID
// controller "scsi", which hides the ATA health: smartctl decides the type itself first, then tries the scanned
// type (megaraid,N, nvme, …) and SAT; a disk none of them can read stays "unknown".
func smartDisk(ctx context.Context, f []string) SMARTDisk {
	tries := [][]string{nil}
	if len(f) >= 3 && f[1] == "-d" && f[2] != "scsi" && f[2] != "auto" {
		tries = append(tries, []string{"-d", f[2]})
	}
	tries = append(tries, []string{"-d", "sat"})
	var d SMARTDisk
	for _, t := range tries {
		args := append(append([]string{"-H", "-i", "-A"}, t...), f[0])
		out, _ := run(ctx, 20*time.Second, "smartctl", args...) // exit codes carry flags; the text decides
		d = parseSmart(f[0], out)
		if d.Health != "unknown" {
			return d
		}
	}
	return d
}

func parseSmart(dev, out string) SMARTDisk {
	d := SMARTDisk{Device: dev, Health: "unknown"}
	if m := smartModelRe.FindStringSubmatch(out); m != nil {
		d.Model = strings.TrimSpace(m[1])
	}
	if m := smartSerialRe.FindStringSubmatch(out); m != nil {
		d.Serial = strings.TrimSpace(m[1])
	}
	if m := smartHealthRe.FindStringSubmatch(out); m != nil {
		d.Health = strings.TrimSpace(m[1])
		if d.Health == "OK" {
			d.Health = "PASSED"
		}
	}
	if m := smartTempRe.FindStringSubmatch(out); m != nil {
		v := m[1]
		if v == "" {
			v = m[2]
		}
		d.Temperature, _ = strconv.Atoi(v)
	}
	return d
}
