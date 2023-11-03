package autoscaler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	machinev1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	machineAnnotation = "machine.openshift.io/machine"
	machineSetLabel   = "machine.openshift.io/cluster-api-machineset"
)

type ScaleDownReconciler struct {
	client.Client
	Interval time.Duration
	MaxWarm  int
	// exposed for testing purpose
	listNodesHandler        func(ctx context.Context) (*corev1.NodeList, error)
	updateMachineSetHandler func(ctx context.Context, machineSet *machinev1.MachineSet) error
	listMachineSetHandler   func(ctx context.Context) (*machinev1.MachineSetList, error)
	listMachinesHandler     func(ctx context.Context) (*machinev1.MachineList, error)
}

func (r *ScaleDownReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return mgr.Add(manager.RunnableFunc(r.Run))
}

func (r *ScaleDownReconciler) Run(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx, "Controller", "ScaleDown")
	ctx = ctrl.LoggerInto(ctx, log)
	log.Info("Starting reconcile", "interval", r.Interval, "maximum warm", r.MaxWarm)
	return wait.PollUntilContextCancel(ctx, r.Interval, true, r.sync)
}

func (r *ScaleDownReconciler) sync(ctx context.Context) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	if err := r.Reconcile(ctx); err != nil {
		log.Error(err, "Reconcile error")
	}
	return false, nil
}

type MachineNode struct {
	Zone    string
	Node    *corev1.Node
	Machine *machinev1.Machine
}

type nodesByCreationDate []corev1.Node

func (n nodesByCreationDate) Len() int      { return len(n) }
func (n nodesByCreationDate) Swap(i, j int) { n[i], n[j] = n[j], n[i] }
func (n nodesByCreationDate) Less(i, j int) bool {
	return n[i].CreationTimestamp.Before(&n[j].CreationTimestamp)
}

func (r *ScaleDownReconciler) listServingComponentNodes(ctx context.Context) (*corev1.NodeList, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{RequestServingComponentLabel: "true"}); err != nil {
		return &corev1.NodeList{}, fmt.Errorf("failed to get node list: %w", err)
	}
	return nodeList, nil
}

func (r *ScaleDownReconciler) updateMachineSet(ctx context.Context, machineSet *machinev1.MachineSet) error {
	return r.Update(ctx, machineSet)
}

func (r *ScaleDownReconciler) dryModeUpdateMachineSet(ctx context.Context, machineSet *machinev1.MachineSet) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("I would love to scale down ", "name", machineSet.Name)
	return nil
}

func (r *ScaleDownReconciler) listMachineSet(ctx context.Context) (*machinev1.MachineSetList, error) {
	machineSetList := &machinev1.MachineSetList{}
	if err := r.List(ctx, machineSetList); err != nil {
		return &machinev1.MachineSetList{}, fmt.Errorf("failed to list machinesets: %w", err)
	}
	return machineSetList, nil
}

func (r *ScaleDownReconciler) listMachines(ctx context.Context) (*machinev1.MachineList, error) {
	machineList := &machinev1.MachineList{}
	if err := r.List(ctx, machineList); err != nil {
		return &machinev1.MachineList{}, fmt.Errorf("failed to list machines: %w", err)
	}
	return machineList, nil
}

func (r *ScaleDownReconciler) Reconcile(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	nodeList, err := r.listNodesHandler(ctx)
	if err != nil {
		return err
	}
	log.Info("Request serving nodes in cluster", "count", len(nodeList.Items))

	availableNodes := filterNodes(nodeList.Items, isAvailableNode)
	log.Info("Available request serving nodes", "count", len(availableNodes))

	// Sort nodes by creation date so we remove the oldest nodes
	sort.Sort(nodesByCreationDate(availableNodes))

	pairCount, pairs, _ := pairNodes(availableNodes)
	log.Info("Available request serving node pairs", "count", pairCount, "pairs", len(pairs))
	if pairCount <= r.MaxWarm {
		log.Info("Node pairs do not exceed maximum. Nothing to do.")
		return nil
	}

	log.Info("There are more available node pairs than the maximum, will scale down")
	var nodesToScaleDown []corev1.Node
	for i := range pairs[:len(pairs)-r.MaxWarm] {
		nodesToScaleDown = append(nodesToScaleDown, pairs[i]...)
	}
	log.Info("Nodes to scale down", "nodes", nodeNames(nodesToScaleDown))

	machineList, err := r.listMachinesHandler(ctx)
	if err != nil {
		return err
	}

	machineSetList, err := r.listMachineSetHandler(ctx)
	if err != nil {
		return err
	}

	machineSetsToScaleDown := machineSetsForNodes(log, nodesToScaleDown, machineList.Items, machineSetList.Items)

	for i := range machineSetsToScaleDown {
		machineSet := &machineSetsToScaleDown[i]
		if machineSet.Spec.Replicas == nil || *machineSet.Spec.Replicas == 0 {
			// Machineset already scaled down
			log.Info("MachineSet already scaled down", "name", machineSet.Name)
			continue
		}
		machineSet.Spec.Replicas = pointer.Int32(0)
		if err := r.updateMachineSetHandler(ctx, machineSet); err != nil {
			return fmt.Errorf("failed to scale down machineset %s: %w", machineSet.Name, err)
		}
		log.Info("Scaled down machineset", "name", machineSet.Name)
	}

	return nil
}

func machineSetsForNodes(log logr.Logger, nodes []corev1.Node, machines []machinev1.Machine, machineSets []machinev1.MachineSet) []machinev1.MachineSet {
	machinesByName := map[string]*machinev1.Machine{}
	machineSetsByName := map[string]*machinev1.MachineSet{}

	var result []machinev1.MachineSet

	for i := range machines {
		machine := &machines[i]
		machinesByName[machine.Name] = machine
	}

	for i := range machineSets {
		machineSet := &machineSets[i]
		machineSetsByName[machineSet.Name] = machineSet
	}

	for _, node := range nodes {
		machineNameNamespace := node.Annotations[machineAnnotation]
		if len(machineNameNamespace) == 0 {
			log.Info("WARNING: no machine annotation found on node", "node", node.Name)
			continue
		}
		machineName := strings.SplitN(machineNameNamespace, "/", 2)[1]
		machine := machinesByName[machineName]
		if machine == nil {
			log.Info("WARNING: no machine found with name", "name", machineName)
			continue
		}
		machineSetName := machine.Labels[machineSetLabel]
		if len(machineSetName) == 0 {
			log.Info("WARNING: no machineset label found on machine", "name", machineName)
			continue
		}
		machineSet := machineSetsByName[machineSetName]
		if machineSet == nil {
			log.Info("WARNING: no machineset found with name", "name", machineSetName)
			continue
		}
		result = append(result, *machineSet)
	}

	return result
}

func nodeNames(nodes []corev1.Node) []string {
	var result = make([]string, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, node.Name)
	}
	return result
}
