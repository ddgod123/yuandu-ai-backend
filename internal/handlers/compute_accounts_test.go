package handlers

import "testing"

func TestParseOptionalInt64Query(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		value, err := parseOptionalInt64Query(" ")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		if value != nil {
			t.Fatalf("expected nil value, got %v", *value)
		}
	})

	t.Run("valid", func(t *testing.T) {
		value, err := parseOptionalInt64Query(" 123 ")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		if value == nil || *value != 123 {
			t.Fatalf("expected 123, got %+v", value)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		if _, err := parseOptionalInt64Query("12.3"); err == nil {
			t.Fatalf("expected parse error")
		}
	})
}

func TestParseOptionalBoolQuery(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    bool
		hasWant bool
		wantErr bool
	}{
		{name: "empty", input: "", hasWant: false},
		{name: "true_text", input: "true", want: true, hasWant: true},
		{name: "true_numeric", input: "1", want: true, hasWant: true},
		{name: "false_text", input: "false", want: false, hasWant: true},
		{name: "false_numeric", input: "0", want: false, hasWant: true},
		{name: "invalid", input: "maybe", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, err := parseOptionalBoolQuery(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil err, got %v", err)
			}
			if !tc.hasWant {
				if value != nil {
					t.Fatalf("expected nil value")
				}
				return
			}
			if value == nil {
				t.Fatalf("expected bool value")
			}
			if *value != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, *value)
			}
		})
	}
}
