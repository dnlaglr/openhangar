package telemetry

import (
	"os"
	"strings"
)

func Disabled() bool {
	return truthy(os.Getenv("DO_NOT_TRACK")) || truthy(os.Getenv("HANGAR_TELEMETRY_DISABLED"))
}

func truthy(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))

	return v != "" && v != "0" && v != "false"
}
