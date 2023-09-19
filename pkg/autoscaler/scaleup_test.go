package autoscaler

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	machinev1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
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
