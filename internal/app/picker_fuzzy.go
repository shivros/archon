package app

import (
	"sort"
	"strings"
	"unicode"
)

type pickerMatch struct {
	index int
	score int
	label string
	id    string
}

func pickerFilterIndices(query string, count int, build func(index int) (label, id, search string)) []int {
	if count <= 0 {
		return nil
	}
	query = normalizePickerMatchText(query)
	if query == "" {
		out := make([]int, count)
		for i := 0; i < count; i++ {
			out[i] = i
		}
		return out
	}
	matches := make([]pickerMatch, 0, count)
	for i := 0; i < count; i++ {
		label, id, search := build(i)
		search = normalizePickerMatchText(search)
		if search == "" {
			search = normalizePickerMatchText(label + " " + id)
		}
		score, ok := fuzzyPickerScore(query, search)
		if !ok {
			continue
		}
		matches = append(matches, pickerMatch{
			index: i,
			score: score,
			label: strings.ToLower(strings.TrimSpace(label)),
			id:    strings.ToLower(strings.TrimSpace(id)),
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if matches[i].label != matches[j].label {
			return matches[i].label < matches[j].label
		}
		if matches[i].id != matches[j].id {
			return matches[i].id < matches[j].id
		}
		return matches[i].index < matches[j].index
	})
	out := make([]int, 0, len(matches))
	for _, match := range matches {
		out = append(out, match.index)
	}
	return out
}

func fuzzyPickerScore(query, candidate string) (int, bool) {
	qr := []rune(normalizePickerMatchText(query))
	cr := []rune(normalizePickerMatchText(candidate))
	if len(qr) == 0 {
		return 0, true
	}
	if len(cr) == 0 {
		return 0, false
	}
	qi := 0
	prev := -2
	score := 0
	for ci, ch := range cr {
		if qi >= len(qr) || ch != qr[qi] {
			continue
		}
		bonus := 10
		if ci == 0 || pickerBoundaryRune(cr[ci-1]) {
			bonus += 6
		}
		if prev+1 == ci {
			bonus += 8
		}
		if ci < 8 {
			bonus += 4
		}
		score += bonus
		prev = ci
		qi++
		if qi == len(qr) {
			break
		}
	}
	if qi != len(qr) {
		return 0, false
	}
	score -= len(cr) - len(qr)
	return score, true
}

func normalizePickerMatchText(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

func pickerBoundaryRune(ch rune) bool {
	if unicode.IsSpace(ch) {
		return true
	}
	return ch == '_' || ch == '-' || ch == '/' || ch == '.' || ch == ':'
}
