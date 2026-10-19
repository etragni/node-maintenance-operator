package maintenance

import "testing"

func TestDecideTable(t *testing.T) {
	cases := []struct {
		name  string
		in    Observation
		want  Action
		phase Phase
	}{
		{"node missing", Observation{}, ActionReject, PhaseFailed},
		{"control plane rejected", Observation{NodeFound: true, NodeIsControlPlane: true}, ActionReject, PhaseFailed},
		{"duplicate rejected", Observation{NodeFound: true, DuplicateRequest: true}, ActionReject, PhaseFailed},
		{"schedulable node is cordoned", Observation{NodeFound: true}, ActionCordon, PhaseCordoned},
		{"cordoned node with pods is drained", Observation{NodeFound: true, NodeUnschedulable: true, PendingPods: 3}, ActionDrain, PhaseDraining},
		{"cordoned empty node is done", Observation{NodeFound: true, NodeUnschedulable: true}, ActionNone, PhaseDrained},
		{"deleting after acting uncordons", Observation{Deleting: true, Acted: true, NodeFound: true, NodeUnschedulable: true}, ActionUncordon, PhaseReleasing},
		{"deleting a rejected request does not touch the node", Observation{Deleting: true, NodeFound: true, NodeUnschedulable: true}, ActionRelease, PhaseReleasing},
		{"deleting with node already schedulable releases", Observation{Deleting: true, Acted: true, NodeFound: true}, ActionRelease, PhaseReleasing},
		{"deleting with node gone releases", Observation{Deleting: true, Acted: true}, ActionRelease, PhaseReleasing},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Decide(c.in)
			if got.Action != c.want || got.Phase != c.phase {
				t.Fatalf("Decide(%+v) = %s/%s, want %s/%s", c.in, got.Action, got.Phase, c.want, c.phase)
			}
		})
	}
}

// TestDecideInvariants enumerates every combination of the boolean inputs and
// checks the safety properties the operator must always satisfy.
func TestDecideInvariants(t *testing.T) {
	for mask := 0; mask < 1<<6; mask++ {
		for _, pods := range []int{0, 3} {
			o := Observation{
				Deleting:           mask&1 != 0,
				Acted:              mask&2 != 0,
				NodeFound:          mask&4 != 0,
				NodeIsControlPlane: mask&8 != 0,
				NodeUnschedulable:  mask&16 != 0,
				DuplicateRequest:   mask&32 != 0,
				PendingPods:        pods,
			}
			d := Decide(o)
			if d != Decide(o) {
				t.Fatalf("Decide is not deterministic for %+v", o)
			}

			if o.Deleting && (d.Action == ActionCordon || d.Action == ActionDrain) {
				t.Errorf("must never cordon/drain while deleting: %+v -> %s", o, d.Action)
			}
			if o.NodeIsControlPlane && !o.Deleting && (d.Action == ActionCordon || d.Action == ActionDrain) {
				t.Errorf("must never touch a control-plane node: %+v -> %s", o, d.Action)
			}
			if o.DuplicateRequest && !o.Deleting && (d.Action == ActionCordon || d.Action == ActionDrain) {
				t.Errorf("a duplicate request must never act: %+v -> %s", o, d.Action)
			}
			if d.Action == ActionUncordon && !(o.Acted && o.NodeFound && o.NodeUnschedulable) {
				t.Errorf("uncordon only a node we cordoned: %+v", o)
			}
			if d.Action == ActionDrain && !o.NodeUnschedulable {
				t.Errorf("must cordon before draining: %+v", o)
			}
			if d.Action == ActionNone && d.Phase != PhaseDrained {
				t.Errorf("ActionNone must report Drained: %+v", o)
			}
		}
	}
}
