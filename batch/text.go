package batch

import (
	"math"
	"net/url"
	"strconv"
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
	if rounded, ok := roundKGText(value); ok {
		return rounded
	}
	return value
}

func roundKGText(text string) (string, bool) {
	value := strings.TrimSpace(text)
	if value == "" {
		return "", false
	}
	value = strings.ReplaceAll(value, ",", ".")
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return "", false
	}
	return strconv.FormatFloat(roundToOneDecimal(parsed), 'f', -1, 64), true
}

func roundToOneDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}

func EncodeScanPayload(companyName, productName, kgText, bruttoText, epc string) string {
	parts := []string{companyName, productName, kgText, bruttoText, epc}
	for i := range parts {
		parts[i] = url.QueryEscape(parts[i])
	}
	return DefaultQRBaseURL + strings.Join(parts, "/")
}
