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

// Package node implements the `create node` command
package node

import (
	"time"

	"github.com/spf13/cobra"

	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"
	"sigs.k8s.io/kind/pkg/errors"
	"sigs.k8s.io/kind/pkg/log"

	"sigs.k8s.io/kind/pkg/internal/cli"
	"sigs.k8s.io/kind/pkg/internal/runtime"
)

type flagpole struct {
	Name  string
	Count int
	Image string
	Wait  time.Duration
}

// NewCommand returns a new cobra.Command for adding nodes to a cluster
func NewCommand(logger log.Logger, streams cmd.IOStreams) *cobra.Command {
	flags := &flagpole{}
	cmd := &cobra.Command{
		Args:  cobra.NoArgs,
		Use:   "node",
		Short: "Creates worker nodes in a cluster",
		Long: `Creates one or more worker nodes in an existing kind cluster.

Each new node is provisioned as a container, configured, and joined to the
cluster with "kubeadm join". Only worker nodes can be created.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.OverrideDefaultName(cmd.Flags())
			return runE(logger, flags)
		},
	}
	cmd.Flags().StringVarP(
		&flags.Name,
		"name",
		"n",
		cluster.DefaultName,
		"the cluster name",
	)
	cmd.Flags().IntVar(
		&flags.Count,
		"count",
		1,
		"the number of worker nodes to create",
	)
	cmd.Flags().StringVar(
		&flags.Image,
		"image",
		"",
		"node docker image to use for the new nodes (defaults to the image of an existing node in the cluster)",
	)
	cmd.Flags().DurationVar(
		&flags.Wait,
		"wait",
		time.Duration(0),
		"wait for the new node(s) to be ready before returning (e.g. -w=90s)",
	)
	return cmd
}

func runE(logger log.Logger, flags *flagpole) error {
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
		runtime.GetDefault(logger),
	)
	if err := provider.AddNodes(flags.Name, flags.Count, flags.Image, flags.Wait); err != nil {
		return errors.Wrapf(err, "failed to create node(s) in cluster %q", flags.Name)
	}
	return nil
}
