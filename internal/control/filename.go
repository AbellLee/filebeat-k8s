package control

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var managedFilenameRE = regexp.MustCompile(`^fbctl-[0-9]{3,5}-[a-zA-Z0-9._-]+\.yml$`)

func SafeName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '.' || r == '_' || r == '-':
			if !lastDash {
				b.WriteRune(r)
				lastDash = r == '-'
			}
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), ".-_")
	if out == "" {
		return "policy"
	}
	return out
}

func SafePathSegment(s string) string {
	return SafeName(s)
}

func ManagedFilename(priority int, policyID string) string {
	if priority < 0 {
		priority = 0
	}
	if priority > 99999 {
		priority = 99999
	}
	return fmt.Sprintf("fbctl-%03d-%s.yml", priority, SafeName(policyID))
}

func ValidateManagedFilename(filename string) error {
	if !managedFilenameRE.MatchString(filename) {
		return fmt.Errorf("invalid managed filename %q", filename)
	}
	if strings.Contains(filename, "/") || strings.Contains(filename, `\`) || strings.Contains(filename, "..") {
		return fmt.Errorf("managed filename must be a base fbctl yml file: %q", filename)
	}
	return nil
}
