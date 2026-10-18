package main

import "testing"

func TestValidWebPassword(t *testing.T) {
	for _, password := range []string{"", "change-me", "your-secure-password"} {
		if validWebPassword(password) {
			t.Errorf("validWebPassword(%q) = true, want false", password)
		}
	}
	if !validWebPassword("0123456789abcdef0123456789abcdef") {
		t.Error("validWebPassword(random password) = false, want true")
	}
}
