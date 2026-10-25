package execx

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestSudoArgvRunsInvokedPathWithOriginalArgs(t *testing.T) {
	argv := sudoArgv("/usr/bin/sudo", "/usr/bin/sboxkit", []string{"update"}, envFrom(nil))

	want := []string{"/usr/bin/sudo", "--", "/usr/bin/sboxkit", "update"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %q, want %q (no VAR= arguments, so restrictive sudoers without SETENV still work)", argv, want)
	}
}

func TestSudoArgvForwardsOnlySetSboxkitVariables(t *testing.T) {
	env := envFrom(map[string]string{"SBOXKIT_HOME": "/tmp/state", "SBOXKIT_LANG": "zh", "HOME": "/home/u"})

	argv := sudoArgv("/usr/bin/sudo", "/usr/bin/sboxkit", nil, env)

	// sudo 只在 -- 之前识别 VAR=值；放在 -- 之后会被当成要执行的命令。
	want := []string{"/usr/bin/sudo", "SBOXKIT_HOME=/tmp/state", "SBOXKIT_LANG=zh", "--", "/usr/bin/sboxkit"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %q, want %q", argv, want)
	}
}

func TestReexecAsRootStopsAfterOneSudoHop(t *testing.T) {
	if IsRoot() {
		t.Skip("running as root")
	}
	t.Setenv("SUDO_UID", "1000")
	t.Setenv("SUDO_COMMAND", "/usr/bin/sboxkit --resume=tools/update")

	err := ReexecAsRoot("/usr/bin/sboxkit", nil)

	if err == nil || !strings.Contains(err.Error(), "sudo") {
		t.Fatalf("ReexecAsRoot after a sudo hop = %v, want an explanatory error instead of looping", err)
	}
}

func TestReexecutedBySudoIgnoresSudoVariablesLeftByAnotherCommand(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"plain login", map[string]string{}, false},
		{"su back from a sudo root shell", map[string]string{"SUDO_UID": "1000", "SUDO_COMMAND": "/bin/bash"}, false},
		{"sudo ran sboxkit", map[string]string{"SUDO_UID": "1000", "SUDO_COMMAND": "/usr/bin/sboxkit --resume=tools/update"}, true},
		{"sudo without SUDO_COMMAND", map[string]string{"SUDO_UID": "1000"}, true},
	}
	for _, c := range cases {
		getenv := func(key string) string { return c.env[key] }
		if got := reexecutedBySudo("/usr/bin/sboxkit", getenv); got != c.want {
			t.Errorf("%s: reexecutedBySudo = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestInvokingUserHomeIgnoresSudoUserWhenNotRoot(t *testing.T) {
	if IsRoot() {
		t.Skip("running as root")
	}
	t.Setenv("SUDO_USER", "root-should-not-matter")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}

	if got := InvokingUserHome(); got != home {
		t.Fatalf("InvokingUserHome = %q, want own home %q when not running as root", got, home)
	}
}
