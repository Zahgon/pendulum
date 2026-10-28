package dtparser

import (
	"strings"

	"github.com/python-pendulum/pendulum/internal/pyrt"
)

// timelex is _timelex: it splits a string into word, number, space and punctuation
// tokens, following str.isalpha(), str.isdigit() and str.isspace().
type timelex struct {
	in         []rune
	pos        int
	charstack  []rune
	tokenstack []string
	eof        bool
}

func splitTokens(s string) []string {
	lx := &timelex{in: []rune(s)}
	var out []string
	for {
		tok, ok := lx.getToken()
		if !ok {
			return out
		}
		out = append(out, tok)
	}
}

func (lx *timelex) read() (rune, bool) {
	for lx.pos < len(lx.in) {
		c := lx.in[lx.pos]
		lx.pos++
		if c != 0 {
			return c, true
		}
	}
	return 0, false
}

// getToken returns the next token; ok is false where Python returns None.
func (lx *timelex) getToken() (string, bool) {
	if len(lx.tokenstack) > 0 {
		tok := lx.tokenstack[0]
		lx.tokenstack = lx.tokenstack[1:]
		return tok, true
	}
	seenLetters := false
	var token []rune
	haveToken := false
	state := ""
	for !lx.eof {
		var next rune
		if len(lx.charstack) > 0 {
			next = lx.charstack[0]
			lx.charstack = lx.charstack[1:]
		} else {
			c, ok := lx.read()
			if !ok {
				lx.eof = true
				break
			}
			next = c
		}
		last := func() rune { return token[len(token)-1] }
		switch state {
		case "":
			token, haveToken = []rune{next}, true
			if pyrt.IsAlpha(next) {
				state = "a"
				continue
			}
			if pyrt.IsDigit(next) {
				state = "0"
				continue
			}
			if pyrt.IsSpace(next) {
				token = []rune{' '}
			}
		case "a":
			seenLetters = true
			if pyrt.IsAlpha(next) {
				token = append(token, next)
				continue
			}
			if next == '.' {
				token = append(token, next)
				state = "a."
				continue
			}
			lx.charstack = append(lx.charstack, next)
		case "0":
			if pyrt.IsDigit(next) {
				token = append(token, next)
				continue
			}
			if next == '.' || (next == ',' && len(token) >= 2) {
				token = append(token, next)
				state = "0."
				continue
			}
			lx.charstack = append(lx.charstack, next)
		case "a.":
			seenLetters = true
			if next == '.' || pyrt.IsAlpha(next) {
				token = append(token, next)
				continue
			}
			if pyrt.IsDigit(next) && last() == '.' {
				token = append(token, next)
				state = "0."
				continue
			}
			lx.charstack = append(lx.charstack, next)
		case "0.":
			if next == '.' || pyrt.IsDigit(next) {
				token = append(token, next)
				continue
			}
			if pyrt.IsAlpha(next) && last() == '.' {
				token = append(token, next)
				state = "a."
				continue
			}
			lx.charstack = append(lx.charstack, next)
		}
		break
	}
	if !haveToken {
		return "", false
	}
	tok := string(token)
	if (state == "a." || state == "0.") &&
		(seenLetters || strings.Count(tok, ".") > 1 || strings.ContainsRune(".,", token[len(token)-1])) {
		parts := splitDecimal(tok)
		tok = parts[0]
		for _, p := range parts[1:] {
			if p != "" {
				lx.tokenstack = append(lx.tokenstack, p)
			}
		}
	}
	if state == "0." && !strings.Contains(tok, ".") {
		tok = strings.ReplaceAll(tok, ",", ".")
	}
	return tok, true
}

// splitDecimal is re.split('([.,])', s).
func splitDecimal(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == ',' {
			out = append(out, s[start:i], s[i:i+1])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
