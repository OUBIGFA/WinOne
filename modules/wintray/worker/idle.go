package worker

// Empty managed workers can leave after initialization has reconciled native
// startup state. Errors and recovery workers stay available for their owner.
type idleConditions struct {
	Managed      bool
	Initialized  bool
	Programs     int
	Busy         bool
	HasError     bool
	RecoveryOnly bool
}

func (s idleConditions) eligible() bool {
	return s.Managed && s.Initialized && s.Programs == 0 && !s.Busy && !s.HasError && !s.RecoveryOnly
}
