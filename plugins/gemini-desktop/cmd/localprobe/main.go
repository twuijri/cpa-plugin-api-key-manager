package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gemini-desktop-plugin/internal/desktop"
	"os"
)

func main() {
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err = json.Unmarshal(raw, &token); err != nil {
		panic(err)
	}
	err = desktop.Generate(context.Background(), token.AccessToken, "", "Reply with exactly GEMINI_PLUGIN_OK", func(text string) error { fmt.Println("snapshot bytes", len(text)); return nil })
	if err != nil {
		fmt.Printf("error: %v; cause: %v\n", err, errors.Unwrap(err))
	}
}
