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

// Package node implements the `delete node` command
package node

import (
	"github.com/spf13/cobra"

	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"
	"sigs.k8s.io/kind/pkg/errors"
	"sigs.k8s.io/kind/pkg/log"

	"sigs.k8s.io/kind/pkg/internal/cli"
	"sigs.k8s.io/kind/pkg/internal/runtime"
)

type flagpole struct {
	Name string
}

// NewCommand returns a new cobra.Command for removing a node from a cluster
func NewCommand(logger log.Logger, streams cmd.IOStreams) *cobra.Command {
	flags := &flagpole{}
	cmd := &cobra.Command{
		Args:  cobra.ExactArgs(1),
		Use:   "node NODE_NAME",
		Short: "Deletes a worker node from a cluster",
		Long: `Deletes a worker node from an existing kind cluster.

The node is cordoned and drained, deleted from the Kubernetes API, and its
container is removed. Only worker nodes can be deleted.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.OverrideDefaultName(cmd.Flags())
			return runE(logger, flags, args[0])
		},
	}
	cmd.Flags().StringVarP(
		&flags.Name,
		"name",
		"n",
		cluster.DefaultName,
		"the cluster name",
	)
	return cmd
}

func runE(logger log.Logger, flags *flagpole, nodeName string) error {
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
		runtime.GetDefault(logger),
	)
	if err := provider.DeleteNode(flags.Name, nodeName); err != nil {
		return errors.Wrapf(err, "failed to delete node %q from cluster %q", nodeName, flags.Name)
	}
	return nil
}
