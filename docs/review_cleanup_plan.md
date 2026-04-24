# RouterCommander Cleanup Plan

## Purpose

This document organizes the current review findings and follow-up work for
`routercommander` into three groups:

- must-fix before reuse
- reliability improvements
- nice-to-have modernization

The intent is to keep future cleanup practical and focused. The first group is
about issues that can materially affect safety or correctness. The second group
improves robustness and maintainability. The third group is about improving the
tool without changing its core value proposition.

## Must-Fix Before Reuse

These items should be addressed before `routercommander` is reused as a serious
building block for automated incident-time collection.

### 1. SSH host identity handling

Status:

- partially improved
- TOFU support has been introduced
- explicit insecure mode exists

Why it matters:

- this tool sends credentials to routers
- if host identity is not handled safely, a man-in-the-middle can impersonate a
  router and capture credentials or command output

What must be true before reuse:

- host keys are not blindly accepted by default
- TOFU behavior is deterministic
- known-host lookups use one normalized host form
- insecure behavior is explicit and opt-in only

Recommended finish line:

- keep TOFU as default
- keep `--insecure` style behavior as explicit opt-in only
- add tests for:
  - unknown host is added
  - known host with same key succeeds
  - known host with different key fails
  - host normalization is consistent for `host`, `host:22`, and IPv6 forms

### 2. `sendCommand()` timeout panic risk

Why it matters:

- the current implementation uses a goroutine plus channels around a blocking
  `stdout.Read`
- if the main path closes channels while the goroutine is still alive, the
  process can panic

Minimum safe fix:

- do not close `errCh` and `doneCh` from the receiver side
- make them buffered with capacity `1`
- keep the timeout logic simple and avoid redesign unless needed

Why this is enough for now:

- `routercommander` is a short-lived CLI tool, not a daemon
- avoiding crash is much more important than perfect goroutine cleanup

Recommended finish line:

- panic risk removed
- timeout path covered by tests if practical
- behavior documented in code comments

### 3. Router and logger lifecycle cleanup

Why it matters:

- each router creates:
  - an SSH client
  - an SSH session
  - a logger with a worker goroutine
- these resources should be closed when processing ends

Current risk:

- file descriptors remain open until process exit
- logger worker goroutines remain alive
- repeated or long-running use becomes less predictable

Recommended fix:

- add deferred cleanup in `process()`:
  - `defer r.Close()`
  - `defer r.GetLogger().Close()` when logger is not nil

Recommended finish line:

- normal processing path closes sessions and loggers cleanly
- early-return paths also close resources

## Reliability Improvements

These items are not as urgent as the must-fix group, but they would noticeably
improve day-to-day correctness and predictability.

### 1. Routers file parsing

Problem:

- the last router entry is dropped if the file has no trailing newline

Recommended fix:

- use `bufio.Scanner`
- or append the final partial line before breaking on EOF

Why it matters:

- missing one router from a batch run is easy to overlook
- files without trailing newlines are common

### 2. Test suite stability and intent

Current observation:

- parser tests were failing because `reflect.DeepEqual` compared runtime-filled
  fields, not only semantic YAML content
- this was improved by adding a normalizer

Recommended next steps:

- keep parser tests focused on semantic config interpretation
- add small targeted tests for:
  - TOFU host normalization
  - routers file parsing
  - timeout-path non-panic behavior
  - cleanup behavior where practical

### 3. Shared SSH verifier state design

Current state:

- mutex-protected shared globals are acceptable for now

Potential improvement:

- replace package-level globals with a verifier object

Why it would help:

- better testability
- less hidden state
- clearer ownership of:
  - known hosts file
  - in-memory host map
  - insecure mode

This is not strictly required immediately if the current model remains simple,
but it would improve clarity.

### 4. Logging and debug noise cleanup

Examples:

- special-case debug prints in local execution
- noisy informational messages that may not be useful outside development

Why it matters:

- the tool is intended to help during stressful troubleshooting situations
- logs should stay readable and intentional

Recommended direction:

- keep high-value progress logs
- remove or demote ad hoc debugging lines

## Nice-to-Have Modernization

These items are useful but should not block reuse if the must-fix and
reliability items are handled first.

### 1. Config and schema documentation refresh

Recent cleanup aligned the YAML key from `main_command_group` to `commands`.

Nice follow-up work:

- review all examples for consistency
- make the README shorter and more task-oriented
- add one or two minimal examples:
  - simple collect mode
  - simple repro mode

### 2. Command execution abstraction cleanup

The current command execution path is understandable but densely coupled to
prompt parsing and SSH session behavior.

Nice modernization options:

- isolate prompt parsing more explicitly
- separate SSH transport from command result extraction
- make command timeout behavior easier to test

This should be done only if it can be achieved without destabilizing the tool.

### 3. Notification subsystem cleanup

Email support exists and works as a side feature, but it could be modernized.

Possible future work:

- simplify notifier configuration
- make notification execution optional but explicit
- improve attachment/message formatting tests

### 4. CLI ergonomics

Possible future improvements:

- better flag validation messaging
- config-file support if the tool grows further
- clearer separation between local and remote execution modes

These are useful quality improvements, but not urgent.

## Recommended Execution Order

If cleanup is done incrementally, the best order is:

1. finish SSH identity hardening
2. remove `sendCommand()` timeout panic risk
3. close router and logger resources reliably
4. fix routers file parsing
5. add focused tests for the above
6. then consider shared-state cleanup and general modernization

## Practical Reuse Threshold

`routercommander` becomes a much safer candidate for reuse in future incident
automation once these are true:

- TOFU / insecure host-key handling is explicit and tested
- `sendCommand()` cannot panic on timeout
- router sessions and loggers are closed on normal and early-exit paths
- routers file parsing is deterministic

Once those are in place, the tool is much more credible as a component in a
larger incident-response workflow.
