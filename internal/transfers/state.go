package transfers

type State string

const (
	StateRequested   State = "Requested"
	StateValidating  State = "Validating"
	StateAnalyzing   State = "Analyzing"
	StateApproved    State = "Approved"
	StateFundsPosted State = "FundsPosted"
	StateCompleted   State = "Completed"
	StateRejected    State = "Rejected"
	StateReturned    State = "Returned"
)

// ValidTransitions maps each state to its allowed next states.
var ValidTransitions = map[State][]State{
	StateRequested:   {StateValidating},
	StateValidating:  {StateAnalyzing, StateRejected},
	StateAnalyzing:   {StateApproved, StateRejected},
	StateApproved:    {StateFundsPosted},
	StateFundsPosted: {StateCompleted, StateReturned},
	StateCompleted:   {StateReturned},
	StateRejected:    {},
	StateReturned:    {},
}

func CanTransition(from, to State) bool {
	targets, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == to {
			return true
		}
	}
	return false
}

func IsTerminal(s State) bool {
	return s == StateRejected || s == StateReturned
}
