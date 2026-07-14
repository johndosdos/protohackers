package main

import (
	"regexp"
	"testing"
)

func TestBoguscoin(t *testing.T) {
	// tonyAddress := "7YWHMfk9JZe0LM0g1ZauHuiSxhI"

	r := regexp.MustCompile(`^7[a-zA-Z0-9]{25,34}$`)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid 26 char input",
			input: "7F1u3wSD5RbOHQmupo9nx4TnhQ",
			want:  true,
		},
		{
			name:  "valid 32 char input",
			input: "7REGLO5mmTPlNZiL5SjsAHdheig4KFjH",
			want:  true,
		},
		{
			name:  "doesn't start with 7",
			input: "4V4Df4q5yMCqlDXnyYFyZKOudnWLzI",
			want:  false,
		},
		{
			name:  "char length [24] doesn't meet requirements",
			input: "UQkZNuE35WoDoxOrcJ1bbGeU",
			want:  false,
		},
		{
			name:  "char length [36] doesn't meet requirements",
			input: "CHfSwc1W6ZBlbI3RBjn2MXgiVOmaWcctwgnl",
			want:  false,
		},
		{
			name:  "with multiple whitespace",
			input: "   7HfSwc1W6ZBlbI3RBjn2MXgiVOmaWcctwgnl   ",
			want:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := isBoguscoin(r, test.input)
			if got != test.want {
				t.Errorf("isBoguscoin(%q) = %t, want=%t", test.input, got, test.want)
			}
		})
	}
}
