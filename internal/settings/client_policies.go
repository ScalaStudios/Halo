package settings

import (
	"slices"

	"halo/internal/httpx"
	"halo/internal/store"
)

func validateClientPolicies(policies []store.ClientPolicy) error {
	seen := map[string]bool{}
	for _, policy := range policies {
		if !slices.Contains([]string{"all", "web", "spa", "native", "service"}, policy.ApplicationType) || seen[policy.ApplicationType] {
			return httpx.Invalid("Use at most one client policy per application type: all, web, spa, native, or service.")
		}
		seen[policy.ApplicationType] = true
		if slices.ContainsFunc(policy.AllowedAuthMethods, func(method string) bool {
			return !slices.Contains([]string{"none", "client_secret_basic", "private_key_jwt"}, method)
		}) {
			return httpx.Invalid("Choose client authentication methods from none, client_secret_basic, or private_key_jwt.")
		}
		limits := []struct{ seconds, minimum, maximum int }{
			{policy.MaxAccessTokenTTL, 60, 86400},
			{policy.MaxIDTokenTTL, 60, 86400},
			{policy.MaxRefreshTokenTTL, 1, 90 * 86400},
		}
		for _, limit := range limits {
			if limit.seconds != 0 && (limit.seconds < limit.minimum || limit.seconds > limit.maximum) {
				return httpx.Invalid("Set client policy lifetime limits within the supported token lifetimes, or use 0 for no limit.")
			}
		}
	}
	return nil
}
