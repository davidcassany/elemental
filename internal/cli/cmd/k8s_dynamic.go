/* Copyright © 2025-2026 SUSE LLC
 * SPDX-License-Identifier: Apache-2.0
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// K8sDynamicFlags contains the flags for the k8s-dynamic command.
type K8sDynamicFlags struct {
	ConfigPath string
}

// K8sDynamicArgs holds parsed k8s-dynamic command flags.
var K8sDynamicArgs K8sDynamicFlags

// NewK8sDynamicCommand creates the k8s-dynamic command with subcommands.
func NewK8sDynamicCommand(appName string, applyAction func(context.Context, *cli.Command) error) *cli.Command {
	return &cli.Command{
		Name:  "k8s-dynamic",
		Usage: "Manage dynamic Kubernetes configuration from Dynamic Node User Data",
		Commands: []*cli.Command{
			{
				Name:      "apply",
				Usage:     "Read Dynamic Node User Data and render Kubernetes config templates",
				UsageText: fmt.Sprintf("%s k8s-dynamic apply [OPTIONS]", appName),
				Action:    applyAction,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "config",
						Usage:       "Path to Dynamic Node User Data YAML file",
						Destination: &K8sDynamicArgs.ConfigPath,
					},
				},
			},
		},
	}
}
