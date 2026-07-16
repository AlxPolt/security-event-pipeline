package sanitizer

import (
	"regexp"
	"unicode/utf8"
)

const (
	maxStringLength = 1000
	truncateSuffix  = "...[truncated]"
)

var (
	rePassword = regexp.MustCompile(`(?i)(password|pwd|pass)[\s"]*[:=][\s"]*[^\s"&]+`)
	reToken    = regexp.MustCompile(`(?i)(token|bearer[\s]+|authorization[\s]*[:=][\s"]*(?:bearer\s+)?)[^\s"&]+`)
	reSecret   = regexp.MustCompile(`(?i)(client_secret|secret)[\s"]*[:=][\s"]*[^\s"&]+`)
	reAPIKey   = regexp.MustCompile(`(?i)(api[_-]?key|apikey)[\s"]*[:=][\s"]*[^\s"&]+`)
	reURLCreds = regexp.MustCompile(`(https?|nats|amqp|redis|postgres|mysql)://([^:]+):([^@]+)@`)
)

func Sanitize(input string) string {
	if input == "" {
		return ""
	}

	s := truncateRunes(input, maxStringLength)

	s = rePassword.ReplaceAllString(s, "password=***")
	s = reToken.ReplaceAllString(s, "token=***")
	s = reSecret.ReplaceAllString(s, "secret=***")
	s = reAPIKey.ReplaceAllString(s, "api_key=***")
	s = reURLCreds.ReplaceAllString(s, "$1://$2:***@")

	return s
}

func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return Sanitize(err.Error())
}

func truncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	i := 0
	for pos := range s {
		if i == maxRunes {
			return s[:pos] + truncateSuffix
		}
		i++
	}
	return s
}
