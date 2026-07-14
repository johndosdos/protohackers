package main

import (
	"testing"
)

func TestBoguscoin(t *testing.T) {
	// tonyAddress := "7YWHMfk9JZe0LM0g1ZauHuiSxhI"

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single boguscoin address",
			input: "please pay 7F1u3wSD5RbOHQmupo9nx4TnhQ",
			want:  "please pay 7YWHMfk9JZe0LM0g1ZauHuiSxhI",
		},
		{
			name:  "multiple boguscoin addresses",
			input: "please send to 7F1u3wSD5RbOHQmupo9nx4TnhQ and 7adNeSwJkMakpEcln9HEtthSRtxdmEHOT8T",
			want:  "please send to 7YWHMfk9JZe0LM0g1ZauHuiSxhI and 7YWHMfk9JZe0LM0g1ZauHuiSxhI",
		},
		{
			name:  "no boguscoin address (unchanged)",
			input: "hello there!",
			want:  "hello there!",
		},
		{
			name:  "partial boguscoin address (unchanged)",
			input: "from 7F1u",
			want:  "from 7F1u",
		},
		{
			name:  "preserved newline",
			input: "hello there\n",
			want:  "hello there\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := rewriteMessage(test.input)
			if got != test.want {
				t.Errorf("rewriteMessage(%q) = %q, want=%q", test.input, got, test.want)
			}
		})
	}
}
