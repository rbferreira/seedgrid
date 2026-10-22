package transfer

import (
	"os"
	"syscall"
)

// ownerOf returns the owner of path; ok=false when it cannot be read.
func ownerOf(path string) (uid, gid int, ok bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, 0, false
	}
	st, isStat := fi.Sys().(*syscall.Stat_t)
	if !isStat {
		return 0, 0, false
	}
	return int(st.Uid), int(st.Gid), true
}

// chownBestEffort gives path the uid and gid, when the Agent can (it runs as root on a Node).
// A failure is not an error: on a Node SeedGrid installs everything is root's already, and a
// file the Engine cannot write is what the Engine's own error then reports — failing the
// migration here would lose a copy that may well be readable.
func chownBestEffort(path string, uid, gid int) {
	if os.Geteuid() != 0 {
		return
	}
	_ = os.Lchown(path, uid, gid)
}
