package paths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fakeProbe(arg0 string, env map[string]string, files map[string]bool, links map[string]string) userModeProbe {
	return userModeProbe{
		env:      func(key string) string { return env[key] },
		arg0:     arg0,
		lookPath: func(name string) (string, error) { return "/usr/local/bin/" + name, nil },
		exists:   func(path string) bool { return files[path] },
		readlink: func(path string) (string, error) {
			if target, ok := links[path]; ok {
				return target, nil
			}
			return "", errors.New("not a symlink")
		},
	}
}

func TestBundleRootFoundNextToExecutable(t *testing.T) {
	probe := fakeProbe("/opt/kit/sboxkit", nil, map[string]bool{"/opt/kit/" + UserModeMarker: true}, nil)

	if got := detectUserModeRoot(probe); got != "/opt/kit" {
		t.Fatalf("userMode root = %q, want /opt/kit", got)
	}
}

func TestBundleRootFollowsEntrySymlinkIntoArchive(t *testing.T) {
	probe := fakeProbe("sboxkit", nil,
		map[string]bool{"/home/u/kit/" + UserModeMarker: true},
		map[string]string{"/usr/local/bin/sboxkit": "/home/u/kit/sboxkit"},
	)

	if got := detectUserModeRoot(probe); got != "/home/u/kit" {
		t.Fatalf("userMode root = %q, want /home/u/kit", got)
	}
}

func TestBundleRootSurvivesSelfUpdateLink(t *testing.T) {
	// 自更新后包内 sboxkit 指向 .sboxkit/sboxkit-versions/current：第一跳所在目录
	// 就是包目录，标记必须在追踪到版本目录之前被找到。
	probe := fakeProbe("/home/u/kit/sboxkit", nil,
		map[string]bool{"/home/u/kit/" + UserModeMarker: true},
		map[string]string{"/home/u/kit/sboxkit": ".sboxkit/sboxkit-versions/current"},
	)

	if got := detectUserModeRoot(probe); got != "/home/u/kit" {
		t.Fatalf("userMode root = %q, want /home/u/kit", got)
	}
}

func TestUserModeEnvZeroForcesSystemMode(t *testing.T) {
	probe := fakeProbe("/opt/kit/sboxkit", map[string]string{userModeEnv: "0"},
		map[string]bool{"/opt/kit/" + UserModeMarker: true}, nil)

	if got := detectUserModeRoot(probe); got != "" {
		t.Fatalf("userMode root = %q, want system mode", got)
	}
}

func TestUserModeEnvOneWithoutMarkerUsesExecutableDir(t *testing.T) {
	probe := fakeProbe("/srv/tools/sboxkit", map[string]string{userModeEnv: "1"}, nil, nil)

	if got := detectUserModeRoot(probe); got != "/srv/tools" {
		t.Fatalf("userMode root = %q, want /srv/tools", got)
	}
}

func TestNoMarkerMeansSystemMode(t *testing.T) {
	probe := fakeProbe("/usr/bin/sboxkit", nil, nil, nil)

	if got := detectUserModeRoot(probe); got != "" {
		t.Fatalf("userMode root = %q, want system mode", got)
	}
}

func TestSymlinkLoopTerminates(t *testing.T) {
	probe := fakeProbe("/a/sboxkit", nil, nil, map[string]string{
		"/a/sboxkit": "/b/sboxkit",
		"/b/sboxkit": "/a/sboxkit",
	})

	if got := detectUserModeRoot(probe); got != "" {
		t.Fatalf("userMode root = %q, want system mode", got)
	}
}

func TestFromUserModeRootKeepsRuntimeInsideState(t *testing.T) {
	p := FromUserModeRoot("/home/u/kit/.sboxkit", "/home/u/kit")

	if !p.UserMode || p.Runtime != "/home/u/kit/.sboxkit/runtime" {
		t.Fatalf("userMode paths = %+v", p)
	}
	if p.SeedBinDir() != "/home/u/kit/core" || p.SeedRulesetDir() != "/home/u/kit/ruleset" {
		t.Fatalf("seed dirs = %q %q", p.SeedBinDir(), p.SeedRulesetDir())
	}
}

func TestSystemPathsHaveNoUserModeSeeds(t *testing.T) {
	p := FromRoot("/var/lib/sboxkit")

	if p.UserMode || p.Runtime != RuntimeDir || p.SeedBinDir() != "" {
		t.Fatalf("system paths = %+v", p)
	}
}

func TestDetectDefaultsUserModeStateToArchive(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "sboxkit")
	if err := os.WriteFile(filepath.Join(dir, UserModeMarker), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SBOXKIT_HOME", "")
	t.Setenv(userModeEnv, "")
	oldArgs := os.Args
	os.Args = []string{exe}
	t.Cleanup(func() { os.Args = oldArgs })

	p := Detect()

	if !p.UserMode || p.State != filepath.Join(dir, UserModeStateDir) {
		t.Fatalf("Detect() = %+v", p)
	}
}
