package governance

import "regexp"

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

// CheckTaint evaluates the payload and returns true if it contains malicious signatures.
func (s *Scanner) CheckTaint(payload string) bool {
	return s.injectionRegex.MatchString(payload)
}
