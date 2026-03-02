package transcriptdomain

import (
	"fmt"
	"strconv"
	"strings"
)

type RevisionToken string

func ParseRevisionToken(raw string) (RevisionToken, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", fmt.Errorf("revision token is required")
	}
	if strings.ContainsAny(token, " \t\n\r") {
		return "", fmt.Errorf("revision token contains whitespace")
	}
	return RevisionToken(token), nil
}

func MustParseRevisionToken(raw string) RevisionToken {
	token, err := ParseRevisionToken(raw)
	if err != nil {
		panic(err)
	}
	return token
}

func (t RevisionToken) String() string {
	return strings.TrimSpace(string(t))
}

func (t RevisionToken) IsZero() bool {
	return strings.TrimSpace(string(t)) == ""
}

func (t RevisionToken) Sequence() (uint64, bool) {
	trimmed := t.String()
	if trimmed == "" {
		return 0, false
	}
	seq, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}

func CompareRevisionTokens(left, right RevisionToken) (int, error) {
	if _, err := ParseRevisionToken(left.String()); err != nil {
		return 0, fmt.Errorf("left revision: %w", err)
	}
	if _, err := ParseRevisionToken(right.String()); err != nil {
		return 0, fmt.Errorf("right revision: %w", err)
	}
	leftSeq, leftOK := left.Sequence()
	rightSeq, rightOK := right.Sequence()
	if leftOK && rightOK {
		switch {
		case leftSeq < rightSeq:
			return -1, nil
		case leftSeq > rightSeq:
			return 1, nil
		default:
			return 0, nil
		}
	}
	return strings.Compare(left.String(), right.String()), nil
}

func IsRevisionNewer(candidate, current RevisionToken) (bool, error) {
	cmp, err := CompareRevisionTokens(candidate, current)
	if err != nil {
		return false, err
	}
	return cmp > 0, nil
}

func NextRevisionToken(current RevisionToken) (RevisionToken, error) {
	seq, ok := current.Sequence()
	if !ok {
		return "", fmt.Errorf("cannot increment non-numeric revision token %q", current.String())
	}
	return RevisionToken(strconv.FormatUint(seq+1, 10)), nil
}
