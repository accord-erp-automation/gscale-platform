package godex

import (
	"net/url"
	"strings"

	"golang.org/x/text/unicode/norm"
)

func SanitizeLabelText(v string) string {
	v = norm.NFKC.String(v)
	v = strings.NewReplacer("\r", " ", "\n", " ", "^", " ", "~", " ").Replace(v)
	return strings.Join(strings.Fields(v), " ")
}

func NormalizeKGValue(text string) string {
	value := SanitizeLabelText(text)
	lowered := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lowered, "kg:"):
		value = strings.TrimSpace(strings.SplitN(value, ":", 2)[1])
	case strings.HasSuffix(lowered, "kg"):
		value = strings.TrimSpace(value[:len(value)-2])
	}
	return value
}

func EncodeScanPayload(companyName, productName, kgText, bruttoText, epc string) string {
	parts := []string{companyName, productName, kgText, bruttoText, epc}
	for i := range parts {
		parts[i] = url.QueryEscape(parts[i])
	}
	return DefaultQRBaseURL + strings.Join(parts, "/")
}
