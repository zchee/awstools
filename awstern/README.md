# awstern

awstern is a CloudWatch Logs tailing CLI tool inspired by [stern](https://github.com/stern/stern) for Kubernetes. It allows you to tail logs from multiple CloudWatch log groups concurrently with color-coded output.

## Features

- 🔍 **Regex-based log group filtering** - Match log groups using regular expressions
- 🎨 **Color-coded output** - Each log group gets a consistent color for easy identification
- ⚡ **Concurrent tailing** - Tail multiple log groups simultaneously
- 🕒 **Time-based filtering** - Show logs from a specific time window
- 📊 **Multiple output formats** - Support for default, JSON, and raw output formats
- 🚫 **Exclude patterns** - Filter out unwanted log groups
- ⏱️ **Configurable polling** - Adjust polling interval for new logs

## Installation

```bash
# From source
cd awstern
go build -o awstern .

# Or using go install
go install github.com/zchee/awstools/awstern@latest
```

## Usage

### Basic Examples

```bash
# Tail all log groups
awstern --all-log-groups

# Tail log groups matching "ecs"
awstern ecs

# Tail log groups with a regex pattern
awstern "^/aws/ecs/.*"

# Tail specific log group types
awstern "RDSOSMetrics"
awstern "kaleidologs"
```

### Advanced Examples

```bash
# Tail logs from the last hour
awstern --since 1h ecs

# Show only the last 10 lines from each log group
awstern --tail 10 ecs

# Output in JSON format
awstern --output json ecs

# Exclude test log groups
awstern --exclude "test" ecs

# Tail without timestamps
awstern --timestamps=false ecs

# Limit to 10 log groups maximum
awstern --max-log-groups 10 ecs

# Custom poll interval (check for new logs every 5 seconds)
awstern --poll-interval 5s ecs

# Force color output even when piping
awstern --color always ecs | less -R
```

### Using as Default Command

awstern makes `tail` the default command, so these are equivalent:

```bash
awstern tail ecs
awstern ecs
```

## CLI Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--all-log-groups` | `-A` | `false` | Tail from all log groups |
| `--since` | `-s` | `5m` | Return logs newer than a relative duration (e.g., 5s, 2m, 3h) |
| `--tail` | | `0` | Number of lines to show from the end of logs (0 = all) |
| `--timestamps` | `-t` | `true` | Print timestamps |
| `--color` | | `auto` | Color output (auto/always/never) |
| `--output` | `-o` | `default` | Output format (default/json/raw) |
| `--exclude` | | | Regular expression to exclude log groups |
| `--max-log-groups` | | `50` | Maximum number of log groups to tail |
| `--poll-interval` | | `1s` | Interval to poll for new logs |
| `--region` | | | AWS region (overrides default) |

## Output Formats

### Default Format

Includes timestamp, log group name, log stream name, and message:

```
2025-01-15T10:30:45.123Z [/aws/ecs/my-service/my-task] Log message here
```

### JSON Format

```json
{
  "timestamp": "2025-01-15T10:30:45.123Z",
  "log_group": "/aws/ecs/my-service",
  "log_stream": "ecs/my-task/abc123",
  "message": "Log message here"
}
```

### Raw Format

Only the log message (useful for piping to other tools):

```
Log message here
```

## AWS Credentials

awstern uses the standard AWS credential chain:

1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
2. AWS credentials file (`~/.aws/credentials`)
3. IAM roles (when running on EC2, ECS, or Lambda)

Required IAM permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "logs:DescribeLogGroups",
        "logs:FilterLogEvents"
      ],
      "Resource": "*"
    }
  ]
}
```

## Common Use Cases

### Tail ECS Service Logs

```bash
awstern "/aws/ecs/my-service"
```

### Monitor RDS Logs

```bash
awstern "RDSOSMetrics"
```

### Debug Lambda Functions

```bash
awstern "/aws/lambda" --since 10m
```

### Filter Application Logs

```bash
awstern "production" --exclude "health-check"
```

## Comparison with stern

| Feature | stern (Kubernetes) | awstern (CloudWatch) |
|---------|-------------------|----------------------|
| Resource | Pods | Log Groups |
| Filter by | Pod name regex | Log group name regex |
| Color output | ✅ | ✅ |
| Concurrent tailing | ✅ | ✅ |
| Timestamps | ✅ | ✅ |
| Multiple formats | ✅ | ✅ |
| Exclude pattern | ✅ | ✅ |

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](../CONTRIBUTING.md) for details.

## License

Apache License 2.0. See [LICENSE](../LICENSE) for details.
