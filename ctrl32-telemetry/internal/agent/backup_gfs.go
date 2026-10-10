package agent

import (
	"sort"
	"time"
)

// gfsKeep decides which backups to keep (grandfather–father–son, URS K-01): the newest `daily` backups, plus the
// newest backup of each of the last `weekly` ISO weeks and of the last `monthly` calendar months that have one. The
// newest backup is always kept. The result is indexed like times.
func gfsKeep(times []time.Time, daily, weekly, monthly int) []bool {
	keep := make([]bool, len(times))
	idx := make([]int, len(times))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return times[idx[a]].After(times[idx[b]]) })
	for n, i := range idx {
		if n < daily || n == 0 {
			keep[i] = true
		}
	}
	tier := func(limit int, period func(time.Time) [2]int) {
		seen := map[[2]int]bool{}
		for _, i := range idx {
			if len(seen) >= limit {
				return
			}
			p := period(times[i])
			if !seen[p] {
				seen[p] = true
				keep[i] = true
			}
		}
	}
	tier(weekly, func(t time.Time) [2]int { y, w := t.ISOWeek(); return [2]int{y, w} })
	tier(monthly, func(t time.Time) [2]int { return [2]int{t.Year(), int(t.Month())} })
	return keep
}

// backupTime reads the time from a backup name (ctrl32-YYYYMMDD-HHMMSS…) in the server's zone.
func backupTime(name string) (time.Time, bool) {
	if len(name) < len("ctrl32-20060102-150405") {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("20060102-150405", name[7:22], time.Local)
	return t, err == nil
}
