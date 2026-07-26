package retrieval

import "strings"

func IsChallenged(body string, statusCode int) bool {
	if statusCode == 403 || statusCode == 429 {
		return true
	}
	if len(body) < 200 && statusCode > 0 {
		return true
	}
	lower := strings.ToLower(body)
	markers := []string{
		"checking your browser",
		"attention required",
		"just a moment",
		"cf-please-wait",
		"verify you are human",
		"access denied",
		"captcha",
		"you have been blocked",
		"noindex",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func IsRefused(body string, statusCode int) bool {
	if statusCode == 403 || statusCode == 429 || statusCode == 503 {
		return true
	}
	lower := strings.ToLower(body)
	refusedMarkers := []string{
		"access denied",
		"you have been blocked",
		"request blocked",
		"rate limit exceeded",
	}
	for _, m := range refusedMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}
