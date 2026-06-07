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

// Package addnode implements adding worker nodes to an existing cluster.
package addnode

import (
	"fmt"
	"strings"
	"time"

	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/internal/create/actions/kubeadmjoin"
	"sigs.k8s.io/kind/pkg/cluster/internal/kubeadm"
	"sigs.k8s.io/kind/pkg/cluster/internal/providers"
	"sigs.k8s.io/kind/pkg/cluster/internal/providers/common"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
	"sigs.k8s.io/kind/pkg/errors"
	"sigs.k8s.io/kind/pkg/exec"
	"sigs.k8s.io/kind/pkg/internal/apis/config"
	"sigs.k8s.io/kind/pkg/internal/cli"
	"sigs.k8s.io/kind/pkg/log"
)

// Options holds the options for adding nodes to a cluster
type Options struct {
	// ClusterName is the name of the cluster to add nodes to
	ClusterName string
	// Count is the number of nodes to add
	Count int
	// Role is the role of the nodes to add (only "worker" is supported)
	Role string
	// Image overrides the node image; if empty the image of an existing node
	// in the cluster is used
	Image string
	// WaitForReady is how long to wait for the new nodes to become Ready
	// before returning; if 0 the command returns immediately after join
	WaitForReady time.Duration
}

// AddNodes adds new worker nodes to an existing cluster
func AddNodes(logger log.Logger, p providers.Provider, opts *Options) error {
	if opts.Role == "" {
		opts.Role = constants.WorkerNodeRoleValue
	}
	if opts.Role != constants.WorkerNodeRoleValue {
		return errors.Errorf("only worker nodes can be added, got role %q", opts.Role)
	}
	if opts.Count < 1 {
		return errors.Errorf("count must be at least 1, got %d", opts.Count)
	}

	// discover the existing cluster
	allNodes, err := p.ListNodes(opts.ClusterName)
	if err != nil {
		return err
	}
	if len(allNodes) == 0 {
		return errors.Errorf("unknown cluster %q, no nodes found", opts.ClusterName)
	}

	// we run kubeadm token / kubectl commands against the bootstrap control plane
	controlPlanes, err := nodeutils.ControlPlaneNodes(allNodes)
	if err != nil {
		return err
	}
	if len(controlPlanes) == 0 {
		return errors.Errorf("cluster %q has no control-plane node", opts.ClusterName)
	}
	bootstrap := controlPlanes[0]

	// detect the cluster IP family from an existing kubernetes node, so the
	// new nodes are configured (and join) with the right address family
	kubeNodes, err := nodeutils.InternalNodes(allNodes)
	if err != nil {
		return err
	}
	if len(kubeNodes) == 0 {
		return errors.Errorf("cluster %q has no kubernetes nodes", opts.ClusterName)
	}
	ipFamily, err := detectIPFamily(kubeNodes[0])
	if err != nil {
		return err
	}

	// build a minimal config describing the new nodes to provision
	cfg := &config.Cluster{Name: opts.ClusterName}
	cfg.Networking.IPFamily = ipFamily
	for i := 0; i < opts.Count; i++ {
		cfg.Nodes = append(cfg.Nodes, config.Node{
			Role:  config.NodeRole(opts.Role),
			Image: opts.Image,
		})
	}

	status := cli.StatusForLogger(logger)

	logger.V(0).Infof("Adding %d node(s) to cluster %q ...", opts.Count, opts.ClusterName)

	// provision the new node containers
	newNodes, err := p.CreateNodes(status, cfg)
	if err != nil {
		return err
	}

	// gather the data needed to generate join configuration
	controlPlaneEndpoint, err := p.GetAPIServerInternalEndpoint(opts.ClusterName)
	if err != nil {
		return err
	}
	providerInfo, err := p.Info()
	if err != nil {
		return err
	}
	providerName := fmt.Sprintf("%s", p)

	// the well-known bootstrap token kind uses at creation time expires after
	// 24h, so generate a fresh one for joining the new nodes
	token, err := createBootstrapToken(bootstrap)
	if err != nil {
		return err
	}

	// write the join config to each new node and join it
	status.Start("Joining worker nodes 🚜")
	joinFns := make([]func() error, 0, len(newNodes))
	for _, node := range newNodes {
		node := node // capture loop variable
		joinFns = append(joinFns, func() error {
			if err := writeJoinConfig(node, opts.ClusterName, controlPlaneEndpoint, token, providerName, ipFamily, providerInfo.Rootless); err != nil {
				return err
			}
			return kubeadmjoin.RunKubeadmJoin(logger, node)
		})
	}
	if err := errors.UntilErrorConcurrent(joinFns); err != nil {
		status.End(false)
		return err
	}
	status.End(true)

	// optionally wait for the new nodes to become Ready
	if opts.WaitForReady > 0 {
		if err := waitForNodesReady(logger, bootstrap, newNodes, opts.WaitForReady); err != nil {
			return err
		}
	}

	for _, node := range newNodes {
		logger.V(0).Infof("Added node %q to cluster %q", node.String(), opts.ClusterName)
	}
	return nil
}

// detectIPFamily determines the cluster IP family from the addresses assigned
// to an existing node. kind creates the node network per the cluster's IP
// family, so the addresses a node receives reflect that family.
func detectIPFamily(node nodes.Node) (config.ClusterIPFamily, error) {
	ipv4, ipv6, err := node.IP()
	if err != nil {
		return "", errors.Wrap(err, "failed to get IP for node")
	}
	switch {
	case ipv4 != "" && ipv6 != "":
		return config.DualStackFamily, nil
	case ipv6 != "":
		return config.IPv6Family, nil
	case ipv4 != "":
		return config.IPv4Family, nil
	default:
		return "", errors.Errorf("node %q has no IP address", node.String())
	}
}

// createBootstrapToken creates a fresh bootstrap token on the given control
// plane node and returns it
func createBootstrapToken(controlPlane nodes.Node) (string, error) {
	cmd := controlPlane.Command("kubeadm", "token", "create", "--ttl", "15m")
	lines, err := exec.OutputLines(cmd)
	if err != nil {
		return "", errors.Wrap(err, "failed to create bootstrap token")
	}
	// the token is printed on its own line; take the last non-empty line
	for i := len(lines) - 1; i >= 0; i-- {
		if token := strings.TrimSpace(lines[i]); token != "" {
			return token, nil
		}
	}
	return "", errors.New("failed to parse bootstrap token from kubeadm output")
}

// writeJoinConfig generates a kubeadm join configuration for a worker node and
// writes it to the well known location on the node
func writeJoinConfig(node nodes.Node, clusterName, controlPlaneEndpoint, token, providerName string, ipFamily config.ClusterIPFamily, rootless bool) error {
	kubeVersion, err := nodeutils.KubeVersion(node)
	if err != nil {
		return errors.Wrap(err, "failed to get kubernetes version from node")
	}

	ipv4, ipv6, err := node.IP()
	if err != nil {
		return errors.Wrap(err, "failed to get IP for node")
	}

	data := kubeadm.ConfigData{
		NodeProvider:         providerName,
		ClusterName:          clusterName,
		ControlPlaneEndpoint: controlPlaneEndpoint,
		APIBindPort:          common.APIServerInternalPort,
		Token:                token,
		KubernetesVersion:    kubeVersion,
		ControlPlane:         false,
		NodeName:             node.String(),
		IPFamily:             ipFamily,
		RootlessProvider:     rootless,
	}
	switch ipFamily {
	case config.IPv6Family:
		if ipv6 == "" {
			return errors.Errorf("node %q has no IPv6 address but cluster is IPv6", node.String())
		}
		data.NodeAddress = ipv6
	case config.DualStackFamily:
		// default to IPv4 primary, matching kind's dual-stack default
		data.NodeAddress = fmt.Sprintf("%s,%s", ipv4, ipv6)
	default:
		data.NodeAddress = ipv4
	}

	kubeadmConfig, err := kubeadm.Config(data)
	if err != nil {
		return errors.Wrap(err, "failed to generate kubeadm join config")
	}
	if err := nodeutils.WriteFile(node, "/kind/kubeadm.conf", kubeadmConfig); err != nil {
		return errors.Wrap(err, "failed to write kubeadm config to node")
	}
	return nil
}

// waitForNodesReady waits until all of the given nodes report Ready, or until
// the timeout elapses
func waitForNodesReady(logger log.Logger, controlPlane nodes.Node, newNodes []nodes.Node, timeout time.Duration) error {
	logger.V(0).Infof("Waiting ≤ %s for new node(s) to be Ready ⏳", timeout.Round(time.Second))
	until := time.Now().Add(timeout)
	for _, node := range newNodes {
		name := node.String()
		if !tryUntil(until, func() bool {
			return nodeIsReady(controlPlane, name)
		}) {
			logger.V(0).Infof(" • WARNING: Timed out waiting for node %q to be Ready ⚠️", name)
			return errors.Errorf("timed out waiting for node %q to be Ready", name)
		}
	}
	logger.V(0).Info(" • New node(s) are Ready 💚")
	return nil
}

// nodeIsReady returns true if the named node reports a Ready condition of True
func nodeIsReady(controlPlane nodes.Node, nodeName string) bool {
	cmd := controlPlane.Command(
		"kubectl",
		"--kubeconfig=/etc/kubernetes/admin.conf",
		"get", "node", nodeName,
		"-o=jsonpath={.status.conditions[?(@.type=='Ready')].status}",
	)
	lines, err := exec.OutputLines(cmd)
	if err != nil || len(lines) == 0 {
		return false
	}
	return strings.Contains(lines[0], "True")
}

// tryUntil calls try in a loop until the deadline until has passed or try
// returns true, returning whether try ever returned true
func tryUntil(until time.Time, try func() bool) bool {
	for until.After(time.Now()) {
		if try() {
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}
