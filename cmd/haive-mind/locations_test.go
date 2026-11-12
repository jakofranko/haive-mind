package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// An absolute XDG_DATA_HOME / XDG_STATE_HOME decides where the data and the
// logs go, under a haive-mind directory.
func TestDefaultDirs_HonoursAbsoluteXDGVariables(t *testing.T) {
	data := filepath.Join(t.TempDir(), "share")
	state := filepath.Join(t.TempDir(), "state")
	env := envOf(map[string]string{"XDG_DATA_HOME": data, "XDG_STATE_HOME": state})

	gotData, gotLogs, err := defaultDirs(env, "linux", "/home/ann")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(data, "haive-mind"); gotData != want {
		t.Errorf("data dir = %q, want %q", gotData, want)
	}
	if want := filepath.Join(state, "haive-mind"); gotLogs != want {
		t.Errorf("log dir = %q, want %q", gotLogs, want)
	}
}

// An unset, empty or relative XDG variable is ignored, as the XDG spec says,
// and the directories under the home directory are used.
func TestDefaultDirs_IgnoresUnusableXDGVariablesOnLinux(t *testing.T) {
	home := filepath.Join(t.TempDir(), "ann")
	for name, vars := range map[string]map[string]string{
		"unset":    {},
		"empty":    {"XDG_DATA_HOME": "", "XDG_STATE_HOME": ""},
		"relative": {"XDG_DATA_HOME": "share", "XDG_STATE_HOME": "./state"},
	} {
		t.Run(name, func(t *testing.T) {
			gotData, gotLogs, err := defaultDirs(envOf(vars), "linux", home)
			if err != nil {
				t.Fatal(err)
			}
			if want := filepath.Join(home, ".local", "share", "haive-mind"); gotData != want {
				t.Errorf("data dir = %q, want %q", gotData, want)
			}
			if want := filepath.Join(home, ".local", "state", "haive-mind"); gotLogs != want {
				t.Errorf("log dir = %q, want %q", gotLogs, want)
			}
		})
	}
}

// On Windows, without XDG variables, data and logs share a haive-mind
// directory under %LocalAppData%.
func TestDefaultDirs_WindowsUsesLocalAppData(t *testing.T) {
	local := filepath.Join(t.TempDir(), "Local")
	env := envOf(map[string]string{"LOCALAPPDATA": local})

	gotData, gotLogs, err := defaultDirs(env, "windows", filepath.Join(t.TempDir(), "ann"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(local, "haive-mind")
	if gotData != want || gotLogs != want {
		t.Errorf("data dir = %q, log dir = %q, want both %q", gotData, gotLogs, want)
	}
}

// On macOS, without XDG variables, data goes under Application Support and
// logs under Logs.
func TestDefaultDirs_MacOSUsesLibrary(t *testing.T) {
	home := filepath.Join(t.TempDir(), "ann")

	gotData, gotLogs, err := defaultDirs(envOf(nil), "darwin", home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "Library", "Application Support", "haive-mind"); gotData != want {
		t.Errorf("data dir = %q, want %q", gotData, want)
	}
	if want := filepath.Join(home, "Library", "Logs", "haive-mind"); gotLogs != want {
		t.Errorf("log dir = %q, want %q", gotLogs, want)
	}
}

// With nowhere to anchor a default (no usable XDG variable and no home
// directory) the answer is an error, never a path relative to the working
// directory. An XDG variable alone is enough when it is set.
func TestDefaultDirs_NoHomeIsAnErrorUnlessXDGIsSet(t *testing.T) {
	if _, _, err := defaultDirs(envOf(nil), "linux", ""); err == nil {
		t.Error("no XDG variables and no home: want an error")
	}

	data, state := t.TempDir(), t.TempDir()
	env := envOf(map[string]string{"XDG_DATA_HOME": data, "XDG_STATE_HOME": state})
	if _, _, err := defaultDirs(env, "linux", ""); err != nil {
		t.Errorf("XDG variables set, no home: %v", err)
	}
}

func xdgEnv(t *testing.T) (env func(string) string, data, state string) {
	t.Helper()
	data, state = filepath.Join(t.TempDir(), "share"), filepath.Join(t.TempDir(), "state")
	return envOf(map[string]string{"XDG_DATA_HOME": data, "XDG_STATE_HOME": state}), data, state
}

// With no path flags, the host key and transcripts go to the data directory,
// the log file to the log directory, and the directories exist afterwards.
func TestResolveLocations_DefaultsWhenNoFlagsAreGiven(t *testing.T) {
	env, data, state := xdgEnv(t)

	got, err := resolveLocations("", "", "", env, "linux", "/home/ann")
	if err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(data, "haive-mind")
	want := locations{
		HostKey:      filepath.Join(dataDir, "id_ed25519"),
		DataDir:      dataDir,
		LogFile:      filepath.Join(state, "haive-mind", "haive-mind.log"),
		LogIsDefault: true,
	}
	if got != want {
		t.Errorf("locations = %+v, want %+v", got, want)
	}
	for _, dir := range []string{dataDir, filepath.Join(state, "haive-mind")} {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			t.Errorf("%s was not created: %v", dir, err)
		}
	}
}

// Each flag replaces only its own item; the rest keep their defaults. A path
// the user gave is used as is, and a default directory nobody needs is not
// created.
func TestResolveLocations_EachFlagOverridesOnlyItsOwnItem(t *testing.T) {
	env, data, state := xdgEnv(t)
	dataDir := filepath.Join(data, "haive-mind")
	logDir := filepath.Join(state, "haive-mind")
	mine := t.TempDir()

	got, err := resolveLocations("", filepath.Join(mine, "transcripts"), "", env, "linux", "/home/ann")
	if err != nil {
		t.Fatal(err)
	}
	if got.DataDir != filepath.Join(mine, "transcripts") {
		t.Errorf("data dir = %q, want the flag's", got.DataDir)
	}
	if got.HostKey != filepath.Join(dataDir, "id_ed25519") || got.LogFile != filepath.Join(logDir, "haive-mind.log") || !got.LogIsDefault {
		t.Errorf("host key and log file should stay default: %+v", got)
	}

	all, err := resolveLocations(filepath.Join(mine, "key"), filepath.Join(mine, "d"), filepath.Join(mine, "x.log"), env, "linux", "/home/ann")
	if err != nil {
		t.Fatal(err)
	}
	if all.HostKey != filepath.Join(mine, "key") || all.DataDir != filepath.Join(mine, "d") || all.LogFile != filepath.Join(mine, "x.log") || all.LogIsDefault {
		t.Errorf("all flags given, want them used as is: %+v", all)
	}
}

func TestResolveLocations_CreatesNoDefaultDirectoryItDoesNotNeed(t *testing.T) {
	env, data, state := xdgEnv(t)
	mine := t.TempDir()

	if _, err := resolveLocations(filepath.Join(mine, "key"), filepath.Join(mine, "d"), filepath.Join(mine, "x.log"), env, "linux", "/home/ann"); err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{data, state} {
		if _, err := os.Stat(base); !os.IsNotExist(err) {
			t.Errorf("%s exists although every path was given (stat error: %v)", base, err)
		}
	}
}

// A default directory that cannot be created is a startup error that names the
// directory and the flag that picks another place. It never falls back to the
// working directory.
func TestResolveLocations_UncreatableDefaultNamesTheDirectoryAndFlag(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(file, "share") // a directory under a regular file
	ok := t.TempDir()

	_, err := resolveLocations("", "", "", envOf(map[string]string{"XDG_DATA_HOME": blocked, "XDG_STATE_HOME": ok}), "linux", "/home/ann")
	if err == nil {
		t.Fatal("want an error for an uncreatable data directory")
	}
	for _, want := range []string{filepath.Join(blocked, "haive-mind"), "--data-dir", "--host-key"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	_, err = resolveLocations("", "x", "", envOf(map[string]string{"XDG_DATA_HOME": ok, "XDG_STATE_HOME": blocked}), "linux", "/home/ann")
	if err == nil {
		t.Fatal("want an error for an uncreatable log directory")
	}
	for _, want := range []string{filepath.Join(blocked, "haive-mind"), "--log-file"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// The default directories are private to the user.
func TestResolveLocations_CreatesDefaultDirectoriesPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits")
	}
	env, data, state := xdgEnv(t)

	if _, err := resolveLocations("", "", "", env, "linux", "/home/ann"); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(data, "haive-mind"), filepath.Join(state, "haive-mind")} {
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o700 {
			t.Errorf("%s mode = %o, want 700", dir, fi.Mode().Perm())
		}
	}
}
