package bridge

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"miftah.local/plugin/internal/core"
	"net/http"
	"time"
)

const priceSource = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// Fetch a public catalog only: never send keys, model selections or user data.
// No redirects or caller-supplied URLs; cap time and response size.
func referencePrices() Response {
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	res, err := client.Get(priceSource)
	if err != nil {
		return reply(502, map[string]string{"error": "price source unavailable; existing prices unchanged"})
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return reply(502, map[string]string{"error": "price source unavailable; existing prices unchanged"})
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return reply(502, map[string]string{"error": "invalid price catalog"})
	}
	prices, err := parsePrices(data)
	if err != nil {
		return reply(502, map[string]string{"error": "invalid price catalog"})
	}
	return reply(200, map[string]any{"prices": prices, "source": priceSource, "fetched_at": time.Now().UTC().Format(time.RFC3339)})
}

func parsePrices(data []byte) (map[string]core.Price, error) {
	var rows map[string]json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	prices := map[string]core.Price{}
	for id, raw := range rows {
		if id == "" || len(id) > 200 {
			continue
		}
		var row struct {
			Input  *float64 `json:"input_cost_per_token"`
			Output *float64 `json:"output_cost_per_token"`
			Mode   string   `json:"mode"`
		}
		if json.Unmarshal(raw, &row) != nil || row.Input == nil || row.Output == nil || (row.Mode != "chat" && row.Mode != "completion") {
			continue
		}
		in, out := *row.Input*1e12, *row.Output*1e12
		if in < 0 || out < 0 || in > 1e9 || out > 1e9 || math.IsNaN(in) || math.IsNaN(out) {
			continue
		}
		prices[id] = core.Price{InputPrice: int64(math.Round(in)), OutputPrice: int64(math.Round(out)), Source: "LiteLLM reference"}
	}
	if len(prices) == 0 {
		return nil, errors.New("empty catalog")
	}
	return prices, nil
}
