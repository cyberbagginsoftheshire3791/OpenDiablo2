package d2player

// T10 (23 Sep 2026): the help overlay (H) speaks Strigoi. D2's bullets --
// hold Ctrl to run, Alt to highlight items, F1-F8 for skills -- described a
// game this is not; a friend pressing H on his first night needs the verbs
// this one has. The D2 callouts along the bottom (orbs, belt, mini-panel)
// still point at the D2 HUD that is still there, and are left.

// strigoiHelpTitle replaces "Diablo II Help".
const strigoiHelpTitle = "Strigoi -- what you can do"

// strigoiHelp is the bulleted list, one line a verb, in the order a first
// night asks for them. The keys are the constants the controls read, so a
// rebinding that forgets this list is caught by TestStrigoiHelpNamesTheKeys.
func strigoiHelp() []string {
	return []string{
		"Click the ground to walk -- in a fight, your Move (2 tiles).",
		"F or click an enemy to strike -- in a fight, your Action. E ends your turn.",
		"L lights or douses the torch in your off hand.",
		"I opens your kit: wear, eat, and MAKE.   T opens your talents.",
		"K forages for branches: half an hour, head down.",
		"X stakes the dead man at your feet.   D digs him a hasty grave.",
		"Click a villager to talk: 1-9 to answer, Esc to walk away.",
		"H shows or hides this help.   Esc opens the game menu.",
	}
}
