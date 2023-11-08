package autoscaler

import (
	"context"
	"fmt"
	"strings"
	"time"

	machinev1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	RequestServingComponentLabel = "hypershift.openshift.io/request-serving-component"
	ZoneLabel                    = "topology.kubernetes.io/zone"
	HostedClusterLabel           = "hypershift.openshift.io/cluster"
)

type ScaleUpReconciler struct {
	client.Client
	Interval time.Duration
	MinWarm  int
	// exposed for testing purpose
	listNodesHandler        func(ctx context.Context) (*corev1.NodeList, error)
	updateMachineSetHandler func(ctx context.Context, machineSet *machinev1.MachineSet) error
	listMachineSetHandler   func(ctx context.Context) (*machinev1.MachineSetList, error)
}

func (r *ScaleUpReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return mgr.Add(manager.RunnableFunc(r.Run))
}

func (r *ScaleUpReconciler) Run(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx, "Controller", "ScaleUp")
	ctx = ctrl.LoggerInto(ctx, log)
	log.Info("Starting reconcile", "interval", r.Interval, "minimum warm", r.MinWarm)
	return wait.PollUntilContextCancel(ctx, r.Interval, true, r.sync)
}

func (r *ScaleUpReconciler) listServingComponentNodes(ctx context.Context) (*corev1.NodeList, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{RequestServingComponentLabel: "true"}); err != nil {
		return &corev1.NodeList{}, fmt.Errorf("failed to get node list: %w", err)
	}
	return nodeList, nil
}

func (r *ScaleUpReconciler) updateMachineSet(ctx context.Context, machineSet *machinev1.MachineSet) error {
	return r.Update(ctx, machineSet)
}

func (r *ScaleUpReconciler) dryModeUpdateMachineSet(ctx context.Context, machineSet *machinev1.MachineSet) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("I would love to scale up ", "name", machineSet.Name)
	return nil
}

func (r *ScaleUpReconciler) listMachineSet(ctx context.Context) (*machinev1.MachineSetList, error) {
	machineSetList := &machinev1.MachineSetList{}
	if err := r.List(ctx, machineSetList); err != nil {
		return &machinev1.MachineSetList{}, fmt.Errorf("failed to list machinesets: %w", err)
	}
	return machineSetList, nil
}

func (r *ScaleUpReconciler) sync(ctx context.Context) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	if err := r.Reconcile(ctx); err != nil {
		log.Error(err, "Reconcile error")
	}
	return false, nil
}

func (r *ScaleUpReconciler) Reconcile(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	nodeList, err := r.listNodesHandler(ctx)
	if err != nil {
		return err
	}
	log.Info("Request serving nodes in cluster", "count", len(nodeList.Items))

	availableNodes := filterNodes(nodeList.Items, isAvailableNode)
	log.Info("Request serving nodes in cluster that have not been assigned to a HostedCluster", "count", len(availableNodes))

	availableNodePairs, _, unpairedNodes := pairNodes(availableNodes)
	log.Info("Available request serving node pairs", "count", availableNodePairs)
	if availableNodePairs < r.MinWarm {
		log.Info("Warm node pair count is less than minimum, will scale up", "pairs needed", r.MinWarm-availableNodePairs)
		if err := r.scaleUp(ctx, r.MinWarm-availableNodePairs, unpairedNodes); err != nil {
			return fmt.Errorf("failed to scale up: %w", err)
		}
	}
	return nil
}

func (r *ScaleUpReconciler) scaleUp(ctx context.Context, pairsNeeded int, unpairedNodes []corev1.Node) error {
	log := ctrl.LoggerFrom(ctx)

	machineSetList, err := r.listMachineSetHandler(ctx)
	if err != nil {
		return err
	}

	requestServingMachineSets := filterMachineSets(machineSetList.Items, requestServingComponentMachineSet)
	log.Info("Request serving machinesets", "count", len(requestServingMachineSets))

	// machineSetsWithNoNodes are machine sets that do not yet have a corresponding node available (they're available to be scaled up)
	machineSetsWithNoNodes := filterMachineSets(requestServingMachineSets, availableMachineSet)
	log.Info("Machinesets with no nodes", "count", len(machineSetsWithNoNodes))

	// inProgressMachineSets is the subset of machinesets without nodes but have a spec
	// of replicas==1. These are machinesets that are in progress.
	inProgressMachineSets := filterMachineSets(machineSetsWithNoNodes, scaledUpMachineSet)
	log.Info("Machinesets with no nodes that are in progress", "count", len(inProgressMachineSets))

	// Using the machinesets that are in progress, determine which machinesets to scale up
	machineSetsToScaleUp := machineSetsToScaleUp(append(inProgressMachineSets, filterMachineSets(machineSetsWithNoNodes, notScaledUpMachineSet)...), unpairedNodes, pairsNeeded)

	log.Info("Machinesets to scale up", "count", len(machineSetsToScaleUp), "names", machineSetNames(machineSetsToScaleUp))
	for i := range machineSetsToScaleUp {
		machineSet := &machineSetsToScaleUp[i]
		machineSet.Spec.Replicas = pointer.Int32(1)
		if err := r.updateMachineSetHandler(ctx, machineSet); err != nil {
			return fmt.Errorf("failed to scale up machineset %s: %w", machineSet.Name, err)
		}
		log.Info("Scaled up machineset", "name", machineSet.Name)
	}
	return nil
}

func machineSetNames(machineSets []machinev1.MachineSet) []string {
	result := make([]string, 0, len(machineSets))
	for _, machineSet := range machineSets {
		result = append(result, machineSet.Name)
	}
	return result
}

func machineSetsToScaleUp(availablePool []machinev1.MachineSet, unpairedNodes []corev1.Node, pairsNeeded int) []machinev1.MachineSet {
	var result []machinev1.MachineSet

	// Find matches for unpaired nodes among available machineSets,
	// adding those machineSets to the list to scale up
	for len(unpairedNodes) > 0 && pairsNeeded > 0 {
		node := &unpairedNodes[0]
		foundMatch := false
		for i, ms := range availablePool {
			if !strings.HasSuffix(ms.Name, nodeZone(node)) {
				foundMatch = true
				unpairedNodes = unpairedNodes[1:]
				result = append(result, ms)
				availablePool = append(availablePool[:i], availablePool[i+1:]...)
				pairsNeeded--
				break
			}
		}
		if !foundMatch {
			break
		}
	}

	// Finally find machineSet matches in the available pool
	for len(availablePool) > 0 && pairsNeeded > 0 {
		machineSet := &availablePool[0]
		foundMatch := false
		for i := range availablePool[1:] {
			if machineSetZone(&availablePool[1+i]) != machineSetZone(machineSet) {
				foundMatch = true
				result = append(result, *machineSet, availablePool[i+1])
				availablePool = append(availablePool[1:1+i], availablePool[i+2:]...)
				pairsNeeded--
				break
			}
		}
		if !foundMatch {
			break
		}
	}

	return result
}

// removeNodePair looks through a list of nodes and finds a pair with different
// zones. That pair of nodes is removed from the list
func removeNodePair(nodes []corev1.Node) (remainingNodes []corev1.Node, pair []corev1.Node, found bool) {
	remainingNodes = nodes
	if len(nodes) == 0 {
		return
	}

	first := nodes[0]
	for i := range nodes[1:] {
		if nodeZone(&nodes[1+i]) != nodeZone(&first) {
			pair = []corev1.Node{first, nodes[i+1]}
			remainingNodes = append(remainingNodes[1:1+i], remainingNodes[i+2:]...)
			found = true
			return
		}
	}

	return
}

// pairNodes finds node pairs (nodes with different zones) in the given
// list of nodes. Returns the number of pairs found and the list of nodes
// that could not be paired.
func pairNodes(nodes []corev1.Node) (pairCount int, paired [][]corev1.Node, unpaired []corev1.Node) {
	unpaired = nodes
	// Find how many pairs of nodes with distinct zones we have
	pairCount = 0
	for len(unpaired) > 0 {
		var found bool
		var pair []corev1.Node
		unpaired, pair, found = removeNodePair(unpaired)
		if !found {
			break
		}
		paired = append(paired, pair)
		pairCount++
	}
	return
}

func filterNodes(nodes []corev1.Node, filter func(*corev1.Node) bool) []corev1.Node {
	var result []corev1.Node
	for i := range nodes {
		if filter(&nodes[i]) {
			result = append(result, nodes[i])
		}
	}
	return result
}

// isAvailableNode returns true if the node
// does not have a hosted cluster label.
func isAvailableNode(node *corev1.Node) bool {
	for k := range node.Labels {
		if k == HostedClusterLabel {
			return false
		}
	}
	return true
}

func scaledUpMachineSet(machineSet *machinev1.MachineSet) bool {
	return machineSet.Spec.Replicas != nil && *machineSet.Spec.Replicas > 0
}

func notScaledUpMachineSet(machineSet *machinev1.MachineSet) bool {
	return !scaledUpMachineSet(machineSet)
}

func availableMachineSet(machineSet *machinev1.MachineSet) bool {
	return machineSet.Status.AvailableReplicas == 0
}

func requestServingComponentMachineSet(machineSet *machinev1.MachineSet) bool {
	for k, v := range machineSet.Spec.Template.Spec.ObjectMeta.Labels {
		if k == RequestServingComponentLabel && v == "true" {
			return true
		}
	}
	return false
}

func filterMachineSets(machineSets []machinev1.MachineSet, filter func(*machinev1.MachineSet) bool) []machinev1.MachineSet {
	var result []machinev1.MachineSet
	for i := range machineSets {
		if filter(&machineSets[i]) {
			result = append(result, machineSets[i])
		}
	}
	return result
}

func machineSetZone(machineSet *machinev1.MachineSet) string {
	providerSpec := &unstructured.Unstructured{}
	if _, _, err := unstructured.UnstructuredJSONScheme.Decode(machineSet.Spec.Template.Spec.ProviderSpec.Value.Raw, nil, providerSpec); err != nil {
		// TODO: swallowing error here
		return machineSet.Name[len(machineSet.Name)-5:] // TODO: what if the len(machineSet.Name)<5
	}
	zone, _, err := unstructured.NestedString(providerSpec.Object, "placement", "availabilityZone")
	if err != nil {
		// TODO: swallowing error here
		return machineSet.Name[len(machineSet.Name)-5:] // TODO: what if the len(machineSet.Name)<5
	}
	return zone
}

func nodeZone(n *corev1.Node) string {
	if n == nil {
		return ""
	}
	return n.Labels[ZoneLabel]
}
