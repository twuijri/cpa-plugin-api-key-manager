package core

import "errors"

func validateKeyFallbacks(st State, k Key) error {
	if len(k.Fallbacks) > 200 {
		return errors.New("too many fallback rows")
	}
	selected := map[string]bool{}
	for _, m := range k.Models {
		selected[m] = true
	}
	seen := map[string]bool{}
	for _, f := range k.Fallbacks {
		if !selected[f.Primary] || seen[f.Primary] {
			return errors.New("fallback primary must be selected once")
		}
		seen[f.Primary] = true
		p, ok := route(st, f.Primary)
		if !ok || p.Kind != "direct" {
			return errors.New("key fallback primary must be a direct model, not a named route")
		}
		p.Targets = append([]string{f.Primary}, f.Fallbacks...)
		p.RetryStatuses = f.RetryStatuses
		p.RetryUnknown = f.RetryUnknown
		if err := validRoute(p); err != nil {
			return err
		}
		for _, target := range f.Fallbacks {
			r, ok := route(st, target)
			if !ok || r.Kind != "direct" {
				return errors.New("configure direct model pricing for each fallback")
			}
		}
	}
	return nil
}
