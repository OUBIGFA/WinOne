package worker

import "testing"

func TestIdleExitRequiresManagedInitializedEmptySuccessfulState(t *testing.T) {
	ready := idleConditions{Managed: true, Initialized: true}
	if !ready.eligible() {
		t.Fatal("empty managed worker remained resident")
	}
	for _, value := range []idleConditions{
		{Initialized: true}, {Managed: true}, {Managed: true, Initialized: true, Programs: 1},
		{Managed: true, Initialized: true, Busy: true}, {Managed: true, Initialized: true, HasError: true},
		{Managed: true, Initialized: true, RecoveryOnly: true},
	} {
		if value.eligible() {
			t.Fatalf("unsafe idle exit: %+v", value)
		}
	}
}
