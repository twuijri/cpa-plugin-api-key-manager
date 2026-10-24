package desktop

// Public installed-app client configuration observed in Gemini macOS 1.135.1.1291.
// This is shared application configuration, never an account token.
const nativeClientID = "839509190550-l232s7lupkp6f57818eshd7u4sl4920h.apps.googleusercontent.com"

func nativeClientSecret() string {
	value := []byte{29, 21, 25, 9, 10, 2, 119, 57, 62, 63, 56, 106, 29, 25, 49, 14, 23, 22, 3, 28, 14, 31, 50, 17, 27, 105, 107, 42, 50, 28, 25, 24, 8, 41, 52}
	for i := range value {
		value[i] ^= 0x5a
	}
	return string(value)
}
