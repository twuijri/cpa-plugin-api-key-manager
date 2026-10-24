package desktop

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// NativeModel describes an account's native Gemini mode, not a public API model ID.
type NativeModel struct {
	ConfigID  string `json:"config_id"`
	Title     string `json:"title"`
	Category  int    `json:"category"`
	VariantID string `json:"variant_id,omitempty"`
}

// Alias is a readable proxy identifier; Category and VariantID select the native mode.
func (m NativeModel) Alias() string {
	var b strings.Builder
	separator := false
	runes := []rune(strings.ToLower(m.Title))
	for i, r := range runes {
		decimalDot := r == '.' && i > 0 && i+1 < len(runes) && unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || decimalDot {
			if separator && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			separator = false
		} else {
			separator = true
		}
	}
	if b.Len() == 0 {
		return fmt.Sprintf("gemini-desktop-mode-%d", m.Category)
	}
	return "gemini-desktop-" + b.String()
}

const userStatusMethod = "/assistant.mobile.robin.RobinService/GetUserStatus"

// DiscoverModels reads only native model metadata from the account's status response.
func DiscoverModels(ctx context.Context, token, proxyURL string) ([]NativeModel, error) {
	if token == "" {
		return nil, failure(401, "Gemini access token is missing")
	}
	conn, err := nativeConnection(proxyURL)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return discoverModelsOnConnection(ctx, conn, token)
}

func discoverModelsOnConnection(ctx context.Context, conn grpc.ClientConnInterface, token string) ([]NativeModel, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	var response []byte
	if err := conn.Invoke(ctx, userStatusMethod, []byte{}, &response); err != nil {
		return nil, rpcError(err)
	}
	models, err := parseNativeModels(response)
	if err != nil {
		return nil, failure(502, "Gemini returned invalid native model metadata")
	}
	return models, nil
}

func validModelText(s string, limit int, required bool) bool {
	if len(s) > limit || !utf8.ValidString(s) || (required && strings.TrimSpace(s) == "") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}
func validateNativeModel(m NativeModel) error {
	if m.Category <= 0 || int64(m.Category) > 2147483647 || !validModelText(m.Title, 128, true) || !validModelText(m.VariantID, 256, false) || !validModelText(m.ConfigID, 128, false) {
		return fmt.Errorf("invalid native mode")
	}
	return nil
}

// Mode policy (34.2.1) is authoritative, including disabled modes. Legacy
// configs (17) supply stable config IDs by category, or act as an old-server fallback.
func parseNativeModels(response []byte) ([]NativeModel, error) {
	top, err := parseWire(response)
	if err != nil {
		return nil, err
	}
	var legacy []NativeModel
	var policies [][]byte
	for _, f := range top {
		switch f.Number {
		case 17:
			if f.Kind != 2 {
				return nil, fmt.Errorf("invalid legacy mode field")
			}
			m, _, err := parseNativeMode(f.Bytes, false)
			if err != nil {
				return nil, err
			}
			legacy = append(legacy, m)
		case 34:
			if f.Kind != 2 {
				return nil, fmt.Errorf("invalid mode policy field")
			}
			policies = append(policies, f.Bytes)
		}
	}
	models := []NativeModel{}
	if len(policies) > 0 {
		for _, policy := range policies {
			configs, err := nativePolicyConfigs(policy)
			if err != nil {
				return nil, err
			}
			for _, config := range configs {
				m, disabled, err := parseNativeMode(config, true)
				if err != nil {
					return nil, err
				}
				if disabled {
					continue
				}
				if m.ConfigID == "" {
					for _, old := range legacy {
						if old.Category == m.Category {
							m.ConfigID = old.ConfigID
							break
						}
					}
				}
				models = append(models, m)
			}
		}
	} else {
		models = legacy
	}
	out := make([]NativeModel, 0, len(models))
	seen := map[string]bool{}
	aliases := map[string]string{}
	for _, m := range models {
		if err := validateNativeModel(m); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%d:%s", m.Category, m.VariantID)
		if seen[key] {
			continue
		}
		seen[key] = true
		alias := m.Alias()
		if previous, ok := aliases[alias]; ok && previous != key {
			return nil, fmt.Errorf("ambiguous native mode alias")
		}
		aliases[alias] = key
		out = append(out, m)
		if len(out) > 64 {
			return nil, fmt.Errorf("too many native modes")
		}
	}
	return out, nil
}

func parseNativeMode(data []byte, modern bool) (NativeModel, bool, error) {
	var m NativeModel
	fields, err := parseWire(data)
	if err != nil {
		return m, false, err
	}
	titleFallback := ""
	disabled := false
	for _, f := range fields {
		categoryField, titleField, fallbackField := 23, 19, 2
		if modern {
			categoryField, titleField, fallbackField = 3, 14, 6
		}
		switch f.Number {
		case 1:
			if f.Kind != 2 {
				return m, false, fmt.Errorf("invalid mode ID")
			}
			m.ConfigID = string(f.Bytes)
		case categoryField:
			if f.Kind != 0 || f.Integer > 2147483647 {
				return m, false, fmt.Errorf("invalid mode category")
			}
			m.Category = int(f.Integer)
		case titleField:
			if f.Kind != 2 {
				return m, false, fmt.Errorf("invalid mode title")
			}
			m.Title = string(f.Bytes)
		case fallbackField:
			if f.Kind != 2 {
				return m, false, fmt.Errorf("invalid fallback title")
			}
			titleFallback = string(f.Bytes)
		default:
			if modern && f.Number == 4 {
				if f.Kind != 0 {
					return m, false, fmt.Errorf("invalid disabled flag")
				}
				disabled = f.Integer != 0
			}
			if modern && f.Number == 5 {
				if f.Kind != 2 {
					return m, false, fmt.Errorf("invalid mode variant")
				}
				m.VariantID = string(f.Bytes)
			}
		}
	}
	if m.Title == "" {
		m.Title = titleFallback
	}
	if err := validateNativeModel(m); err != nil {
		return m, false, err
	}
	return m, disabled, nil
}

func nativePolicyConfigs(policy []byte) ([][]byte, error) {
	fields, err := parseWire(policy)
	if err != nil {
		return nil, err
	}
	var configs [][]byte
	for _, f := range fields {
		if f.Number != 2 {
			continue
		}
		if f.Kind != 2 {
			return nil, fmt.Errorf("invalid policy config")
		}
		entries, err := parseWire(f.Bytes)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Number != 1 {
				continue
			}
			if entry.Kind != 2 {
				return nil, fmt.Errorf("invalid policy mode")
			}
			configs = append(configs, entry.Bytes)
		}
	}
	return configs, nil
}
