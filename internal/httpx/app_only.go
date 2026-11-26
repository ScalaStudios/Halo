package httpx

import "strings"

func appOnlyAllows(path string) bool {
	return !strings.HasPrefix(path, "/api/v1/") || strings.HasPrefix(path, "/api/v1/auth/")
}
