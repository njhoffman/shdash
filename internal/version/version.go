// Package version holds the build's version. `just build` and `just
// install` set it through -ldflags; a plain `go install module@version`
// does not, so the module version and VCS stamp from the build info stand
// in for whatever ldflags left at its default.
package version

import "runtime/debug"

const unset = "unknown"

var (
	Version = "dev"
	Commit  = unset
	Date    = unset
)

// String is "VERSION (COMMIT, DATE)".
func String() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		info = nil
	}
	v, c, d := resolve(Version, Commit, Date, info)
	return v + " (" + c + ", " + d + ")"
}

// resolve fills the values ldflags did not set from info (nil when the
// binary has no build info).
func resolve(version, commit, date string, info *debug.BuildInfo) (string, string, string) {
	if info == nil {
		return version, commit, date
	}
	if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	for _, s := range info.Settings {
		switch {
		case s.Key == "vcs.revision" && commit == unset && s.Value != "":
			commit = s.Value[:min(7, len(s.Value))]
		case s.Key == "vcs.time" && date == unset && s.Value != "":
			date = s.Value
		}
	}
	return version, commit, date
}
