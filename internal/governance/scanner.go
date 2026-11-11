package governance

import "regexp"

var invisibleCharsRegex = regexp.MustCompile("[\u200B\u200C\u200D\uFEFF\u0000\u00AD]")

// Scanner is responsible for detecting prompt injections and malicious payloads in transit.
type Scanner struct {
	injectionRegex *regexp.Regexp
}

// NewScanner initializes the RE2 engine with predefined threat signatures.
// It compiles the regex once, making CheckTaint thread-safe and highly performant.
func NewScanner() *Scanner {
	return &Scanner{
		// Matches common Portuguese and English bypass attempts, handling newlines and spacing.
		injectionRegex: regexp.MustCompile(`(?i)(ignore[\s\n]*.*instruções|ignore[\s\n]*.*instructions|acesso[\s\n]*root)`),
	}
}

// normalizePayload strips invisible unicode evasion characters (zero-width spaces, soft hyphens, null bytes).
func normalizePayload(payload string) string {
	return invisibleCharsRegex.ReplaceAllString(payload, "")
}

// CheckTaint evaluates the payload and returns true if it contains malicious signatures.
func (s *Scanner) CheckTaint(payload string) bool {
	normalized := normalizePayload(payload)
	return s.injectionRegex.MatchString(normalized)
}
