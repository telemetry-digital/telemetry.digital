//go:build windows

package replication

// chownLike: on Windows the file inherits the ACL of the configuration directory.
func chownLike(path, ref string) {}
