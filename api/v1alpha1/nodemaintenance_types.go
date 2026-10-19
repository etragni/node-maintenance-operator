package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// NodeMaintenanceSpec is the desired state: which node must be put in maintenance.
type NodeMaintenanceSpec struct {
	// NodeName is the name of the worker node to put in maintenance.
	// +kubebuilder:validation:MinLength=1
	NodeName string `json:"nodeName"`

	// Reason is a human readable justification, kept for auditing.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// NodeMaintenanceStatus is the observed state, written only by the controller.
type NodeMaintenanceStatus struct {
	// Phase is the current lifecycle phase.
	// +kubebuilder:validation:Enum=Cordoned;Draining;Drained;Failed;Releasing
	// +optional
	Phase string `json:"phase,omitempty"`

	// Message gives details about the current phase.
	// +optional
	Message string `json:"message,omitempty"`

	// PendingPods is the number of pods still to be evicted from the node.
	// +optional
	PendingPods int `json:"pendingPods,omitempty"`

	// ObservedGeneration is the last spec generation processed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// NodeMaintenance requests a declarative cordon and drain of a node.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=nm
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Pending",type=integer,JSONPath=`.status.pendingPods`
type NodeMaintenance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeMaintenanceSpec   `json:"spec,omitempty"`
	Status NodeMaintenanceStatus `json:"status,omitempty"`
}

// NodeMaintenanceList contains a list of NodeMaintenance.
// +kubebuilder:object:root=true
type NodeMaintenanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeMaintenance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeMaintenance{}, &NodeMaintenanceList{})
}
