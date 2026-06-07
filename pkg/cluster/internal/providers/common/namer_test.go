/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"testing"

	"sigs.k8s.io/kind/pkg/internal/assert"
)

func TestMakeNodeNamer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		clusterName string
		nodes       []string // list of role nodes that belong to the cluster
		want        []string
	}{
		{
			name:        "Default cluster name one node",
			clusterName: "kind",
			nodes:       []string{"control-plane"},
			want:        []string{"kind-control-plane"},
		},
		{
			name:        "Cluster with 3 nodes",
			clusterName: "kind-test",
			nodes:       []string{"control-plane", "worker", "worker"},
			want:        []string{"kind-test-control-plane", "kind-test-worker", "kind-test-worker2"},
		},
		{
			name:        "Cluster with many nodes",
			clusterName: "ab1",
			nodes:       []string{"control-plane", "control-plane", "control-plane", "external-load-balancer", "worker", "worker", "worker"},
			want:        []string{"ab1-control-plane", "ab1-control-plane2", "ab1-control-plane3", "ab1-external-load-balancer", "ab1-worker", "ab1-worker2", "ab1-worker3"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var names []string
			nodeNamer := MakeNodeNamer(tc.clusterName)
			for _, nodeRole := range tc.nodes {
				names = append(names, nodeNamer(nodeRole))
			}
			assert.DeepEqual(t, tc.want, names)
		})
	}
}

func TestMakeNodeNamerWithExisting(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		clusterName string
		existing    []string // existing node names in the cluster
		roles       []string // roles to generate names for
		want        []string
	}{
		{
			name:        "continue after single worker",
			clusterName: "kind",
			existing:    []string{"kind-control-plane", "kind-worker"},
			roles:       []string{"worker"},
			want:        []string{"kind-worker2"},
		},
		{
			name:        "continue after multiple workers",
			clusterName: "kind",
			existing:    []string{"kind-control-plane", "kind-worker", "kind-worker2", "kind-worker3"},
			roles:       []string{"worker", "worker"},
			want:        []string{"kind-worker4", "kind-worker5"},
		},
		{
			name:        "gap from removed node continues after max",
			clusterName: "kind",
			existing:    []string{"kind-control-plane", "kind-worker", "kind-worker3"},
			roles:       []string{"worker"},
			want:        []string{"kind-worker4"},
		},
		{
			name:        "first worker when none exist",
			clusterName: "kind",
			existing:    []string{"kind-control-plane"},
			roles:       []string{"worker"},
			want:        []string{"kind-worker"},
		},
		{
			name:        "ignores names without cluster prefix",
			clusterName: "kind",
			existing:    []string{"other-worker", "kind-worker"},
			roles:       []string{"worker"},
			want:        []string{"kind-worker2"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var names []string
			nodeNamer := MakeNodeNamerWithExisting(tc.clusterName, tc.existing)
			for _, role := range tc.roles {
				names = append(names, nodeNamer(role))
			}
			assert.DeepEqual(t, tc.want, names)
		})
	}
}

func TestSplitRoleIndex(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in        string
		wantRole  string
		wantIndex int
	}{
		{"worker", "worker", 1},
		{"worker2", "worker", 2},
		{"worker10", "worker", 10},
		{"control-plane", "control-plane", 1},
		{"control-plane3", "control-plane", 3},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			role, index := splitRoleIndex(tc.in)
			assert.DeepEqual(t, tc.wantRole, role)
			assert.DeepEqual(t, tc.wantIndex, index)
		})
	}
}
