package autoscaler

import (
	"context"
	"fmt"
	"testing"
	"time"

	machinev1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	noClusterID string = ""
	zoneA       string = "zone-A"
	zoneB       string = "zone-B"
	noMachine   string = ""
)

func nHoursAgo(n int) metav1.Time {
	now := time.Now()
	return metav1.NewTime(now.Add(time.Duration(-n) * time.Hour))
}

func newNode(name, zone, clusterID, machineAnnotationValue string, creationTimeStamp metav1.Time) corev1.Node {
	n := corev1.Node{
		TypeMeta: metav1.TypeMeta{Kind: "Node"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			UID:               uuid.NewUUID(),
			Labels:            make(map[string]string),
			Annotations:       make(map[string]string),
			CreationTimestamp: creationTimeStamp,
		},
		Spec:   corev1.NodeSpec{},
		Status: corev1.NodeStatus{},
	}
	if len(zone) > 0 {
		n.Labels[ZoneLabel] = zone
	}
	if len(clusterID) > 0 {
		n.Labels[HostedClusterLabel] = clusterID
	}
	if len(machineAnnotationValue) > 0 {
		n.Annotations[machineAnnotation] = machineAnnotationValue
	}
	return n
}

func newMachine(name, machineSetLabelValue string) machinev1.Machine {
	m := machinev1.Machine{
		TypeMeta: metav1.TypeMeta{Kind: "Machine"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: make(map[string]string),
		},
	}
	if len(machineSetLabelValue) > 0 {
		m.Labels[machineSetLabel] = machineSetLabelValue
	}
	return m
}

func newMachineSet(name string) machinev1.MachineSet {
	return machinev1.MachineSet{
		TypeMeta: metav1.TypeMeta{Kind: "MachineSet"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: machinev1.MachineSetSpec{
			Replicas: pointer.Int32(1),
		},
	}
}

func TestScaleDownReconciler_Reconcile(t *testing.T) {
	type fields struct {
		Client                  client.Client
		Interval                time.Duration
		MaxWarm                 int
		listNodesHandler        func(ctx context.Context) (*corev1.NodeList, error)
		updateMachineSetHandler func(ctx context.Context, machineSet *machinev1.MachineSet) error
		listMachineSetHandler   func(ctx context.Context) (*machinev1.MachineSetList, error)
		listMachinesHandler     func(ctx context.Context) (*machinev1.MachineList, error)
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
			name: "Error listing machines",
			fields: fields{
				Interval: time.Second,
				listNodesHandler: func(ctx context.Context) (*corev1.NodeList, error) {
					return &corev1.NodeList{
						TypeMeta: metav1.TypeMeta{Kind: "NodeList"},
						Items: []corev1.Node{
							newNode("node5", zoneA, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node6", zoneB, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node7", zoneA, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node8", zoneB, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node9", zoneA, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node10", zoneB, noClusterID, noMachine, nHoursAgo(10)),
						},
					}, nil
				},
				listMachinesHandler: func(ctx context.Context) (*machinev1.MachineList, error) {
					return nil, fmt.Errorf("Hey look Ma! An error!")
				},
				MaxWarm: 2,
			},
			args:    args{ctx: context.TODO()},
			wantErr: true,
		},
		{
			name: "Error listing machineSets",
			fields: fields{
				Interval: time.Second,
				listNodesHandler: func(ctx context.Context) (*corev1.NodeList, error) {
					return &corev1.NodeList{
						TypeMeta: metav1.TypeMeta{Kind: "NodeList"},
						Items: []corev1.Node{
							newNode("node5", zoneA, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node6", zoneB, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node7", zoneA, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node8", zoneB, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node9", zoneA, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node10", zoneB, noClusterID, noMachine, nHoursAgo(10)),
						},
					}, nil
				},
				listMachinesHandler: func(ctx context.Context) (*machinev1.MachineList, error) {
					return &machinev1.MachineList{}, nil // no machines needed
				},
				listMachineSetHandler: func(ctx context.Context) (*machinev1.MachineSetList, error) {
					return nil, fmt.Errorf("Hey look Ma! An error!")
				},
				MaxWarm: 2,
			},
			args:    args{ctx: context.TODO()},
			wantErr: true,
		},
		{
			name: "Nothing to do",
			fields: fields{
				Interval: time.Second,
				listNodesHandler: func(ctx context.Context) (*corev1.NodeList, error) {
					return &corev1.NodeList{
						TypeMeta: metav1.TypeMeta{Kind: "NodeList"},
						Items: []corev1.Node{
							newNode("node1", zoneA, "cluster-1", noMachine, nHoursAgo(10)),
							newNode("node2", zoneB, "cluster-1", noMachine, nHoursAgo(10)),
							newNode("node3", zoneA, "cluster-2", noMachine, nHoursAgo(10)),
							newNode("node4", zoneB, "cluster-2", noMachine, nHoursAgo(10)),
							newNode("node5", zoneA, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node6", zoneB, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node7", zoneA, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node8", zoneB, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node9", zoneA, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node10", zoneB, noClusterID, noMachine, nHoursAgo(10)),
						},
					}, nil
				},
				MaxWarm: 3,
			},
			args:    args{ctx: context.TODO()},
			wantErr: false,
		},
		{
			name: "Remove one pair",
			fields: fields{
				Interval: time.Second,
				listNodesHandler: func(ctx context.Context) (*corev1.NodeList, error) {
					return &corev1.NodeList{
						TypeMeta: metav1.TypeMeta{Kind: "NodeList"},
						Items: []corev1.Node{
							newNode("node1", zoneA, "cluster-1", noMachine, nHoursAgo(9)),
							newNode("node2", zoneB, "cluster-1", noMachine, nHoursAgo(8)),
							newNode("node3", zoneA, "cluster-2", noMachine, nHoursAgo(7)),
							newNode("node4", zoneB, "cluster-2", noMachine, nHoursAgo(7)),
							newNode("node5", zoneA, noClusterID, noMachine, nHoursAgo(10)),
							newNode("node6", zoneB, noClusterID, noMachine, nHoursAgo(11)),
							newNode("node7", zoneA, noClusterID, "openshift-machine-api/machine-A7", nHoursAgo(12)),
							newNode("node8", zoneB, noClusterID, "openshift-machine-api/machine-B8", nHoursAgo(13)),
							newNode("node9", zoneA, noClusterID, "openshift-machine-api/machine-A9", nHoursAgo(14)),
							newNode("node10", zoneB, noClusterID, "openshift-machine-api/machine-B10", nHoursAgo(15)),
						},
					}, nil
				},
				listMachinesHandler: func(ctx context.Context) (*machinev1.MachineList, error) {
					return &machinev1.MachineList{
						TypeMeta: metav1.TypeMeta{Kind: "MachineList"},
						Items: []machinev1.Machine{
							newMachine("machine-A1", "machineSet-A1"),
							newMachine("machine-B2", "machineSet-B2"),
							newMachine("machine-A3", "machineSet-A3"),
							newMachine("machine-B4", "machineSet-B4"),
							newMachine("machine-A5", "machineSet-A5"),
							newMachine("machine-B6", "machineSet-B6"),
							newMachine("machine-A7", "machineSet-A7"),
							newMachine("machine-B8", "machineSet-B8"),
							newMachine("machine-A9", "machineSet-A9"),
							newMachine("machine-B10", "machineSet-B10"),
						},
					}, nil
				},
				listMachineSetHandler: func(ctx context.Context) (*machinev1.MachineSetList, error) {
					return &machinev1.MachineSetList{
						TypeMeta: metav1.TypeMeta{Kind: "MachineSetList"},
						Items: []machinev1.MachineSet{
							newMachineSet("machineSet-A1"),
							newMachineSet("machineSet-B2"),
							newMachineSet("machineSet-A3"),
							newMachineSet("machineSet-B4"),
							newMachineSet("machineSet-A5"),
							newMachineSet("machineSet-B6"),
							newMachineSet("machineSet-A7"),
							newMachineSet("machineSet-B8"),
							newMachineSet("machineSet-A9"),
							newMachineSet("machineSet-B10"),
						},
					}, nil
				},
				updateMachineSetHandler: func(ctx context.Context, machineSet *machinev1.MachineSet) error {
					switch machineSet.Name {
					case "machineSet-B10":
						return nil
					case "machineSet-A9":
						return nil
					}
					return fmt.Errorf("Hey dude you're scaling the wrong machineSet")
				},
				MaxWarm: 2,
			},
			args:    args{ctx: context.TODO()},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ScaleDownReconciler{
				Client:                  tt.fields.Client,
				Interval:                tt.fields.Interval,
				MaxWarm:                 tt.fields.MaxWarm,
				listNodesHandler:        tt.fields.listNodesHandler,
				updateMachineSetHandler: tt.fields.updateMachineSetHandler,
				listMachineSetHandler:   tt.fields.listMachineSetHandler,
				listMachinesHandler:     tt.fields.listMachinesHandler,
			}
			if err := r.Reconcile(tt.args.ctx); (err != nil) != tt.wantErr {
				t.Errorf("ScaleDownReconciler.Reconcile() test %s error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}
