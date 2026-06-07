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
	"fmt"
	"strconv"
	"strings"
)

// MakeNodeNamer returns a func(role string)(nodeName string)
// used to name nodes based on their role and the clusterName
func MakeNodeNamer(clusterName string) func(string) string {
	counter := make(map[string]int)
	return func(role string) string {
		count := 1
		suffix := ""
		if v, ok := counter[role]; ok {
			count += v
			suffix = fmt.Sprintf("%d", count)
		}
		counter[role] = count
		return fmt.Sprintf("%s-%s%s", clusterName, role, suffix)
	}
}

// MakeNodeNamerWithExisting works like MakeNodeNamer but seeds the per-role
// counters from the given existing node names so that generated names continue
// after the highest existing index for each role. This is used when adding
// nodes to an existing cluster to avoid colliding with nodes that are already
// present (including in the presence of gaps left by previously removed nodes).
func MakeNodeNamerWithExisting(clusterName string, existing []string) func(string) string {
	counter := make(map[string]int)
	prefix := clusterName + "-"
	for _, name := range existing {
		rem := strings.TrimPrefix(name, prefix)
		if rem == name {
			// name does not have the expected cluster prefix, skip it
			continue
		}
		role, idx := splitRoleIndex(rem)
		if idx > counter[role] {
			counter[role] = idx
		}
	}
	return func(role string) string {
		count := 1
		suffix := ""
		if v, ok := counter[role]; ok {
			count += v
			suffix = fmt.Sprintf("%d", count)
		}
		counter[role] = count
		return fmt.Sprintf("%s-%s%s", clusterName, role, suffix)
	}
}

// splitRoleIndex splits a node name suffix (the portion after
// "<clusterName>-") into its role and 1-based index. The first node of a role
// has no numeric suffix and is treated as index 1, the second is "<role>2",
// etc., matching MakeNodeNamer's scheme.
func splitRoleIndex(s string) (role string, index int) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		// no trailing digits, this is the first (unsuffixed) node
		return s, 1
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil {
		return s, 1
	}
	return s[:i], n
}
