// Copyright 2025 The awstools Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

// Command awstern Command awstern tails AWS CloudWatch Logs.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/zchee/awstools/awstern/cmd"
)

const version = "0.0.1"

func main() {
	root := newRootCmd()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	root.SetContext(ctx)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	tailCmd := cmd.NewTailCmd()

	cmd := &cobra.Command{
		Use:     "awstern",
		Short:   "Tails AWS CloudWatch Logs.",
		Long:    `awstern: Tails AWS CloudWatch Logs concurrently.`,
		Version: version,
		// Make tail the default command when no subcommand is specified
		Args: cobra.ArbitraryArgs,
		RunE: tailCmd.RunE,
	}

	// Copy flags from tail command to root
	cmd.Flags().AddFlagSet(tailCmd.Flags())

	// Add tail as explicit subcommand
	cmd.AddCommand(tailCmd)

	// Completion subcommand
	completion := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Args:  cobra.ExactValidArgs(1),
		ValidArgs: []string{
			"bash", "zsh", "fish", "powershell",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	}
	cmd.AddCommand(completion)

	return cmd
}
