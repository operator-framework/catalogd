package version

import (
	"fmt"
	"runtime"

	genericversion "k8s.io/apimachinery/pkg/version"
)

var (
	gitVersion   = "v0.0.0-master+$Format:%h$"
	gitCommit    = "$Format:%H$"          // sha1 from git, output of $(git rev-parse HEAD)
	gitTreeState = ""                     // state of git tree, either "clean" or "dirty"
	buildDate    = "1970-01-01T00:00:00Z" // build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
)

// ControllerVersion returns a version string for the controller
func ControllerVersion() string {
	return gitVersion
}

// ApiserverVersion returns a version.Info object for the apiserver
func ApiserverVersion() genericversion.Info {
	return genericversion.Info{
		GitVersion:   gitVersion,
		GitCommit:    gitCommit,
		GitTreeState: gitTreeState,
		BuildDate:    buildDate,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
