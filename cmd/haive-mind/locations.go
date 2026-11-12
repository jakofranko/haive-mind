package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// appDir is the directory name haive-mind uses under each base directory.
const appDir = "haive-mind"

// defaultDirs returns where haive-mind keeps its data (transcripts and the SSH
// host key) and its logs when the matching flags are not given. An absolute
// XDG_DATA_HOME / XDG_STATE_HOME wins on every platform; otherwise the
// platform's own convention applies. A directory that would need an unknown
// home directory is "": there is no default, and never one relative to the
// working directory.
func defaultDirs(getenv func(string) string, goos, home string) (data, logs string) {
	dataBase, logsBase := platformBases(getenv, goos, home)
	return underApp(xdgBase(getenv, "XDG_DATA_HOME", dataBase)), underApp(xdgBase(getenv, "XDG_STATE_HOME", logsBase))
}

// underApp is the haive-mind directory under base, or "" when base is unknown.
func underApp(base string) string {
	if !filepath.IsAbs(base) {
		return ""
	}
	return filepath.Join(base, appDir)
}

// platformBases are the base directories for data and logs when no XDG
// variable decides. Either is empty when it would need a home directory that
// is not known.
func platformBases(getenv func(string) string, goos, home string) (data, logs string) {
	under := func(elem ...string) string {
		if home == "" {
			return ""
		}
		return filepath.Join(append([]string{home}, elem...)...)
	}
	switch goos {
	case "windows":
		if local := getenv("LOCALAPPDATA"); filepath.IsAbs(local) {
			return local, local
		}
		return under("AppData", "Local"), under("AppData", "Local")
	case "darwin":
		return under("Library", "Application Support"), under("Library", "Logs")
	}
	return under(".local", "share"), under(".local", "state")
}

// xdgBase is the value of the XDG variable name, or fallback when it is unset,
// empty or not an absolute path (the spec says such values are invalid and
// must be ignored).
func xdgBase(getenv func(string) string, name, fallback string) string {
	if v := getenv(name); filepath.IsAbs(v) {
		return v
	}
	return fallback
}

// locations are the effective paths the server uses.
type locations struct {
	HostKey string
	DataDir string
	LogFile string
	// LogIsDefault is true when LogFile was not given with --log-file.
	LogIsDefault bool
}

// resolveLocations turns the three path flags (empty when not given) into the
// paths the server uses, creating the default directories it ends up using.
// Each flag replaces only its own item, and a path the user gave is used as is.
func resolveLocations(hostKey, dataDir, logFile string, getenv func(string) string, goos, home string) (locations, error) {
	loc := locations{HostKey: hostKey, DataDir: dataDir, LogFile: logFile}
	if hostKey != "" && dataDir != "" && logFile != "" {
		return loc, nil
	}
	defData, defLogs := defaultDirs(getenv, goos, home)
	if loc.HostKey == "" || loc.DataDir == "" {
		var flags []string
		if loc.DataDir == "" {
			flags = append(flags, "--data-dir")
		}
		if loc.HostKey == "" {
			flags = append(flags, "--host-key")
		}
		if defData == "" {
			return locations{}, noDefault("data directory", "XDG_DATA_HOME", strings.Join(flags, " and "))
		}
		if err := makeDir(defData, strings.Join(flags, " and ")); err != nil {
			return locations{}, err
		}
	}
	if loc.HostKey == "" {
		loc.HostKey = filepath.Join(defData, "id_ed25519")
	}
	if loc.DataDir == "" {
		loc.DataDir = defData
	}
	if loc.LogFile == "" {
		if defLogs == "" {
			return locations{}, noDefault("log directory", "XDG_STATE_HOME", "--log-file")
		}
		if err := makeDir(defLogs, "--log-file"); err != nil {
			return locations{}, err
		}
		loc.LogFile = filepath.Join(defLogs, "haive-mind.log")
		loc.LogIsDefault = true
	}
	return loc, nil
}

// noDefault is the error for a default directory that cannot be placed because
// the home directory is unknown.
func noDefault(what, variable, flags string) error {
	return fmt.Errorf("cannot find a home directory to place the default %s; set %s to an absolute path or use %s", what, variable, flags)
}

// makeDir creates dir (private to the user) and, if it cannot, says which flag
// puts the files somewhere else.
func makeDir(dir, flag string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create %s: %w (use %s to choose another location)", dir, err, flag)
	}
	return nil
}
