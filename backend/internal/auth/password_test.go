package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	pw := "Sup3r-Secret-Pass!"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == pw {
		t.Fatal("hash must not equal plaintext")
	}
	ok, err := VerifyPassword(pw, hash)
	if err != nil || !ok {
		t.Fatalf("verify correct: ok=%v err=%v", ok, err)
	}
	bad, err := VerifyPassword("wrong-password", hash)
	if err != nil {
		t.Fatalf("verify wrong err: %v", err)
	}
	if bad {
		t.Fatal("verify must fail for wrong password")
	}
}

func TestHashUniqueSalt(t *testing.T) {
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	if h1 == h2 {
		t.Fatal("identical passwords must produce different hashes (random salt)")
	}
}

func TestPasswordPolicy(t *testing.T) {
	p := DefaultPolicy
	cases := map[string]bool{
		"Sup3r-Secret-Pass!": true,  // meets all
		"short1!A":           false, // too short
		"alllowercase123!":   false, // no upper
		"ALLUPPERCASE123!":   false, // no lower
		"NoDigitsHere!!":     false, // no digit
		"NoSymbol1234AB":     false, // no symbol
	}
	for pw, want := range cases {
		err := p.Validate(pw)
		if (err == nil) != want {
			t.Errorf("Validate(%q): got err=%v want valid=%v", pw, err, want)
		}
	}
}
