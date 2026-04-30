package main

import "testing"

func TestResolvePrintBackend(t *testing.T) {
	if got := resolvePrintBackend("", "godex"); got != printBackendGoDEX {
		t.Fatalf("default backend mismatch: %q", got)
	}
	if got := resolvePrintBackend("zpl", "godex"); got != printBackendZebra {
		t.Fatalf("request printer should override default: %q", got)
	}
	if got := resolvePrintBackend("g500", "zebra"); got != printBackendGoDEX {
		t.Fatalf("godex alias mismatch: %q", got)
	}
	if got := resolvePrintBackend("unknown", "godex"); got != printBackendGoDEX {
		t.Fatalf("unknown request printer should fall back to default: %q", got)
	}
}
