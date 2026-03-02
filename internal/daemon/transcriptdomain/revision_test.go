package transcriptdomain

import "testing"

func TestParseRevisionToken(t *testing.T) {
	token, err := ParseRevisionToken(" 42 ")
	if err != nil {
		t.Fatalf("ParseRevisionToken: %v", err)
	}
	if token.String() != "42" {
		t.Fatalf("unexpected token: %q", token.String())
	}
}

func TestParseRevisionTokenRejectsEmpty(t *testing.T) {
	if _, err := ParseRevisionToken("   "); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestParseRevisionTokenRejectsInternalWhitespace(t *testing.T) {
	if _, err := ParseRevisionToken("rev 1"); err == nil {
		t.Fatal("expected whitespace validation error")
	}
}

func TestMustParseRevisionTokenPanicsOnInvalidInput(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic from MustParseRevisionToken")
		}
	}()
	_ = MustParseRevisionToken(" ")
}

func TestRevisionTokenIsZero(t *testing.T) {
	if !RevisionToken("").IsZero() {
		t.Fatal("expected empty token to be zero")
	}
	if MustParseRevisionToken("1").IsZero() {
		t.Fatal("expected non-empty token to be non-zero")
	}
}

func TestCompareRevisionTokensNumeric(t *testing.T) {
	cmp, err := CompareRevisionTokens(MustParseRevisionToken("2"), MustParseRevisionToken("10"))
	if err != nil {
		t.Fatalf("CompareRevisionTokens: %v", err)
	}
	if cmp >= 0 {
		t.Fatalf("expected 2 < 10, got cmp=%d", cmp)
	}
}

func TestCompareRevisionTokensLexicographicFallback(t *testing.T) {
	cmp, err := CompareRevisionTokens(MustParseRevisionToken("r-2"), MustParseRevisionToken("r-10"))
	if err != nil {
		t.Fatalf("CompareRevisionTokens: %v", err)
	}
	if cmp <= 0 {
		t.Fatalf("expected r-2 > r-10 lexicographically, got cmp=%d", cmp)
	}
}

func TestCompareRevisionTokensRejectsInvalidLeft(t *testing.T) {
	_, err := CompareRevisionTokens(RevisionToken(" "), MustParseRevisionToken("1"))
	if err == nil {
		t.Fatal("expected invalid left token error")
	}
}

func TestCompareRevisionTokensRejectsInvalidRight(t *testing.T) {
	_, err := CompareRevisionTokens(MustParseRevisionToken("1"), RevisionToken(" "))
	if err == nil {
		t.Fatal("expected invalid right token error")
	}
}

func TestIsRevisionNewer(t *testing.T) {
	newer, err := IsRevisionNewer(MustParseRevisionToken("5"), MustParseRevisionToken("4"))
	if err != nil {
		t.Fatalf("IsRevisionNewer: %v", err)
	}
	if !newer {
		t.Fatal("expected candidate to be newer")
	}

	newer, err = IsRevisionNewer(MustParseRevisionToken("4"), MustParseRevisionToken("5"))
	if err != nil {
		t.Fatalf("IsRevisionNewer: %v", err)
	}
	if newer {
		t.Fatal("expected candidate to be older")
	}
}

func TestIsRevisionNewerRejectsInvalidToken(t *testing.T) {
	_, err := IsRevisionNewer(RevisionToken(" "), MustParseRevisionToken("1"))
	if err == nil {
		t.Fatal("expected IsRevisionNewer invalid-token error")
	}
}

func TestNextRevisionToken(t *testing.T) {
	next, err := NextRevisionToken(MustParseRevisionToken("9"))
	if err != nil {
		t.Fatalf("NextRevisionToken: %v", err)
	}
	if next.String() != "10" {
		t.Fatalf("unexpected next token: %q", next.String())
	}
}

func TestNextRevisionTokenNonNumeric(t *testing.T) {
	if _, err := NextRevisionToken(MustParseRevisionToken("rev-1")); err == nil {
		t.Fatal("expected increment error for non-numeric token")
	}
}
