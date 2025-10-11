package ui

import "testing"

// TestGenerateConfirmationCodes tests the GenConfirmationCode function
func TestGenerateConfirmationCodes(t *testing.T) {

	for i := range 100 {
		code := GenConfirmationCode()

		if len(code) < 5 || len(code) > 8 {
			t.Errorf("Generated code length out of bounds: got %d, want between 5 and 8", len(code))
		}
		for _, c := range code {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
				t.Errorf("Generated code contains invalid character: %q", c)
			}
		}

		if i%10 == 0 {
			t.Logf("Sample generated code: %s", code)
		}
	}
}
