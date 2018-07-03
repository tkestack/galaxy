package ldflags

import "fmt"

var (
	GO_VERSION string
	GIT_COMMIT string
	BUILD_TIME string
)

func Footprint() string {
	return fmt.Sprintf("go-version %s, git-commit %s, build-time %s", GO_VERSION, GIT_COMMIT, BUILD_TIME)
}
