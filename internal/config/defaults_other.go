//go:build !windows && !darwin

package config

import _ "embed"

//go:embed default.json
var defaultJSON []byte
