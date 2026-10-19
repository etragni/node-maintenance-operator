// Package controller wires the pure decision logic to the Kubernetes API.
package controller

import (
	"context"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/drain"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/example/nodemaintenance-operator/api/v1alpha1"
	"github.com/example/nodemaintenance-operator/internal/maintenance"
)

const (
	finalizerName     = "maintenance.example.com/uncordon"
	controlPlaneLabel = "node-role.kubernetes.io/control-plane"
)

// NodeMaintenanceReconciler reconciles NodeMaintenance objects.
type NodeMaintenanceReconciler struct {
	client.Client
	Clientset kubernetes.Interface
}

// +kubebuilder:rbac:groups=maintenance.example.com,resources=nodemaintenances,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=maintenance.example.com,resources=nodemaintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=maintenance.example.com,resources=nodemaintenances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch

// Reconcile observes the world, asks the pure Decide function what to do,
// executes that single action and reports the outcome in status.
func (r *NodeMaintenanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var nm v1alpha1.NodeMaintenance
	if err := r.Get(ctx, req.NamespacedName, &nm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// The finalizer guarantees we get a chance to uncordon before deletion.
	if nm.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(&nm, finalizerName) {
		if err := r.Update(ctx, &nm); err != nil {
			return ctrl.Result{}, err
		}
	}

	helper := r.newDrainHelper(ctx)
	obs, node, err := r.observe(ctx, &nm, helper)
	if err != nil {
		r.reportError(ctx, &nm, err)
		return ctrl.Result{}, err
	}

	decision := maintenance.Decide(obs)
	logger.Info("decision", "node", nm.Spec.NodeName, "action", decision.Action, "phase", decision.Phase)

	if err := r.execute(ctx, &nm, node, helper, decision); err != nil {
		r.reportError(ctx, &nm, err)
		return ctrl.Result{}, err // the workqueue retries with exponential backoff
	}
	if decision.Action == maintenance.ActionRelease {
		return ctrl.Result{}, nil // the object is going away, nothing left to report
	}

	if err := r.updateStatus(ctx, &nm, decision, obs.PendingPods); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: decision.RequeueAfter}, nil
}

// observe builds the Observation consumed by the pure decision function.
func (r *NodeMaintenanceReconciler) observe(ctx context.Context, nm *v1alpha1.NodeMaintenance, h *drain.Helper) (maintenance.Observation, *corev1.Node, error) {
	obs := maintenance.Observation{
		Deleting: !nm.DeletionTimestamp.IsZero(),
		Acted:    hasActed(nm.Status.Phase),
	}

	var node corev1.Node
	err := r.Get(ctx, types.NamespacedName{Name: nm.Spec.NodeName}, &node)
	if apierrors.IsNotFound(err) {
		return obs, nil, nil
	}
	if err != nil {
		return obs, nil, err
	}
	obs.NodeFound = true
	_, obs.NodeIsControlPlane = node.Labels[controlPlaneLabel]
	obs.NodeUnschedulable = node.Spec.Unschedulable

	if obs.DuplicateRequest, err = r.isDuplicate(ctx, nm); err != nil {
		return obs, &node, err
	}

	if obs.NodeUnschedulable && !obs.Deleting && !obs.DuplicateRequest && !obs.NodeIsControlPlane {
		pods, errs := h.GetPodsForDeletion(node.Name)
		if len(errs) > 0 {
			return obs, &node, utilerrors.NewAggregate(errs)
		}
		obs.PendingPods = len(pods.Pods())
	}
	return obs, &node, nil
}

// isDuplicate reports whether an older, live NodeMaintenance targets the same node.
func (r *NodeMaintenanceReconciler) isDuplicate(ctx context.Context, nm *v1alpha1.NodeMaintenance) (bool, error) {
	var all v1alpha1.NodeMaintenanceList
	if err := r.List(ctx, &all); err != nil {
		return false, err
	}
	for i := range all.Items {
		other := &all.Items[i]
		if other.Name == nm.Name || other.Spec.NodeName != nm.Spec.NodeName || !other.DeletionTimestamp.IsZero() {
			continue
		}
		if other.CreationTimestamp.Before(&nm.CreationTimestamp) {
			return true, nil
		}
		if other.CreationTimestamp.Equal(&nm.CreationTimestamp) && other.Name < nm.Name {
			return true, nil
		}
	}
	return false, nil
}

// execute performs the single side effect chosen by Decide.
func (r *NodeMaintenanceReconciler) execute(ctx context.Context, nm *v1alpha1.NodeMaintenance, node *corev1.Node, h *drain.Helper, d maintenance.Decision) error {
	switch d.Action {
	case maintenance.ActionCordon:
		return drain.RunCordonOrUncordon(h, node.DeepCopy(), true)
	case maintenance.ActionDrain:
		return drain.RunNodeDrain(h, node.Name)
	case maintenance.ActionUncordon:
		return drain.RunCordonOrUncordon(h, node.DeepCopy(), false)
	case maintenance.ActionRelease:
		if controllerutil.RemoveFinalizer(nm, finalizerName) {
			return r.Update(ctx, nm)
		}
	}
	return nil
}

// updateStatus writes status only when it changed, to avoid useless events.
func (r *NodeMaintenanceReconciler) updateStatus(ctx context.Context, nm *v1alpha1.NodeMaintenance, d maintenance.Decision, pending int) error {
	desired := v1alpha1.NodeMaintenanceStatus{
		Phase:              string(d.Phase),
		Message:            d.Message,
		PendingPods:        pending,
		ObservedGeneration: nm.Generation,
	}
	if nm.Status == desired {
		return nil
	}
	nm.Status = desired
	return r.Status().Update(ctx, nm)
}

// reportError records an error message in status on a best-effort basis.
func (r *NodeMaintenanceReconciler) reportError(ctx context.Context, nm *v1alpha1.NodeMaintenance, err error) {
	if nm.Status.Message == err.Error() {
		return
	}
	nm.Status.Message = err.Error()
	_ = r.Status().Update(ctx, nm)
}

func (r *NodeMaintenanceReconciler) newDrainHelper(ctx context.Context) *drain.Helper {
	return &drain.Helper{
		Ctx:                 ctx,
		Client:              r.Clientset,
		IgnoreAllDaemonSets: true,
		DeleteEmptyDirData:  false, // safe default: never lose emptyDir data silently
		GracePeriodSeconds:  -1,    // use each pod's own terminationGracePeriodSeconds
		Timeout:             30 * time.Second,
		Out:                 io.Discard,
		ErrOut:              io.Discard,
	}
}

func hasActed(phase string) bool {
	switch maintenance.Phase(phase) {
	case maintenance.PhaseCordoned, maintenance.PhaseDraining, maintenance.PhaseDrained:
		return true
	}
	return false
}

// SetupWithManager registers the controller. Node events are mapped to the
// NodeMaintenance objects that target that node, so a manual uncordon is noticed.
func (r *NodeMaintenanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NodeMaintenance{}).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(r.nodeToRequests)).
		Complete(r)
}

func (r *NodeMaintenanceReconciler) nodeToRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	var list v1alpha1.NodeMaintenanceList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for _, nm := range list.Items {
		if nm.Spec.NodeName == obj.GetName() {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: nm.Name}})
		}
	}
	return reqs
}
