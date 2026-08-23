package model

import "testing"

func TestIsBroadcast(t *testing.T) {
	cases := []struct {
		name string
		toIP string
		want bool
	}{
		{"empty means broadcast", "", true},
		{"255.255.255.255 is broadcast", "255.255.255.255", true},
		{"other LAN unicast is not broadcast", "192.168.1.10", false},
		{"another unicast is not broadcast", "10.0.0.5", false},
		{"0.0.0.0 is not broadcast by itself", "0.0.0.0", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := Message{ToIP: c.toIP}
			if got := m.IsBroadcast(); got != c.want {
				t.Fatalf("IsBroadcast(%q) = %v, want %v", c.toIP, got, c.want)
			}
		})
	}
}

func TestTargets(t *testing.T) {
	const local = "192.168.1.5"
	cases := []struct {
		name string
		toIP string
		want bool
	}{
		{"broadcast reaches local", "255.255.255.255", true},
		{"empty toIP reaches local", "", true},
		{"unicast to self reaches local", local, true},
		{"unicast to other LAN addr must NOT reach local", "192.168.1.99", false},
		{"unicast to a different subnet must NOT reach local", "10.0.0.7", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := Message{ToIP: c.toIP}
			if got := m.Targets(local); got != c.want {
				t.Fatalf("Targets(%q) for local %q = %v, want %v", c.toIP, local, got, c.want)
			}
		})
	}
}
