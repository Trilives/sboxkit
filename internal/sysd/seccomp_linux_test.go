//go:build linux && (amd64 || arm64)

package sysd

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// socketFamilies maps the RestrictAddressFamilies names the relay units use.
var socketFamilies = map[string]uint32{
	"AF_UNIX":    unix.AF_UNIX,
	"AF_INET":    unix.AF_INET,
	"AF_INET6":   unix.AF_INET6,
	"AF_NETLINK": unix.AF_NETLINK,
	"AF_PACKET":  unix.AF_PACKET,
}

var seccompAuditArch = map[string]uint32{
	"amd64": unix.AUDIT_ARCH_X86_64,
	"arm64": unix.AUDIT_ARCH_AARCH64,
}

// seccomp_data offsets: nr at 0, arch at 4, args[0] (low word, little endian) at 16.
const (
	seccompDataNr    = 0
	seccompDataArch  = 4
	seccompDataArgs0 = 16
)

// parseAddressFamilies resolves an allow-list the same way systemd reads it.
// Deny-lists ("~AF_…") are not used by sboxkit and are rejected here.
func parseAddressFamilies(value string) ([]uint32, error) {
	var families []uint32
	for _, name := range strings.Fields(value) {
		family, ok := socketFamilies[name]
		if !ok {
			return nil, fmt.Errorf("unsupported address family %q", name)
		}
		families = append(families, family)
	}
	if len(families) == 0 {
		return nil, fmt.Errorf("empty address family allow-list %q", value)
	}
	return families, nil
}

// socketFamilyFilter mirrors systemd's RestrictAddressFamilies allow-list:
// socket(2) with any other family fails with EAFNOSUPPORT, every other
// syscall is untouched.
func socketFamilyFilter(arch uint32, families []uint32) []unix.SockFilter {
	load := func(offset uint32) unix.SockFilter {
		return unix.SockFilter{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: offset}
	}
	jumpIfEqual := func(value uint32, skipWhenTrue uint8) unix.SockFilter {
		return unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: value, Jt: skipWhenTrue}
	}
	ret := func(action uint32) unix.SockFilter {
		return unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: action}
	}
	filter := []unix.SockFilter{
		load(seccompDataArch), jumpIfEqual(arch, 1), ret(unix.SECCOMP_RET_ALLOW),
		load(seccompDataNr), jumpIfEqual(unix.SYS_SOCKET, 1), ret(unix.SECCOMP_RET_ALLOW),
		load(seccompDataArgs0),
	}
	for i, family := range families {
		// Skip the remaining comparisons and the deny return to reach ALLOW.
		filter = append(filter, jumpIfEqual(family, uint8(len(families)-i)))
	}
	return append(filter,
		ret(unix.SECCOMP_RET_ERRNO|uint32(unix.EAFNOSUPPORT)),
		ret(unix.SECCOMP_RET_ALLOW),
	)
}

// restrictSocketFamilies installs the filter on every thread of the current
// (helper) process. It is irreversible, so only call it in a child process.
func restrictSocketFamilies(families []uint32) error {
	arch, ok := seccompAuditArch[runtime.GOARCH]
	if !ok {
		return fmt.Errorf("no seccomp audit arch for %s", runtime.GOARCH)
	}
	filter := socketFamilyFilter(arch, families)
	runtime.LockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("set no_new_privs: %w", err)
	}
	prog := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	failedThread, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER,
		unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&prog)))
	if errno != 0 {
		return fmt.Errorf("install seccomp filter: %w", errno)
	}
	if failedThread != 0 {
		return fmt.Errorf("install seccomp filter: thread %d could not be synchronized", failedThread)
	}
	return nil
}
