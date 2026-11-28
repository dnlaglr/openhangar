package telemetry

import "testing"

func TestDisabled(t *testing.T) {
	cases := []struct {
		doNotTrack, hangar string
		want               bool
	}{
		{"", "", false},
		{"0", "false", false},
		{"1", "", true},
		{"", "true", true},
		{"yes", "0", true},
	}

	for _, tc := range cases {
		t.Setenv("DO_NOT_TRACK", tc.doNotTrack)
		t.Setenv("HANGAR_TELEMETRY_DISABLED", tc.hangar)
		if got := Disabled(); got != tc.want {
			t.Errorf("DO_NOT_TRACK=%q HANGAR_TELEMETRY_DISABLED=%q: Disabled() = %v, want %v", tc.doNotTrack, tc.hangar, got, tc.want)
		}
	}
}
