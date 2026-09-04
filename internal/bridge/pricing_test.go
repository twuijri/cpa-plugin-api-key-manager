package bridge

import "testing"

func TestUsageWithNestedDetails(t *testing.T) {
	in, out, ok := usage([]byte(`{"usage":{"prompt_tokens":3,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":2},"completion_tokens_details":{"reasoning_tokens":1}}}`))
	if !ok || in != 3 || out != 4 {
		t.Fatal(in, out, ok)
	}
	if _, _, ok := usage([]byte(`{"usage":{"prompt_tokens":null,"completion_tokens":4}}`)); ok {
		t.Fatal("null tokens accepted")
	}
}

func TestReferencePricesExactConversion(t *testing.T) {
	rows, err := parsePrices([]byte(`{"model-a":{"mode":"chat","input_cost_per_token":0.000002,"output_cost_per_token":0.000008},"free":{"mode":"chat","input_cost_per_token":0,"output_cost_per_token":0},"missing":{"mode":"chat","input_cost_per_token":0.1},"negative":{"mode":"chat","input_cost_per_token":-1,"output_cost_per_token":0},"image":{"mode":"image_generation","input_cost_per_token":0,"output_cost_per_token":0}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows["model-a"].InputPrice != 2000000 || rows["model-a"].OutputPrice != 8000000 {
		t.Fatal(rows)
	}
	for _, raw := range []string{`null`, `{}`, `[]`, `invalid`} {
		if _, err := parsePrices([]byte(raw)); err == nil {
			t.Fatal("accepted invalid catalog")
		}
	}
}
