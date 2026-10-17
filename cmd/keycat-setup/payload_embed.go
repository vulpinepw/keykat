//go:build windows && payload

package main

import _ "embed"

//go:embed payload.bin
var payload []byte
