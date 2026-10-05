package d2enum

// Priority of the event handler
type Priority int

// Priorities
const (
	PriorityLow Priority = iota
	PriorityDefault
	PriorityHigh

	// PriorityTop is above the console: F8's note box (5 Oct 2026), which
	// must be able to open over anything and, open, takes every key.
	PriorityTop
)
