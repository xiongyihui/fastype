//go:build windows

package config

import _ "embed"

//go:embed default.json
var defaultJSON []byte
