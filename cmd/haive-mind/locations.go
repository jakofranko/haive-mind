package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// appDirName is the directory name haive-mind uses under each base directory.
const appDirName = "haive-mind"

// userEnv is what decides where a user's files go by default: the environment
// variables, the operating system and the home directory ("" when unknown).
type userEnv struct {
	getenv func(string) string
	goos   string
	home   string
}

// defaultDirs returns where haive-mind keeps its data (transcripts and the SSH
// host key) and its logs when the matching flags are not given. An absolute
// XDG_DATA_HOME / XDG_STATE_HOME wins on every platform; otherwise the
// platform's own convention applies. A directory that would need an unknown
// home directory is "": there is no default, and never one relative to the
// working directory.
func defaultDirs(env userEnv) (data, logs string) {
	dataBase, logsBase := platformBases(env)
	return underApp(xdgBase(env, "XDG_DATA_HOME", dataBase)), underApp(xdgBase(env, "XDG_STATE_HOME", logsBase))
}

// underApp is the haive-mind directory under base, or "" when base is unknown.
func underApp(base string) string {
	if !filepath.IsAbs(base) {
		return ""
	}
	return filepath.Join(base, appDirName)
}

// underHome is the path of elem under the home directory, or "" when the home
// directory is not known.
func (e userEnv) underHome(elem ...string) string {
	if e.home == "" {
		return ""
	}
	return filepath.Join(append([]string{e.home}, elem...)...)
}

// platformBases are the base directories for data and logs when no XDG
// variable decides. Either is empty when it would need a home directory that
// is not known.
func platformBases(env userEnv) (data, logs string) {
	switch env.goos {
	case "windows":
		local := env.getenv("LOCALAPPDATA")
		if !filepath.IsAbs(local) {
			local = env.underHome("AppData", "Local")
		}
		return local, local
	case "darwin":
		return env.underHome("Library", "Application Support"), env.underHome("Library", "Logs")
	}
	return env.underHome(".local", "share"), env.underHome(".local", "state")
}

// xdgBase is the value of the XDG variable name, or fallback when it is unset,
// empty or not an absolute path (the spec says such values are invalid and
// must be ignored).
func xdgBase(env userEnv, name, fallback string) string {
	if v := env.getenv(name); filepath.IsAbs(v) {
		return v
	}
	return fallback
}

// locations are the effective paths the server uses.
type locations struct {
	HostKey string
	DataDir string
	LogFile string
}

// resolveLocations turns the three path flags (empty when not given) into the
// paths the server uses, creating the default directories it ends up using.
// Each flag replaces only its own item, and a path the user gave is used as is.
func resolveLocations(hostKey, dataDir, logFile string, env userEnv) (locations, error) {
	loc := locations{HostKey: hostKey, DataDir: dataDir, LogFile: logFile}
	needData := hostKey == "" || dataDir == ""
	needLogs := logFile == ""
	if !needData && !needLogs {
		return loc, nil
	}
	defData, defLogs := defaultDirs(env)
	if needData {
		var flags []string
		if dataDir == "" {
			flags = append(flags, "--data-dir")
		}
		if hostKey == "" {
			flags = append(flags, "--host-key")
		}
		alternatives := strings.Join(flags, " and ")
		if defData == "" {
			return locations{}, noDefault("data directory", "XDG_DATA_HOME", alternatives)
		}
		if err := makeDir(defData, alternatives); err != nil {
			return locations{}, err
		}
		if hostKey == "" {
			loc.HostKey = filepath.Join(defData, "id_ed25519")
		}
		if dataDir == "" {
			loc.DataDir = defData
		}
	}
	if needLogs {
		if defLogs == "" {
			return locations{}, noDefault("log directory", "XDG_STATE_HOME", "--log-file")
		}
		if err := makeDir(defLogs, "--log-file"); err != nil {
			return locations{}, err
		}
		loc.LogFile = filepath.Join(defLogs, "haive-mind.log")
	}
	return loc, nil
}

// noDefault is the error for a default directory that cannot be placed because
// the home directory is unknown.
func noDefault(what, variable, flags string) error {
	return fmt.Errorf("cannot find a home directory to place the default %s; set %s to an absolute path or use %s", what, variable, flags)
}

// makeDir creates dir (private to the user) and, if it cannot, says which flags
// (alternatives) put the files somewhere else.
func makeDir(dir, alternatives string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create %s: %w (use %s to choose another location)", dir, err, alternatives)
	}
	return nil
}
