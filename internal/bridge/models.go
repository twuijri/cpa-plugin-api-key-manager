package bridge

import (
	"net/http"
	"sort"
)

const ModelsResourcePath = "/v0/resource/plugins/miftah/models"

// Resource routes bypass CPA's management authentication. Authenticate the
// virtual key here before returning anything; never return the global catalog.
func (a *App) models(r Request) Response {
	if r.Method != http.MethodGet {
		return reply(405, map[string]string{"error": "method not allowed"})
	}
	k, err := a.Store.Authenticate(token(r.Headers))
	if err != nil {
		return reply(401, map[string]any{"error": map[string]string{"message": "Invalid or inactive virtual key", "type": "authentication_error"}})
	}
	names := append([]string(nil), k.Models...)
	sort.Strings(names)
	data := make([]map[string]any, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		// Configured allowed IDs, including optional aliases. Availability at the
		// provider is checked when a request executes, not by making discovery calls.
		data = append(data, map[string]any{"id": name, "object": "model", "created": k.CreatedAt.Unix(), "owned_by": "miftah"})
	}
	return reply(200, map[string]any{"object": "list", "data": data})
}
