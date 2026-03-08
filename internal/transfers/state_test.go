package transfers

import "testing"

func TestCanTransition_AllPairs(t *testing.T) {
	allStates := []State{
		StateRequested, StateValidating, StateAnalyzing, StateApproved,
		StateFundsPosted, StateCompleted, StateRejected, StateReturned,
	}

	expected := map[State]map[State]bool{
		StateRequested:   {StateValidating: true},
		StateValidating:  {StateAnalyzing: true, StateRejected: true},
		StateAnalyzing:   {StateApproved: true, StateRejected: true},
		StateApproved:    {StateFundsPosted: true},
		StateFundsPosted: {StateCompleted: true, StateReturned: true},
		StateCompleted:   {StateReturned: true},
		StateRejected:    {},
		StateReturned:    {},
	}

	for _, from := range allStates {
		for _, to := range allStates {
			want := expected[from][to]
			got := CanTransition(from, to)
			if got != want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestCanTransition_UnknownState(t *testing.T) {
	if CanTransition(State("bogus"), StateRequested) {
		t.Error("expected false for unknown source state")
	}
	if CanTransition(StateRequested, State("bogus")) {
		t.Error("expected false for unknown target state")
	}
}

func TestIsTerminal(t *testing.T) {
	cases := []struct {
		state State
		want  bool
	}{
		{StateRequested, false},
		{StateValidating, false},
		{StateAnalyzing, false},
		{StateApproved, false},
		{StateFundsPosted, false},
		{StateCompleted, false},
		{StateRejected, true},
		{StateReturned, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			got := IsTerminal(tc.state)
			if got != tc.want {
				t.Errorf("IsTerminal(%s) = %v, want %v", tc.state, got, tc.want)
			}
		})
	}
}
