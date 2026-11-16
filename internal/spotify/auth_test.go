package spotify

import "testing"

func TestCodeChallenge_RFCVector(t *testing.T) {
	// Test vector from RFC 7636, Appendix B.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	if got := codeChallenge(verifier); got != want {
		t.Errorf("codeChallenge() = %q, want %q", got, want)
	}
}

func TestNewCodeVerifier(t *testing.T) {
	v := newCodeVerifier()
	if len(v) < 43 || len(v) > 128 {
		t.Errorf("verifier length %d, want 43..128", len(v))
	}
	if newCodeVerifier() == v {
		t.Error("two verifiers were identical")
	}
}
