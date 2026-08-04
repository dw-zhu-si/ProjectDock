package webassets

import "embed"

// Files contains the complete dependency-free ProjectDock interface.
//
//go:embed index.html styles.css js/*.js
var Files embed.FS
