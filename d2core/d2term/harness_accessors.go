package d2term

// HarnessOutput returns terminal output lines starting at index from
// (0-based), plus the total line count, for the Phase 3 playtest harness's
// run_console tool. Compiled in every build configuration; call on the game
// goroutine (the terminal is not otherwise synchronised).
func (t *Terminal) HarnessOutput(from int) (lines []string, total int) {
	total = len(t.outputHistory)

	if from < 0 {
		from = 0
	}

	for i := from; i < total; i++ {
		lines = append(lines, t.outputHistory[i].text)
	}

	return lines, total
}
