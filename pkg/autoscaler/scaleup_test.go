package autoscaler

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	machinev1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRemoveNodePair(t *testing.T) {
	tests := []struct {
		input           []corev1.Node
		expectPair      []corev1.Node
		expectRemaining []corev1.Node
		expectFound     bool
	}{
		{
			input:           []corev1.Node{n("n1", "a"), n("n2", "b"), n("n3", "a"), n("n4", "a")},
			expectPair:      []corev1.Node{n("n1", "a"), n("n2", "b")},
			expectRemaining: []corev1.Node{n("n3", "a"), n("n4", "a")},
			expectFound:     true,
		},
		{
			input:           []corev1.Node{n("n1", "a"), n("n2", "a"), n("n3", "a"), n("n4", "a")},
			expectPair:      nil,
			expectRemaining: []corev1.Node{n("n1", "a"), n("n2", "a"), n("n3", "a"), n("n4", "a")},
			expectFound:     false,
		},
		{
			input:           []corev1.Node{n("n1", "a"), n("n2", "a"), n("n3", "a"), n("n4", "c")},
			expectPair:      []corev1.Node{n("n1", "a"), n("n4", "c")},
			expectRemaining: []corev1.Node{n("n2", "a"), n("n3", "a")},
			expectFound:     true,
		},
		{
			input:           []corev1.Node{n("n1", "a"), n("n2", "b"), n("n3", "c"), n("n4", "d")},
			expectPair:      []corev1.Node{n("n1", "a"), n("n2", "b")},
			expectRemaining: []corev1.Node{n("n3", "c"), n("n4", "d")},
			expectFound:     true,
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test-%d", i), func(t *testing.T) {
			g := NewGomegaWithT(t)
			remaining, pair, found := removeNodePair(tt.input)
			g.Expect(found).To(Equal(tt.expectFound))
			g.Expect(remaining).To(Equal(tt.expectRemaining))
			g.Expect(pair).To(Equal(tt.expectPair))
		})
	}
}

func TestPairNodes(t *testing.T) {

	tests := []struct {
		input    []corev1.Node
		expect   int
		pairs    [][]corev1.Node
		unpaired []corev1.Node
	}{
		{
			input:    []corev1.Node{n("n1", "a"), n("n2", "b"), n("n3", "a"), n("n4", "a")},
			expect:   1,
			pairs:    [][]corev1.Node{{n("n1", "a"), n("n2", "b")}},
			unpaired: []corev1.Node{n("n3", "a"), n("n4", "a")},
		},
		{
			input:    []corev1.Node{n("n1", "a"), n("n2", "b"), n("n3", "a"), n("n4", "c")},
			expect:   2,
			pairs:    [][]corev1.Node{{n("n1", "a"), n("n2", "b")}, {n("n3", "a"), n("n4", "c")}},
			unpaired: []corev1.Node{},
		},
		{
			input:    []corev1.Node{n("n1", "a"), n("n2", "a"), n("n3", "b"), n("n4", "c"), n("n5", "d"), n("n6", "c")},
			expect:   3,
			pairs:    [][]corev1.Node{{n("n1", "a"), n("n3", "b")}, {n("n2", "a"), n("n4", "c")}, {n("n5", "d"), n("n6", "c")}},
			unpaired: []corev1.Node{},
		},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("test-%d", i), func(t *testing.T) {
			g := NewGomegaWithT(t)
			count, pairs, unpaired := pairNodes(tt.input)
			g.Expect(count).To(Equal(tt.expect))
			g.Expect(unpaired).To(Equal(tt.unpaired))
			g.Expect(pairs).To(Equal(tt.pairs))
		})
	}
}

func n(name, zone string) corev1.Node {
	node := corev1.Node{}
	node.Name = name
	node.Labels = map[string]string{
		ZoneLabel: zone,
	}
	return node
}

func ms(name string) machinev1.MachineSet {
	ms := machinev1.MachineSet{}
	ms.Name = name
	return ms
}

func TestScaleUpReconciler_Reconcile(t *testing.T) {
	type fields struct {
		Client                  client.Client
		Interval                time.Duration
		MinWarm                 int
		listNodesHandler        func(ctx context.Context) (*corev1.NodeList, error)
		updateMachineSetHandler func(ctx context.Context, machineSet *machinev1.MachineSet) error
		listMachineSetHandler   func(ctx context.Context) (*machinev1.MachineSetList, error)
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "error listing nodes",
			fields: fields{
				Interval: time.Second,
				listNodesHandler: func(ctx context.Context) (*corev1.NodeList, error) {
					return nil, fmt.Errorf("Hey look Ma! An error!")
				},
			},
			args:    args{ctx: context.TODO()},
			wantErr: true,
		},
		{
			name: "Nothing to do since MinWarm is 2",
			fields: fields{
				Interval: time.Second,
				listNodesHandler: func(ctx context.Context) (*corev1.NodeList, error) {
					return &corev1.NodeList{
						TypeMeta: metav1.TypeMeta{Kind: "NodeList"},
						Items: []corev1.Node{
							newNode("node1", "zone-a1", "cluster-1", "openshift-machine-api/machine-A1", nHoursAgo(9)),
							newNode("node2", "zone-b1", "cluster-1", "openshift-machine-api/machine-B2", nHoursAgo(8)),
							newNode("node3", "zone-a1", "cluster-2", "openshift-machine-api/machine-A3", nHoursAgo(7)),
							newNode("node4", "zone-b1", "cluster-2", "openshift-machine-api/machine-B4", nHoursAgo(7)),
							newNode("node5", "zone-a1", noClusterID, "openshift-machine-api/machine-A5", nHoursAgo(7)),
							newNode("node6", "zone-b1", noClusterID, "openshift-machine-api/machine-B6", nHoursAgo(7)),
							newNode("node7", "zone-a1", noClusterID, "openshift-machine-api/machine-A7", nHoursAgo(7)),
							newNode("node8", "zone-b1", noClusterID, "openshift-machine-api/machine-B8", nHoursAgo(7)),
						},
					}, nil
				},
				MinWarm: 2,
			},
			args:    args{ctx: context.TODO()},
			wantErr: false,
		},
		{
			name: "Error listing machineSets",
			fields: fields{
				Interval: time.Second,
				listNodesHandler: func(ctx context.Context) (*corev1.NodeList, error) {
					return &corev1.NodeList{
						TypeMeta: metav1.TypeMeta{Kind: "NodeList"},
						Items: []corev1.Node{
							newNode("node1", "zone-a1", "cluster-1", "openshift-machine-api/machine-A1", nHoursAgo(9)),
							newNode("node2", "zone-b1", "cluster-1", "openshift-machine-api/machine-B2", nHoursAgo(8)),
							newNode("node3", "zone-a1", "cluster-2", "openshift-machine-api/machine-A3", nHoursAgo(7)),
							newNode("node4", "zone-b1", "cluster-2", "openshift-machine-api/machine-B4", nHoursAgo(7)),
						},
					}, nil
				},
				listMachineSetHandler: func(ctx context.Context) (*machinev1.MachineSetList, error) {
					return nil, fmt.Errorf("Hey look Ma! An error!")
				},
				MinWarm: 2,
			},
			args:    args{ctx: context.TODO()},
			wantErr: true,
		},

		{
			name: "Sucessfully scale up 2",
			fields: fields{
				Interval: time.Second,
				listNodesHandler: func(ctx context.Context) (*corev1.NodeList, error) {
					return &corev1.NodeList{
						TypeMeta: metav1.TypeMeta{Kind: "NodeList"},
						Items: []corev1.Node{
							newNode("node1", "zone-a1", "cluster-1", "openshift-machine-api/machine-A1", nHoursAgo(9)),
							newNode("node2", "zone-b1", "cluster-1", "openshift-machine-api/machine-B2", nHoursAgo(8)),
							newNode("node3", "zone-a1", "cluster-2", "openshift-machine-api/machine-A3", nHoursAgo(7)),
							newNode("node4", "zone-b1", "cluster-2", "openshift-machine-api/machine-B4", nHoursAgo(7)),
						},
					}, nil
				},
				listMachineSetHandler: func(ctx context.Context) (*machinev1.MachineSetList, error) {
					return &machinev1.MachineSetList{
						TypeMeta: metav1.TypeMeta{Kind: "MachineSetList"},
						Items: []machinev1.MachineSet{
							newMachineSet("machineSet-1a-1", desiredReplicas1, "zone-1a", availableReplicas1),
							newMachineSet("machineSet-1b-1", desiredReplicas1, "zone-1b", availableReplicas1),
							newMachineSet("machineSet-1a-2", desiredReplicas1, "zone-1a", availableReplicas1),
							newMachineSet("machineSet-1b-2", desiredReplicas1, "zone-1b", availableReplicas1),
							newMachineSet("machineSet-1a-3", desiredReplicas0, "zone-1a", availableReplicas0),
							newMachineSet("machineSet-1b-3", desiredReplicas0, "zone-1b", availableReplicas0),
							newMachineSet("machineSet-1a-4", desiredReplicas0, "zone-1a", availableReplicas0),
							newMachineSet("machineSet-1b-4", desiredReplicas0, "zone-1b", availableReplicas0),
							newMachineSet("machineSet-1a-5", desiredReplicas0, "zone-1a", availableReplicas0),
							newMachineSet("machineSet-1b-5", desiredReplicas0, "zone-1b", availableReplicas0),
						},
					}, nil
				},
				updateMachineSetHandler: func(ctx context.Context, machineSet *machinev1.MachineSet) error {
					switch machineSet.Name {
					case "machineSet-1a-3", "machineSet-1b-3": // TODO: being sure all are scaled up
						return nil
					case "machineSet-1a-4", "machineSet-1b-4":
						return nil
					}
					return fmt.Errorf("Hey dude you're scaling the wrong machineSet. %s", machineSet.Name)
				},
				MinWarm: 2,
			},
			args:    args{ctx: context.TODO()},
			wantErr: false,
		},

		{
			name: "Remove unapaired nodes - scaleUp machineSet-1a-5 and machineset-1b-5",
			fields: fields{
				Interval: time.Second,
				listNodesHandler: func(ctx context.Context) (*corev1.NodeList, error) {
					return &corev1.NodeList{
						TypeMeta: metav1.TypeMeta{Kind: "NodeList"},
						Items: []corev1.Node{
							newNode("node01", "zone-1a", noClusterID, "openshift-machine-api/machine-A1", nHoursAgo(9)),
							newNode("node02", "zone-1b", noClusterID, "openshift-machine-api/machine-A1", nHoursAgo(9)),
							newNode("node1", "zone-1a", "cluster-1", "openshift-machine-api/machine-A1", nHoursAgo(9)),
							newNode("node2", "zone-1b", "cluster-1", "openshift-machine-api/machine-B2", nHoursAgo(8)),
							newNode("node3", "zone-1a", "cluster-2", "openshift-machine-api/machine-A3", nHoursAgo(7)),
							newNode("node4", "zone-1b", "cluster-2", "openshift-machine-api/machine-B4", nHoursAgo(7)),
						},
					}, nil
				},
				listMachineSetHandler: func(ctx context.Context) (*machinev1.MachineSetList, error) {
					return &machinev1.MachineSetList{
						TypeMeta: metav1.TypeMeta{Kind: "MachineSetList"},
						Items: []machinev1.MachineSet{
							newMachineSet("machineSet-1a-1", desiredReplicas1, "zone-1a", availableReplicas1),
							newMachineSet("machineSet-1b-1", desiredReplicas1, "zone-1b", availableReplicas1),
							newMachineSet("machineSet-1a-2", desiredReplicas1, "zone-1a", availableReplicas1),
							newMachineSet("machineSet-1b-2", desiredReplicas1, "zone-1b", availableReplicas1),
							newMachineSet("machineSet-1a-3", desiredReplicas0, "zone-1a", availableReplicas0),
							newMachineSet("machineSet-1b-3", desiredReplicas0, "zone-1b", availableReplicas0),
							newMachineSet("machineSet-1a-4", desiredReplicas0, "zone-1a", availableReplicas0),
							newMachineSet("machineSet-1b-4", desiredReplicas0, "zone-1b", availableReplicas0),
							newMachineSet("machineSet-1a-5", desiredReplicas1, "zone-1a", availableReplicas0),
							newMachineSet("machineSet-1b-5", desiredReplicas1, "zone-1b", availableReplicas0),
						},
					}, nil
				},
				updateMachineSetHandler: func(ctx context.Context, machineSet *machinev1.MachineSet) error {
					switch machineSet.Name {
					case "machineSet-1a-5", "machineSet-1b-5": // TODO: being sure those are really scaled up
						return nil
					}
					return fmt.Errorf("Hey dude you're scaling the wrong machineSet. %s", machineSet.Name)
				},
				MinWarm: 2,
			},
			args:    args{ctx: context.TODO()},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ScaleUpReconciler{
				Client:                  tt.fields.Client,
				Interval:                tt.fields.Interval,
				MinWarm:                 tt.fields.MinWarm,
				listNodesHandler:        tt.fields.listNodesHandler,
				updateMachineSetHandler: tt.fields.updateMachineSetHandler,
				listMachineSetHandler:   tt.fields.listMachineSetHandler,
			}
			if err := r.Reconcile(tt.args.ctx); (err != nil) != tt.wantErr {
				t.Errorf("ScaleUpReconciler.Reconcile() test %s error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		},
		)
	}
}
