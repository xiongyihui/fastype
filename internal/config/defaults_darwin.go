//go:build darwin

package config

import _ "embed"

//go:embed default_darwin.json
var defaultJSON []byte
