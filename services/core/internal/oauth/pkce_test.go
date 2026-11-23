package oauth

import "testing"

func TestChallengeMatchesTheRFC7636Example(t *testing.T) {
	t.Parallel()
	got := Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"; got != want {
		t.Fatalf("challenge = %s, want %s", got, want)
	}
}

func TestVerifiersAreLongRandomAndUnique(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for range 200 {
		v, err := NewVerifier()
		if err != nil {
			t.Fatal(err)
		}
		if len(v) < 43 || len(v) > 128 {
			t.Fatalf("verifier length %d is outside 43..128", len(v))
		}
		if seen[v] {
			t.Fatal("a verifier repeated")
		}
		seen[v] = true
	}
}
