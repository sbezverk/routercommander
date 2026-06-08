# routercommander

## Overview

`routercommander` automates command collection from one router or from a router inventory. It runs commands over SSH, writes a timestamped log per router, and can optionally process command output against regex patterns and structured tests.

The command profile is described in YAML. A minimal collect profile looks like this:

```yaml
collect:
  process_result: true

commands:
  - command: show cef drops
    command_timeout: 30
    times: 1
    interval: 1
    location:
      - "0/0/CPU0"
      - "0/1/CPU0"
      - "0/2/CPU0"
    patterns:
      - pattern_string: '((:?\w+\s)+)(drops\s+)(packets\s+:)\s+[1-9]\d*\n'
    debug: false
```

In collect mode, `collect.process_result` controls whether command output is checked against configured `patterns` and `tests`. If it is false or omitted, command output is still collected but pattern/test processing is skipped unless a command sets `process_result: true`.

## YAML Model

The active command YAML model uses these top-level sections:

```yaml
collect:
  stop_on_error: false
  process_result: true

repro:
  times: 10
  interval: 5
  stop_when_triggered: true
  if_triggered_commands:
    - command: show tech-support
      command_timeout: 300

tests:
  - command: show cef drops
    command_tests:
      - id: 1
        pattern:
          pattern_string: 'drops\s+packets\s+:\s+[1-9]\d*'
        fields:
          - field_number: 1
            operation: compare_with_value_neq
            value: "0"
        separator: " "

commands:
  - command: show cef drops
    process_result: true
    command_test_ids: [1]
    patterns:
      - pattern_string: 'drops\s+packets\s+:\s+[1-9]\d*'
```

Common command fields:

- `command`: command to execute.
- `command_timeout`: command timeout in seconds.
- `times` and `interval`: repeat a command and delay between repeats.
- `wait_before` and `wait_after`: delay around the command execution.
- `location`: list of locations to append to the command.
- `location_fmt_tmpl`: template used to format locations.
- `location_customized`: command contains `{{.Location}}` where the location should be inserted.
- `pipe_modifier`: pipe modifier appended to the command.
- `debug`: enables command-level debug behavior.
- `process_result`: enables result processing for this command.
- `patterns`: regexes used to record matching output lines.
- `command_test_ids`: restricts which tests run for this command. If omitted, all tests for the command run.

Structured test operations currently supported by `fields[].operation` are:

- `compare_with_previous_neq`
- `compare_with_previous_eq`
- `compare_with_value_neq`
- `compare_with_value_eq`
- `contain_substring`
- `not_contain_substring`

See [testdata/commands_v2.md](testdata/commands_v2.md) for a fuller schema reference.

## Modes

### Collect

In collect mode, `routercommander` runs the commands once unless a command sets `times`. It collects output for every command. Pattern and test evaluation runs only when `collect.process_result` or the command's `process_result` is true.

### Repro

If the YAML has a `repro` section, `routercommander` runs in repro mode. The main `commands` list is executed `repro.times` times, with `repro.interval` seconds between iterations.

Repro mode always processes command output. A repro trigger is produced by a matching `tests.command_tests` entry. When a test triggers, its own `if_triggered_commands` run first. If any test triggers, the global `repro.if_triggered_commands` run after the main command group. If `repro.stop_when_triggered` is true, repro stops after the first triggered iteration.

## To Run

`routercommander` uses SSH to connect to routers. The required flags for a direct single-router run are:

- `--username`: SSH username.
- `--password` or `--password-stdin`: SSH password.
- `--router-name`: router name or address.
- `--commands-file`: command profile YAML.

Example:

```bash
routercommander --username=root --password=1234567 --router-name=router1 --commands-file=./show_fib.yaml
```

To run against multiple routers, pass `--routers-file` with a router inventory YAML. See [docs/router_inventory_schema.md](docs/router_inventory_schema.md) for the inventory format.

Useful batch-control flags:

- `--max-concurrent-sessions`: maximum concurrent SSH sessions. Default is `10`.
- `--sessions-start-interval-ms`: delay between starting sessions. Default is `500`.

## Docker

When running as a container, mount a directory for logs and a directory containing the command YAML:

```bash
docker run --net=host \
  -v /home/some-user/logs:/logs \
  -v /home/some-user/testdata:/testdata \
  docker.io/sbezverk/routercommander:latest \
  --router-name=router \
  --username=user \
  --password='pass' \
  --v=5 \
  --commands-file=./testdata/show_cef.yaml
```

The resulting log file is written under the mounted `/logs` directory.
