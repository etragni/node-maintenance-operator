// Package maintenance contains the pure decision logic of the NodeMaintenance
// operator. It does not depend on Kubernetes: it receives an Observation of the
// world and returns the Decision to execute. Keeping this logic free of side
// effects makes it trivial to test exhaustively.
package maintenance

import "time"

// Phase is the lifecycle phase reported in status.phase.
type Phase string

const (
	PhaseCordoned  Phase = "Cordoned"
	PhaseDraining  Phase = "Draining"
	PhaseDrained   Phase = "Drained"
	PhaseFailed    Phase = "Failed"
	PhaseReleasing Phase = "Releasing"
)

// Action is the single side effect the controller must perform next.
type Action string

const (
	ActionNone     Action = "None"
	ActionCordon   Action = "Cordon"
	ActionDrain    Action = "Drain"
	ActionUncordon Action = "Uncordon"
	ActionRelease  Action = "Release" // remove the finalizer
	ActionReject   Action = "Reject"  // nothing to do, report the problem
)

// Observation is a snapshot of everything the decision depends on.
type Observation struct {
	Deleting           bool // the NodeMaintenance has a deletionTimestamp
	Acted              bool // the operator already cordoned the node for this request
	NodeFound          bool
	NodeIsControlPlane bool
	NodeUnschedulable  bool // node.spec.unschedulable
	DuplicateRequest   bool // an older NodeMaintenance already targets the same node
	PendingPods        int  // pods the drain would still evict
}

// Decision is the outcome of Decide.
type Decision struct {
	Action       Action
	Phase        Phase
	Message      string
	RequeueAfter time.Duration
}

// Decide maps an Observation to a Decision. It is a pure function: the same
// observation always yields the same decision, which is what makes the
// reconcile loop idempotent.
func Decide(o Observation) Decision {
	if o.Deleting {
		if o.Acted && o.NodeFound && o.NodeUnschedulable {
			return Decision{Action: ActionUncordon, Phase: PhaseReleasing, Message: "restoring scheduling on the node", RequeueAfter: time.Second}
		}
		return Decision{Action: ActionRelease, Phase: PhaseReleasing, Message: "maintenance ended"}
	}

	switch {
	case !o.NodeFound:
		return Decision{Action: ActionReject, Phase: PhaseFailed, Message: "node not found", RequeueAfter: 30 * time.Second}
	case o.NodeIsControlPlane:
		return Decision{Action: ActionReject, Phase: PhaseFailed, Message: "control-plane nodes are not supported"}
	case o.DuplicateRequest:
		return Decision{Action: ActionReject, Phase: PhaseFailed, Message: "another NodeMaintenance already targets this node"}
	case !o.NodeUnschedulable:
		return Decision{Action: ActionCordon, Phase: PhaseCordoned, Message: "cordoning the node", RequeueAfter: time.Second}
	case o.PendingPods > 0:
		return Decision{Action: ActionDrain, Phase: PhaseDraining, Message: "evicting pods", RequeueAfter: 10 * time.Second}
	default:
		return Decision{Action: ActionNone, Phase: PhaseDrained, Message: "node drained, ready for maintenance"}
	}
}
