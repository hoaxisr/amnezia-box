package constant

// Version is the awgm-box build counter (e.g. "41"), set by ldflags from the
// release tag. It is not semver and says nothing about the upstream base.
var Version = "unknown"

// UpstreamVersion is the sing-box release this fork is based on. Update it
// when merging a new upstream release.
const UpstreamVersion = "1.15.0-alpha.13"
