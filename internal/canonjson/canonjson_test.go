package canonjson

import (
	"encoding/json"
	"testing"
)

func TestMarshalCanonical(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{map[string]any{"b": 1, "a": []any{true, nil, "x"}}, `{"a":[true,null,"x"],"b":1}`},
		{map[string]any{"z": map[string]any{"y": int64(-3), "x": "<&>"}}, `{"z":{"x":"<&>","y":-3}}`},
		{"tab\tquote\"back\\ctl\x01", `"tab\tquote\"back\\ctl\u0001"`},
		{"é", "\"é\""}, // NFD → NFC
		{json.Number("12345678901234567890"), `12345678901234567890`},
		{[]int64{3, 1}, `[3,1]`},
	}
	for _, c := range cases {
		got, err := Marshal(c.in)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", c.in, err)
		}
		if string(got) != c.want {
			t.Errorf("Marshal(%v) = %s, want %s", c.in, got, c.want)
		}
		if !IsCanonical(got) {
			t.Errorf("IsCanonical(%s) = false", got)
		}
	}
}

func TestRejects(t *testing.T) {
	for _, v := range []any{1.5, float32(2), struct{}{}, map[string]any{"a": 0.1}, json.Number("1.0")} {
		if _, err := Marshal(v); err == nil {
			t.Errorf("Marshal(%v) accepted a non-canonical value", v)
		}
	}
	for _, s := range []string{`{"b":1,"a":2}`, `{"a": 1}`, `[1.0]`, `{"a":1}x`} {
		if IsCanonical([]byte(s)) {
			t.Errorf("IsCanonical(%s) = true", s)
		}
	}
}
