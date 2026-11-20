//go:build !linux && !windows

package diskwatch

func diskStat(string) (uint64, uint64, string, bool) { return 0, 0, "", false }
