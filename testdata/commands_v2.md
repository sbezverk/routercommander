## Command YAML Model

This document describes the current command profile schema used by `routercommander`.

## Top-Level Sections

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
        separator: " "
        fields:
          - field_number: 1
            operation: compare_with_value_neq
            value: "0"
        if_triggered_commands:
          - command: show logging last 50
        check_all_results: false

commands:
  - command: show cef drops
    process_result: true
    command_test_ids: [1]
    patterns:
      - pattern_string: 'drops\s+packets\s+:\s+[1-9]\d*'
```

Supported top-level keys:

- `collect`: collect-mode behavior.
- `repro`: repro-mode iteration and global triggered commands.
- `tests`: structured checks keyed by command.
- `commands`: main command list.

## collect

```yaml
collect:
  stop_on_error: false
  process_result: true
```

Fields:

- `stop_on_error`: when true, a batch run stops after a router fails.
- `process_result`: when true, command output is checked against configured patterns and tests in collect mode.

## repro

```yaml
repro:
  times: 100
  interval: 10
  stop_when_triggered: true
  if_triggered_commands:
    - command: show tech-support
      command_timeout: 300
```

Fields:

- `times`: number of main command-group iterations.
- `interval`: seconds between iterations.
- `stop_when_triggered`: stop repro after the first triggered iteration.
- `if_triggered_commands`: global commands collected after any test triggers.

When `repro` is present, command output is processed regardless of `collect.process_result`.

## commands

```yaml
commands:
  - command: show cef drops
    command_timeout: 30
    times: 2
    interval: 10
    wait_before: 1
    wait_after: 1
    location:
      - "0/0/CPU0"
    location_fmt_tmpl: "{{.Location}}"
    location_customized: false
    pipe_modifier: include drops
    debug: false
    process_result: true
    command_test_ids: [1]
    patterns:
      - pattern_string: 'drops\s+packets\s+:\s+[1-9]\d*'
```

Fields:

- `command`: command text to execute.
- `command_timeout`: timeout in seconds.
- `times`: number of times this command is executed.
- `interval`: seconds between command repeats.
- `wait_before`: seconds to wait before executing the command.
- `wait_after`: seconds to wait after executing the command.
- `location`: locations used with location-aware commands.
- `location_fmt_tmpl`: template for formatting each location.
- `location_customized`: when true, `{{.Location}}` in `command` is replaced with each location.
- `pipe_modifier`: pipe text appended to the command.
- `debug`: command-level debug flag.
- `process_result`: process this command's output even when collect-level processing is disabled.
- `patterns`: regex list used to record matching output lines.
- `command_test_ids`: test IDs to execute for this command. If omitted, all tests for the command execute.

## tests

```yaml
tests:
  - command: show cef drops
    command_tests:
      - id: 1
        pattern:
          pattern_string: 'drops\s+packets\s+:\s+[1-9]\d*'
        occurrence: 1
        number_of_occurrences: 1
        separator: " "
        fields:
          - field_number: 1
            operation: compare_with_value_neq
            value: "0"
        if_triggered_commands:
          - command: show logging last 50
        check_all_results: false
```

Fields:

- `command`: command text this test group applies to. It must match a `commands[].command` value.
- `command_tests`: list of tests for the command.
- `id`: test identifier referenced by `command_test_ids`.
- `pattern.pattern_string`: regex used to find matching output.
- `occurrence`: optional one-based match occurrence to check.
- `number_of_occurrences`: expected number of regex matches. A mismatch triggers the test.
- `separator`: character set used to split the matched line for field extraction. Defaults to whitespace.
- `fields`: checks to run against extracted fields.
- `if_triggered_commands`: commands executed immediately when this test triggers.
- `check_all_results`: when true, all configured field checks must trigger.

Field operations:

- `compare_with_previous_neq`
- `compare_with_previous_eq`
- `compare_with_value_neq`
- `compare_with_value_eq`
- `contain_substring`
- `not_contain_substring`

## Simple Collect Example

```yaml
collect:
  process_result: true

commands:
  - command: show platform
    patterns:
      - pattern_string: '^(?:(?:([a-zA-Z_\-0-9\/\(\)]+)\s+){2})(?!.*IOS XR RUN|.*UP|.*OPERATIONAL)'
```

## Simple Repro Example

```yaml
repro:
  times: 30
  interval: 10
  stop_when_triggered: true
  if_triggered_commands:
    - command: show tech-support
      command_timeout: 300

tests:
  - command: ping 10.0.0.1 count 5
    command_tests:
      - id: 1
        pattern:
          pattern_string: 'Success rate is\s+0\s+percent'
        if_triggered_commands:
          - command: show logging last 50

commands:
  - command: ping 10.0.0.1 count 5
    command_test_ids: [1]
```
