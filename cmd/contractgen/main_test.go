package main

import "testing"

func TestSwiftIdentifierEscapesLanguageKeywords(t *testing.T) {
	if got := swiftIdentifier("switch"); got != "`switch`" {
		t.Fatalf("swiftIdentifier(switch) = %q", got)
	}
	if got := swiftIdentifier("active_lease"); got != "activeLease" {
		t.Fatalf("swiftIdentifier(active_lease) = %q", got)
	}
}
