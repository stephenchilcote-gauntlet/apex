# Unit-Testable Invariants

## State Machine (internal/transfers/state.go)
- 8 states: Requested, Validating, Analyzing, Approved, FundsPosted, Completed, Rejected, Returned
- ValidTransitions map:
  - Requested → Validating
  - Validating → Analyzing, Rejected
  - Analyzing → Approved, Rejected
  - Approved → FundsPosted
  - FundsPosted → Completed, Returned
  - Completed → Returned
  - Rejected → (terminal)
  - Returned → (terminal)
- CanTransition(from, to) returns true only for pairs in the map
- CanTransition returns false for unknown states
- IsTerminal returns true only for Rejected and Returned

## Clock (internal/clock/clock.go)
- BusinessDateCT returns current date if before cutoff
- BusinessDateCT returns next day if at/after cutoff
- BusinessDateCT skips Saturday/Sunday to Monday
- Friday after cutoff → Monday
- Saturday/Sunday → Monday (or next weekday)
