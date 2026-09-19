package d2term

import "testing"

// The variadic argument marker, M4.4c-2a ask 4, recommendation (b). These sit
// in their own file rather than in terminal_test.go so that an upstream merge
// of that file never has to be reconciled against Strigoi's assertions.
//
// parseCommand splits on spaces unless inside quotes, so a console command
// that wants a SENTENCE cannot have one. Measured at S0 item 7: `js 1 + 1`
// returns "command requires different argument count", which is exactly what a
// friend typing `wish I wanted to hide` would have got.
//
// A command whose LAST declared argument ends in "..." now takes the rest of
// the line as that one argument.

// capture binds name with the given declared arguments and returns a pointer
// to the slice the command was actually called with. A nil slice means the
// command never ran, which is a different failure from running with the wrong
// arguments, and the tests below need to tell those apart.
func capture(t *testing.T, name string, declared []string) (*Terminal, *[]string) {
	t.Helper()

	term, err := NewTerminal()
	if err != nil {
		t.Fatal(err)
	}

	var got []string

	if err := term.Bind(name, "test", declared, func(args []string) error {
		got = append([]string(nil), args...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	return term, &got
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// TestAVariadicTailTakesTheRestOfTheLine is the assertion the wish note needs.
func TestAVariadicTailTakesTheRestOfTheLine(t *testing.T) {
	term, got := capture(t, "wish", []string{"text..."})

	if err := term.Execute("wish I wanted to hide"); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	want := []string{"I wanted to hide"}
	if !equal(*got, want) {
		t.Fatalf("got %q, want %q", *got, want)
	}
}

// TestAVariadicTailLeavesAQuotedArgumentAlone: re-joining one word is the
// identity, so the quoted form a friend might still type keeps working and
// keeps its inner spacing.
func TestAVariadicTailLeavesAQuotedArgumentAlone(t *testing.T) {
	term, got := capture(t, "wish", []string{"text..."})

	if err := term.Execute(`wish "I wanted  to hide"`); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	want := []string{"I wanted  to hide"}
	if !equal(*got, want) {
		t.Fatalf("got %q, want %q", *got, want)
	}
}

// TestArgumentsBeforeAVariadicTailArePassedThrough: only the LAST declared
// argument swallows the rest, and the ones before it still arrive one word
// each.
func TestArgumentsBeforeAVariadicTailArePassedThrough(t *testing.T) {
	term, got := capture(t, "note", []string{"kind", "text..."})

	if err := term.Execute("note fear I wanted to hide"); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	want := []string{"fear", "I wanted to hide"}
	if !equal(*got, want) {
		t.Fatalf("got %q, want %q", *got, want)
	}
}

// TestAVariadicTailStillNeedsTheArgumentsBeforeIt: the tail may be empty of
// extra words but the leading arguments are not optional, so a short line is
// still the count error and the command does not run on half its input.
func TestAVariadicTailStillNeedsTheArgumentsBeforeIt(t *testing.T) {
	term, got := capture(t, "note", []string{"kind", "text..."})

	err := term.Execute("note fear")
	if err == nil {
		t.Fatal("Execute accepted a line with no text after the kind")
	}

	if *got != nil {
		t.Fatalf("the command ran anyway, with %q", *got)
	}
}

// TestACommandWithoutAVariadicTailStillNeedsItsExactCount is the negative
// control's other half, and the reason `js` is left as it is: the marker must
// change nothing for a command that does not ask for it. This is S0 item 7's
// measured `js 1 + 1` in unit-test form.
func TestACommandWithoutAVariadicTailStillNeedsItsExactCount(t *testing.T) {
	term, got := capture(t, "js", []string{"code"})

	err := term.Execute("js 1 + 1")
	if err == nil {
		t.Fatal("Execute accepted three words for a one-argument command")
	}

	const want = "command requires different argument count"
	if err.Error() != want {
		t.Fatalf("got %q, want %q", err.Error(), want)
	}

	if *got != nil {
		t.Fatalf("the command ran anyway, with %q", *got)
	}
}

// TestATrailingEllipsisIsOnlyAMarkerOnTheLastArgument: "..." in the middle of
// the declared list means nothing, because the rest of the line can only be
// swallowed once and swallowing it early would silently eat the arguments
// after it.
func TestATrailingEllipsisIsOnlyAMarkerOnTheLastArgument(t *testing.T) {
	term, got := capture(t, "note", []string{"text...", "kind"})

	err := term.Execute("note I wanted to hide fear")
	if err == nil {
		t.Fatal("Execute treated a non-final ... as a variadic marker")
	}

	if *got != nil {
		t.Fatalf("the command ran anyway, with %q", *got)
	}
}
