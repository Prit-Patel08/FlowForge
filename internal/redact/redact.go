package redact

import "regexp"

var (
	reBearerToken = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9\-._~+/]+=*`)
	reNamedSecret = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\b\s*[:=]\s*[^\s,;]+`)
	reEnvSecret   = regexp.MustCompile(`(?i)\b[A-Z0-9_]*(API[_-]?KEY|TOKEN|SECRET|PASSWORD)\b=[^\s,;]+`)
	reCLISecret   = regexp.MustCompile(`(?i)(--?(?:api[_-]?key|token|secret|password))\s+("[^"]+"|'[^']+'|[^\s,;]+)`)
	reAwsKey      = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	reLongHex     = regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)
)

// Line redacts common token/key patterns before lines are surfaced to dashboards.
func Line(s string) string {
	s = reBearerToken.ReplaceAllString(s, "Bearer <REDACTED>")
	s = reNamedSecret.ReplaceAllString(s, "$1=<REDACTED>")
	s = reEnvSecret.ReplaceAllString(s, "<REDACTED_SECRET_ENV>")
	s = reCLISecret.ReplaceAllString(s, "$1 <REDACTED>")
	s = reAwsKey.ReplaceAllString(s, "<REDACTED_AWS_KEY>")
	s = reLongHex.ReplaceAllString(s, "<REDACTED_HEX>")
	return s
}
