/*
Copyright The Kubernetes Authors.

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

// Package deletenode implements removing worker nodes from an existing cluster.
package deletenode

import (
	"strings"

	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/internal/providers"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
	"sigs.k8s.io/kind/pkg/errors"
	"sigs.k8s.io/kind/pkg/exec"
	"sigs.k8s.io/kind/pkg/log"
)

// DeleteNode removes a worker node from an existing cluster. It cordons and
// drains the node, deletes it from the Kubernetes API, and finally removes the
// underlying container.
func DeleteNode(logger log.Logger, p providers.Provider, clusterName, nodeName string) error {
	allNodes, err := p.ListNodes(clusterName)
	if err != nil {
		return err
	}
	if len(allNodes) == 0 {
		return errors.Errorf("unknown cluster %q, no nodes found", clusterName)
	}

	// locate the target node
	var target nodes.Node
	for _, n := range allNodes {
		if n.String() == nodeName {
			target = n
			break
		}
	}
	if target == nil {
		return errors.Errorf("node %q not found in cluster %q", nodeName, clusterName)
	}

	// only worker nodes may be removed; removing a control-plane node requires
	// etcd / load balancer reconfiguration which is not supported here
	role, err := target.Role()
	if err != nil {
		return err
	}
	if role != constants.WorkerNodeRoleValue {
		return errors.Errorf("only worker nodes can be removed, node %q has role %q", nodeName, role)
	}

	// we run kubectl against a control-plane node so the host does not need
	// kubectl or a kubeconfig
	controlPlanes, err := nodeutils.ControlPlaneNodes(allNodes)
	if err != nil {
		return err
	}
	if len(controlPlanes) == 0 {
		return errors.Errorf("cluster %q has no control-plane node", clusterName)
	}
	controlPlane := controlPlanes[0]

	logger.V(0).Infof("Removing node %q from cluster %q ...", nodeName, clusterName)

	// cordon + drain to gracefully evict workloads
	if err := drainNode(logger, controlPlane, nodeName); err != nil {
		return err
	}

	// remove the node object from the API
	if err := deleteNodeFromAPI(controlPlane, nodeName); err != nil {
		return err
	}

	// finally delete the underlying container
	if err := p.DeleteNodes([]nodes.Node{target}); err != nil {
		return err
	}

	logger.V(0).Infof("Removed node %q from cluster %q", nodeName, clusterName)
	return nil
}

// drainNode cordons and drains the named node using kubectl on the control
// plane node
func drainNode(logger log.Logger, controlPlane nodes.Node, nodeName string) error {
	cmd := controlPlane.Command(
		"kubectl",
		"--kubeconfig=/etc/kubernetes/admin.conf",
		"drain", nodeName,
		"--ignore-daemonsets",
		"--delete-emptydir-data",
		"--force",
		"--timeout=90s",
	)
	lines, err := exec.CombinedOutputLines(cmd)
	logger.V(3).Info(strings.Join(lines, "\n"))
	if err != nil {
		return errors.Wrap(err, "failed to drain node")
	}
	return nil
}

// deleteNodeFromAPI deletes the named node object from the Kubernetes API using
// kubectl on the control plane node
func deleteNodeFromAPI(controlPlane nodes.Node, nodeName string) error {
	cmd := controlPlane.Command(
		"kubectl",
		"--kubeconfig=/etc/kubernetes/admin.conf",
		"delete", "node", nodeName,
		"--ignore-not-found=true",
	)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to delete node from API")
	}
	return nil
}
