package core

import (
	"errors"
	"strings"
)

// Prices are microdollars per million tokens, like existing route prices.
type Price struct {
	InputPrice  int64  `json:"input_price"`
	OutputPrice int64  `json:"output_price"`
	Source      string `json:"source,omitempty"`
}

func validKeyPrices(k Key) error {
	if k.PricingMode != "" && k.PricingMode != "models" {
		return errors.New("invalid pricing mode")
	}
	if len(k.Prices) > 1000 {
		return errors.New("too many custom prices")
	}
	for id, p := range k.Prices {
		if id == "" || len(id) > 200 || strings.TrimSpace(id) != id || p.InputPrice < 0 || p.OutputPrice < 0 || p.InputPrice > 1e9 || p.OutputPrice > 1e9 || len(p.Source) > 200 {
			return errors.New("invalid custom model price")
		}
	}
	return nil
}

func reservationPrices(st State, k Key, r Route) map[string]Price {
	prices := make(map[string]Price, len(r.Targets))
	for _, target := range r.Targets {
		p := Price{InputPrice: r.InputPrice, OutputPrice: r.OutputPrice}
		if k.PricingMode == "models" {
			if direct, ok := route(st, target); ok && direct.Kind == "direct" {
				p = Price{InputPrice: direct.InputPrice, OutputPrice: direct.OutputPrice}
			}
		}
		if custom, ok := k.Prices[target]; ok {
			p = custom
		}
		prices[target] = p
	}
	return prices
}
