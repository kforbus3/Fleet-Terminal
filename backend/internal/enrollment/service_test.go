package enrollment

import "testing"

func TestIPPrefix24(t *testing.T) {
	cases := map[string]string{
		"10.100.0.1":   "10.100.0",
		"192.168.5.42": "192.168.5",
		"bogus":        "",
		"10.0.0":       "",
	}
	for in, want := range cases {
		if got := ipPrefix24(in); got != want {
			t.Errorf("ipPrefix24(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIsOverlayAddr(t *testing.T) {
	jump := "10.100.0.1"
	cases := map[string]bool{
		"10.100.0.10": true,  // same /24, not the jump
		"10.100.0.1":  false, // the jump host itself
		"10.99.0.10":  false, // different /24
		"":            false, // empty
	}
	for addr, want := range cases {
		if got := isOverlayAddr(addr, jump); got != want {
			t.Errorf("isOverlayAddr(%q)=%v want %v", addr, got, want)
		}
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("host-ubuntu.prod"); got != "host-ubuntu_prod" {
		t.Errorf("sanitize: got %q", got)
	}
}

func TestParseKV(t *testing.T) {
	out := "noise\nHOSTPUB=abc123=\nmore"
	if got := parseKV(out, "HOSTPUB"); got != "abc123=" {
		t.Errorf("parseKV: got %q", got)
	}
	if got := parseKV(out, "MISSING"); got != "" {
		t.Errorf("parseKV missing: got %q", got)
	}
}
