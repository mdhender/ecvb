// Copyright (c) 2026 Michael D Henderson. All rights reserved.
package ecvb

import "github.com/maloquacious/semver"

var (
	version = semver.Version{
		Major:      0,
		Minor:      1,
		Patch:      0,
		PreRelease: "beta",

		// Automatically populate build metadata with commit info
		Build: semver.Commit(), // Uses Git commit hash from build info
	}
)

func Version() semver.Version {
	return version
}
