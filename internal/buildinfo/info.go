package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

var (
	version  = "devel"
	revision = "unknown"
)

func String() string {
	resolvedVersion := version
	resolvedRevision := revision
	modified := false

	if info, ok := debug.ReadBuildInfo(); ok {
		if resolvedVersion == "devel" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			resolvedVersion = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if resolvedRevision == "unknown" && setting.Value != "" {
					resolvedRevision = setting.Value
				}
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
	}

	if modified {
		resolvedRevision += ", modified"
	}
	return fmt.Sprintf("muamba %s (commit %s; %s; %s/%s)", resolvedVersion, resolvedRevision, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
