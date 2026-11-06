// Package assets embeds the unmodified project-level Python tools and guidance.
// These are deployment assets; the native system never imports or executes them
// to implement its own workflow state transitions.
package assets

import "embed"

//go:embed resources native-core docs
var Files embed.FS
