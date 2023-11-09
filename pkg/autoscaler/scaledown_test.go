package autoscaler

import (
	"context"
	"fmt"
	"testing"
	"time"

	machinev1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	noClusterID        string = ""
	noMachine          string = ""
	desiredReplicas1   int32  = 1
	desiredReplicas0   int32  = 0
	availableReplicas1 int32  = 1
	availableReplicas0 int32  = 0
)

func nHoursAgo(n int) metav1.Time {
	now := time.Now()
	return metav1.NewTime(now.Add(time.Duration(-n) * time.Hour))
}

func newNode(name, zone, clusterID, machine, pairedNodesLabelValue string, creationTimeStamp metav1.Time) corev1.Node {
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
	if len(machine) > 0 {
		n.Annotations[machineAnnotation] = machine
	}
	if len(pairedNodesLabelValue) > 0 {
		n.Labels[PairedNodesLabelKey] = pairedNodesLabelValue
	}
	return n
}

func newMachine(name, machineSet string) machinev1.Machine {
	m := machinev1.Machine{
		TypeMeta: metav1.TypeMeta{Kind: "Machine"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: make(map[string]string),
		},
	}
	if len(machineSet) > 0 {
		m.Labels[machineSetLabel] = machineSet
	}
	return m
}

func newAWSMachineProviderConfigSerialized(region, az string) []byte {
	return []byte(fmt.Sprintf(`{"kind": "AWSMachineProviderConfig", 
	"placement": { 
	  "availabilityZone": "%s",
	  "region": "%s"
	}
  }`, az, region))
}

func newMachineSet(name string, desiredReplicas int32, az string, availableReplicas int32) machinev1.MachineSet {
	ms := machinev1.MachineSet{
		TypeMeta: metav1.TypeMeta{Kind: "MachineSet"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: machinev1.MachineSetSpec{
			Replicas: pointer.Int32(desiredReplicas),
			Template: machinev1.MachineTemplateSpec{
				Spec: machinev1.MachineSpec{
					ObjectMeta: machinev1.ObjectMeta{
						Labels: map[string]string{RequestServingComponentLabel: "true"},
					},
					ProviderSpec: machinev1.ProviderSpec{
						Value: &runtime.RawExtension{
							Raw: newAWSMachineProviderConfigSerialized("region", az),
						},
					},
				},
			},
		},
	}
	if availableReplicas > 0 {
		ms.Status.AvailableReplicas = availableReplicas
	}
	return ms
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
							newNode("node5", "zone-a1", noClusterID, noMachine, "serving-3", nHoursAgo(10)),
							newNode("node6", "zone-b1", noClusterID, noMachine, "serving-3", nHoursAgo(10)),
							newNode("node7", "zone-a1", noClusterID, noMachine, "serving-4", nHoursAgo(10)),
							newNode("node8", "zone-b1", noClusterID, noMachine, "serving-4", nHoursAgo(10)),
							newNode("node9", "zone-a1", noClusterID, noMachine, "serving-5", nHoursAgo(10)),
							newNode("node10", "zone-b1", noClusterID, noMachine, "serving-5", nHoursAgo(10)),
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
							newNode("node5", "zone-a1", noClusterID, noMachine, "serving-3", nHoursAgo(10)),
							newNode("node6", "zone-b1", noClusterID, noMachine, "serving-3", nHoursAgo(10)),
							newNode("node7", "zone-a1", noClusterID, noMachine, "serving-4", nHoursAgo(10)),
							newNode("node8", "zone-b1", noClusterID, noMachine, "serving-4", nHoursAgo(10)),
							newNode("node9", "zone-a1", noClusterID, noMachine, "serving-5", nHoursAgo(10)),
							newNode("node10", "zone-b1", noClusterID, noMachine, "serving-5", nHoursAgo(10)),
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
							newNode("node1", "zone-a1", "cluster-1", noMachine, "serving-3", nHoursAgo(10)),
							newNode("node2", "zone-b1", "cluster-1", noMachine, "serving-3", nHoursAgo(10)),
							newNode("node3", "zone-a1", "cluster-2", noMachine, "serving-4", nHoursAgo(10)),
							newNode("node4", "zone-b1", "cluster-2", noMachine, "serving-4", nHoursAgo(10)),
							newNode("node5", "zone-a1", noClusterID, noMachine, "serving-5", nHoursAgo(10)),
							newNode("node6", "zone-b1", noClusterID, noMachine, "serving-5", nHoursAgo(10)),
							newNode("node7", "zone-a1", noClusterID, noMachine, "serving-6", nHoursAgo(10)),
							newNode("node8", "zone-b1", noClusterID, noMachine, "serving-6", nHoursAgo(10)),
							newNode("node9", "zone-a1", noClusterID, noMachine, "serving-7", nHoursAgo(10)),
							newNode("node10", "zone-b1", noClusterID, noMachine, "serving-7", nHoursAgo(10)),
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
							newNode("node1", "zone-a1", "cluster-1", noMachine, "serving-3", nHoursAgo(9)),
							newNode("node2", "zone-b1", "cluster-1", noMachine, "serving-3", nHoursAgo(8)),
							newNode("node3", "zone-a1", "cluster-2", noMachine, "serving-4", nHoursAgo(7)),
							newNode("node4", "zone-b1", "cluster-2", noMachine, "serving-4", nHoursAgo(7)),
							newNode("node5", "zone-a1", noClusterID, noMachine, "serving-5", nHoursAgo(10)),
							newNode("node6", "zone-b1", noClusterID, noMachine, "serving-5", nHoursAgo(11)),
							newNode("node7", "zone-a1", noClusterID, "openshift-machine-api/machine-A7", "serving-6", nHoursAgo(12)),
							newNode("node8", "zone-b1", noClusterID, "openshift-machine-api/machine-B8", "serving-6", nHoursAgo(13)),
							newNode("node9", "zone-a1", noClusterID, "openshift-machine-api/machine-A9", "serving-7", nHoursAgo(14)),
							newNode("node10", "zone-b1", noClusterID, "openshift-machine-api/machine-B10", "serving-7", nHoursAgo(15)),
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
							newMachineSet("machineSet-A1", desiredReplicas1, "zone-A1", availableReplicas0),
							newMachineSet("machineSet-B2", desiredReplicas1, "zone-B2", availableReplicas0),
							newMachineSet("machineSet-A3", desiredReplicas1, "zone-A3", availableReplicas0),
							newMachineSet("machineSet-B4", desiredReplicas1, "zone-B4", availableReplicas0),
							newMachineSet("machineSet-A5", desiredReplicas1, "zone-A5", availableReplicas0),
							newMachineSet("machineSet-B6", desiredReplicas1, "zone-B6", availableReplicas0),
							newMachineSet("machineSet-A7", desiredReplicas1, "zone-A7", availableReplicas0),
							newMachineSet("machineSet-B8", desiredReplicas1, "zone-B8", availableReplicas0),
							newMachineSet("machineSet-A9", desiredReplicas1, "zone-A9", availableReplicas0),
							newMachineSet("machineSet-B10", desiredReplicas1, "zone-B10", availableReplicas0),
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
		{
			name: "Remove one pair 2",
			fields: fields{
				Interval: time.Second,
				listNodesHandler: func(ctx context.Context) (*corev1.NodeList, error) {
					return &corev1.NodeList{
						TypeMeta: metav1.TypeMeta{Kind: "NodeList"},
						Items: []corev1.Node{
							newNode("node1", "zone-a1", "cluster-1", noMachine, "serving-3", nHoursAgo(9)),
							newNode("node2", "zone-b1", "cluster-1", noMachine, "serving-3", nHoursAgo(8)),
							newNode("node3", "zone-a1", "cluster-2", noMachine, "serving-4", nHoursAgo(7)),
							newNode("node4", "zone-b1", "cluster-2", noMachine, "serving-4", nHoursAgo(7)),
							newNode("node5", "zone-a1", noClusterID, noMachine, "serving-5", nHoursAgo(10)),
							newNode("node6", "zone-b1", noClusterID, noMachine, "serving-5", nHoursAgo(11)),
							newNode("node7", "zone-a1", noClusterID, "openshift-machine-api/machine-A7", "serving-6", nHoursAgo(12)),
							newNode("node8", "zone-b1", noClusterID, "openshift-machine-api/machine-B8", "serving-6", nHoursAgo(13)),
							newNode("node9", "zone-a1", noClusterID, "openshift-machine-api/machine-A9", "serving-7", nHoursAgo(14)),
							newNode("node10", "zone-b1", noClusterID, "openshift-machine-api/machine-B10", "serving-7", nHoursAgo(15)),
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
							newMachineSet("machineSet-A1", desiredReplicas1, "zone-A1", availableReplicas0),
							newMachineSet("machineSet-B2", desiredReplicas1, "zone-B2", availableReplicas0),
							newMachineSet("machineSet-A3", desiredReplicas1, "zone-A3", availableReplicas0),
							newMachineSet("machineSet-B4", desiredReplicas1, "zone-B4", availableReplicas0),
							newMachineSet("machineSet-A5", desiredReplicas1, "zone-A5", availableReplicas0),
							newMachineSet("machineSet-B6", desiredReplicas1, "zone-B6", availableReplicas0),
							newMachineSet("machineSet-A7", desiredReplicas1, "zone-A7", availableReplicas0),
							newMachineSet("machineSet-B8", desiredReplicas1, "zone-B8", availableReplicas0),
							newMachineSet("machineSet-A9", desiredReplicas1, "zone-A9", availableReplicas0),   //
							newMachineSet("machineSet-B10", desiredReplicas0, "zone-B10", availableReplicas0), // to cover machineset already scaled down
						},
					}, nil
				},
				updateMachineSetHandler: func(ctx context.Context, machineSet *machinev1.MachineSet) error {
					switch machineSet.Name {
					case "machineSet-B10":
						return nil
					case "machineSet-A9":
						return fmt.Errorf("you can't scale this")
					}
					return fmt.Errorf("Hey dude you're scaling the wrong machineSet")
				},
				MaxWarm: 2,
			},
			args:    args{ctx: context.TODO()},
			wantErr: true,
		},
		{
			name: "Remove one pair",
			fields: fields{
				Interval: time.Second,
				listNodesHandler: func(ctx context.Context) (*corev1.NodeList, error) {
					return &corev1.NodeList{
						TypeMeta: metav1.TypeMeta{Kind: "NodeList"},
						Items: []corev1.Node{
							newNode("node1", "zone-a1", "cluster-1", noMachine, "serving-3", nHoursAgo(9)),
							newNode("node2", "zone-b1", "cluster-1", noMachine, "serving-3", nHoursAgo(8)),
							newNode("node3", "zone-a1", "cluster-2", noMachine, "serving-4", nHoursAgo(7)),
							newNode("node4", "zone-b1", "cluster-2", noMachine, "serving-4", nHoursAgo(7)),
							newNode("node5", "zone-a1", noClusterID, noMachine, "serving-5", nHoursAgo(10)),
							newNode("node6", "zone-b1", noClusterID, noMachine, "serving-5", nHoursAgo(11)),
							newNode("node7", "zone-a1", noClusterID, "openshift-machine-api/machine-A7", "serving-6", nHoursAgo(12)),
							newNode("node8", "zone-b1", noClusterID, "openshift-machine-api/machine-B8", "serving-6", nHoursAgo(13)),
							newNode("node9", "zone-a1", noClusterID, "openshift-machine-api/machine-A9", "serving-7", nHoursAgo(14)),
							newNode("node10", "zone-b1", noClusterID, "openshift-machine-api/machine-B10", "serving-7", nHoursAgo(15)),
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
							newMachineSet("machineSet-A1", desiredReplicas1, "zone-A1", availableReplicas0),
							newMachineSet("machineSet-B2", desiredReplicas1, "zone-B2", availableReplicas0),
							newMachineSet("machineSet-A3", desiredReplicas1, "zone-A3", availableReplicas0),
							newMachineSet("machineSet-B4", desiredReplicas1, "zone-B4", availableReplicas0),
							newMachineSet("machineSet-A5", desiredReplicas1, "zone-A5", availableReplicas0),
							newMachineSet("machineSet-B6", desiredReplicas1, "zone-B6", availableReplicas0),
							newMachineSet("machineSet-A7", desiredReplicas1, "zone-A7", availableReplicas0),
							newMachineSet("machineSet-B8", desiredReplicas1, "zone-B8", availableReplicas0),
							newMachineSet("machineSet-A9", desiredReplicas1, "zone-A9", availableReplicas0),
							newMachineSet("machineSet-B10", desiredReplicas1, "zone-B10", availableReplicas0),
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
