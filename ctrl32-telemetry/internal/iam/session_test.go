package iam

import (
	"testing"
	"time"
)

// "Stay signed in on this device": only when the policy allows it, capped, and following the policy as it is now.
func TestSessionLimits_RememberDevice(t *testing.T) {
	pol := defaultPolicy()
	idle, abs, rem := sessionLimits(pol, true)
	if rem || idle != 30*time.Minute || abs != 8*time.Hour {
		t.Fatalf("policy off: got %v %v %v, want a normal session", idle, abs, rem)
	}
	pol.RememberDeviceDays = 30
	if _, _, rem := sessionLimits(pol, false); rem {
		t.Fatal("not asked for: must not be remembered")
	}
	idle, abs, rem = sessionLimits(pol, true)
	if !rem || idle != 30*24*time.Hour || abs != 30*24*time.Hour {
		t.Fatalf("allowed: got %v %v %v", idle, abs, rem)
	}
	pol.RememberDeviceDays = 1000
	if _, abs, _ := sessionLimits(pol, true); abs != MaxRememberDeviceDays*24*time.Hour {
		t.Fatalf("cap: got %v", abs)
	}
}

func TestSessionIdle_RememberDevice(t *testing.T) {
	pol := defaultPolicy()
	pol.RememberDeviceDays = 30
	if d, err := sessionIdle(pol, true, 10*24*time.Hour); err != nil || d != 30*24*time.Hour {
		t.Fatalf("remembered, 10 days old: %v %v", d, err)
	}
	if _, err := sessionIdle(pol, true, 31*24*time.Hour); err == nil {
		t.Fatal("older than the allowed days must end")
	}
	pol.RememberDeviceDays = 7 // shortened: applies to open sessions
	if _, err := sessionIdle(pol, true, 10*24*time.Hour); err == nil {
		t.Fatal("shortened policy must end a 10 day old session")
	}
	pol.RememberDeviceDays = 0 // switched off: back to the normal limits
	if d, err := sessionIdle(pol, true, time.Hour); err != nil || d != 30*time.Minute {
		t.Fatalf("switched off, 1 h old: %v %v", d, err)
	}
	if _, err := sessionIdle(pol, true, 9*time.Hour); err == nil {
		t.Fatal("switched off, older than the absolute limit must end")
	}
	if d, err := sessionIdle(pol, false, 100*time.Hour); err != nil || d != 30*time.Minute {
		t.Fatalf("normal session: %v %v", d, err)
	}
}
