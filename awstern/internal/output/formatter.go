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

package output

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"

	"github.com/zchee/awstools/awstern/internal/types"
)

// ANSI color codes
var colors = []string{
	"\033[36m", // cyan
	"\033[33m", // yellow
	"\033[35m", // magenta
	"\033[32m", // green
	"\033[31m", // red
	"\033[34m", // blue
	"\033[96m", // bright cyan
	"\033[93m", // bright yellow
	"\033[95m", // bright magenta
	"\033[92m", // bright green
}

const resetColor = "\033[0m"

// Formatter formats log events for output.
type Formatter struct {
	writer       io.Writer
	format       string
	timestamps   bool
	colorEnabled bool
	colorMap     map[string]string
}

// NewFormatter creates a new output formatter.
func NewFormatter(format string, timestamps bool, colorMode string) *Formatter {
	colorEnabled := shouldEnableColor(colorMode)

	return &Formatter{
		writer:       os.Stdout,
		format:       format,
		timestamps:   timestamps,
		colorEnabled: colorEnabled,
		colorMap:     make(map[string]string),
	}
}

// shouldEnableColor determines if color output should be enabled.
func shouldEnableColor(mode string) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	case "auto":
		// Check if stdout is a terminal
		fileInfo, err := os.Stdout.Stat()
		if err != nil {
			return false
		}
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	default:
		return false
	}
}

// getColor returns a consistent color for a log group name.
func (f *Formatter) getColor(logGroupName string) string {
	if !f.colorEnabled {
		return ""
	}

	if color, exists := f.colorMap[logGroupName]; exists {
		return color
	}

	// Hash the log group name to get a consistent color
	h := fnv.New32a()
	h.Write([]byte(logGroupName))
	colorIndex := int(h.Sum32()) % len(colors)
	color := colors[colorIndex]
	f.colorMap[logGroupName] = color

	return color
}

// Format formats and writes a log event.
func (f *Formatter) Format(event types.LogEvent) error {
	switch f.format {
	case "json":
		return f.formatJSON(event)
	case "raw":
		return f.formatRaw(event)
	default:
		return f.formatDefault(event)
	}
}

// formatDefault formats a log event in the default format.
func (f *Formatter) formatDefault(event types.LogEvent) error {
	color := f.getColor(event.LogGroupName)
	reset := ""
	if f.colorEnabled {
		reset = resetColor
	}

	var output string
	if f.timestamps {
		timestamp := event.Timestamp.Format("2006-01-02T15:04:05.000Z07:00")
		output = fmt.Sprintf("%s%s%s %s[%s/%s]%s %s\n",
			color, timestamp, reset,
			color, event.LogGroupName, event.LogStreamName, reset,
			event.Message)
	} else {
		output = fmt.Sprintf("%s[%s/%s]%s %s\n",
			color, event.LogGroupName, event.LogStreamName, reset,
			event.Message)
	}

	_, err := f.writer.Write([]byte(output))
	return err
}

// formatJSON formats a log event as JSON.
func (f *Formatter) formatJSON(event types.LogEvent) error {
	data := map[string]any{
		"timestamp":  event.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		"log_group":  event.LogGroupName,
		"log_stream": event.LogStreamName,
		"message":    event.Message,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	_, err = f.writer.Write(append(jsonData, '\n'))
	return err
}

// formatRaw formats a log event as raw message only.
func (f *Formatter) formatRaw(event types.LogEvent) error {
	_, err := fmt.Fprintf(f.writer, "%s\n", event.Message)
	return err
}
