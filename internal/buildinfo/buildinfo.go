// Package buildinfo carries the CommitProof source identity, set at link
// time by the Makefile and build/Dockerfile.runner:
//
//	-X github.com/CesarManzoCode/commitproof/internal/buildinfo.Commit=<sha>
//	-X github.com/CesarManzoCode/commitproof/internal/buildinfo.Dirty=<true|false>
//
// Dirty means uncommitted changes outside evidence/ and images.lock.json
// (those two are outputs, not inputs, of the code).
package buildinfo

// Link-time values; "unknown" when built without the Makefile.
var (
	Commit = "unknown"
	Dirty  = "unknown"
)
