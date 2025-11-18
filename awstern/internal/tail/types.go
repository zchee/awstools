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

package tail

import (
	"regexp"
	"time"
)

// Config holds configuration for tailing CloudWatch logs.
type Config struct {
	// LogGroupPattern is a regular expression to match log group names
	LogGroupPattern *regexp.Regexp

	// ExcludePattern is a regular expression to exclude log group names
	ExcludePattern *regexp.Regexp

	// Since specifies how far back to read logs
	Since time.Duration

	// Tail specifies the number of lines to show initially (0 = all)
	Tail int

	// Timestamps enables timestamp display
	Timestamps bool

	// ColorMode controls color output (auto/always/never)
	ColorMode string

	// OutputFormat specifies output format (default/json/raw)
	OutputFormat string

	// MaxLogGroups limits concurrent log groups to tail
	MaxLogGroups int

	// AllLogGroups tails from all log groups
	AllLogGroups bool

	// PollInterval is the interval to poll for new logs
	PollInterval time.Duration
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		LogGroupPattern: regexp.MustCompile(".*"),
		Since:           5 * time.Minute,
		Tail:            0,
		Timestamps:      true,
		ColorMode:       "auto",
		OutputFormat:    "default",
		MaxLogGroups:    50,
		AllLogGroups:    false,
		PollInterval:    1 * time.Second,
	}
}
