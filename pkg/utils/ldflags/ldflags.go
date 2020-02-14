/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package ldflags

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"
)

var (
	GO_VERSION string
	GIT_COMMIT string
	BUILD_TIME string
)

func footprint() string {
	return fmt.Sprintf("go-version %s, git-commit %s, build-time %s", GO_VERSION, GIT_COMMIT, BUILD_TIME)
}

var (
	versionFlag = flag.Bool("version", false, "Print version information and quit")
)

// PrintAndExitIfRequested will check if the -version flag was passed
// and, if so, print the version and exit.
func PrintAndExitIfRequested() {
	if *versionFlag == true {
		fmt.Printf("%s\n", footprint())
		os.Exit(0)
	}
}
