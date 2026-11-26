package alarmmgmt

import "testing"

// The ISA-18.2 state follows from the condition, the acknowledgement, the class and the shelf.
func TestState(t *testing.T) {
	for _, c := range []struct {
		name                                 string
		cleared, acked, ackRequired, shelved bool
		want                                 string
	}{
		{"raised", false, false, true, false, UnackActive},
		{"acknowledged while active", false, true, true, false, AckActive},
		{"returned to normal before anyone acknowledged", true, false, true, false, UnackRTN},
		{"returned to normal and acknowledged", true, true, true, false, Normal},
		{"class without acknowledgement, active", false, false, false, false, AckActive},
		{"class without acknowledgement, returned", true, false, false, false, Normal},
		{"shelved wins over everything", false, false, true, true, Shelved},
		{"shelved and returned", true, false, true, true, Shelved},
	} {
		if got := State(c.cleared, c.acked, c.ackRequired, c.shelved); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// Alarms without a class of their rule take the built-in class of their severity; the built-in classes are the three
// the migration gives every organization.
func TestBuiltinClasses(t *testing.T) {
	if CodeFor("warning") != "warning" || CodeFor("action") != "alarm" || CodeFor("critical") != "alarm" {
		t.Fatal("severity → class")
	}
	seen := map[string]Class{}
	for _, c := range Builtin {
		seen[c.Code] = c
		if c.Priority < 1 || c.Priority > 4 || !c.Builtin {
			t.Errorf("%s: priority %d builtin %v", c.Code, c.Priority, c.Builtin)
		}
	}
	if len(seen) != 3 || !seen["alarm"].AckRequired || !seen["warning"].AckRequired || seen["message"].AckRequired {
		t.Fatalf("built-in classes: %+v", seen)
	}
	if seen["alarm"].Priority >= seen["warning"].Priority || seen["warning"].Priority >= seen["message"].Priority {
		t.Fatal("Alarm must come before Warning before Message")
	}
}
