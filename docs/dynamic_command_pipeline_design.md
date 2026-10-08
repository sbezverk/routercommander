# Dynamic Command Pipeline Design

Status: Proposed

## 1. Summary

routercommander currently executes a static, ordered list of commands for
each router. It can collect command output, match regular expressions, extract
fields for tests, execute commands across router locations, and continue
processing multiple routers concurrently.

The missing capability is data flow between commands. A later command cannot
currently use values extracted from an earlier command. This document proposes
an optional, backward-compatible pipeline model that adds:

- command output extraction
- named variables and records
- inherited context such as the current VRF
- dynamic command rendering
- for-each execution over extracted records
- nested dependent commands
- explicit per-step and per-record failure policies
- per-router state isolation

The design is intentionally a sequential workflow with nested fan-out rather
than a general-purpose directed acyclic graph. It is sufficient for dynamic
router investigations while keeping the implementation and configuration
understandable.

## 2. Problem statement

A typical workflow is:

1. Run a discovery command.
2. Parse its output.
3. Extract one or more values.
4. Use those values in later commands.
5. Repeat the later commands for every matching record.
6. Execute those commands across selected router locations.

For example, the attached IOS XR output contains:

~~~text
VRF: METROE-E

Prefix              Next Hop            Interface
------------------- ------------------- ------------------
42.99.250.22/32     10.169.23.89/32 (?) <recursive>
42.189.250.22/32    10.169.14.59/32 (?) <recursive>
51.189.255.184/29   10.169.14.59/32 (?) <recursive>
~~~

The desired workflow is conceptually:

~~~text
for each unresolved route:
    show cef vrf <vrf> <prefix> hardware egress detail at all locations
    show cef <next-hop> hardware egress detail at all locations
~~~

The output contains a VRF header on one line and route records on later lines.
The parser therefore needs both record extraction and inherited context.

## 3. Goals

The implementation should:

1. Preserve all existing static command profiles without changes.
2. Read the password once and process the complete router inventory in one
   process.
3. Keep extracted state isolated per router.
4. Execute dependent commands in deterministic order.
5. Support one or many records from a discovery command.
6. Support values extracted from a previous command in later command text.
7. Reuse the existing location expansion behavior, including all, all-rp, and
   all-lc.
8. Keep one log file per router and add enough workflow context to make the
   log understandable.
9. Make failure behavior explicit and testable.
10. Provide useful diagnostics for malformed output, missing variables, and
    command expansion limits.
11. Avoid requiring a generated command file per router.
12. Allow the feature to be introduced incrementally in small development
    sessions.
13. Support recursively nested workflow steps so a command at any level can
    extract data for the next level.

## 4. Non-goals for the first version

The first version should not attempt to provide:

- an arbitrary DAG scheduler
- cross-router variables
- cross-router joins
- workflow state persisted across process restarts
- dynamic parallelism inside a single router
- a general shell scripting language
- automatic retry of every failed command
- arbitrary evaluation expressions or a general-purpose condition language
- unrestricted command construction from unvalidated router output

Value-based branching is intentionally a stretch goal for the first
implementation. The runtime model should reserve a first-class branch step so
branching can be added without redesigning command execution or record
contexts.

Recursive workflow trees are in scope from the beginning. Arbitrary
cross-linked DAGs are not.

## 5. Current implementation assessment

### 5.1 Existing command execution

The main command group is currently processed in order by
cmd/pipeline.go. Each command is sent through types.Router.ProcessCommand,
then its output may be pattern-matched and tested.

Relevant code:

- cmd/pipeline.go: main command execution and test processing
- pkg/types/router.go: command execution, location expansion, and command
  templating for locations
- pkg/types/types.go: YAML command and test models
- pkg/types/commands.go: YAML loading and regex compilation

### 5.2 Existing output processing

The current implementation can:

- collect raw command results
- find matching lines using regular expressions
- split a matching line into fields
- store field values for comparison tests
- invoke static if_triggered_commands

The current implementation cannot:

- expose extracted fields as variables for later command text
- associate a row with the most recent VRF header
- iterate dependent commands over all extracted rows
- render a command using a record produced by a previous command

The current if_triggered_commands feature executes configured command
objects directly. Those command objects contain static command text and do not
receive the extracted field store as template data.

### 5.3 Existing location support

The router implementation already expands location selectors:

- all
- all-rp
- all-lc
- explicit locations

The pipeline should reuse this behavior instead of implementing a second
location mechanism.

### 5.4 Existing concurrency and password behavior

The application already:

- reads a stdin password once before starting router processing
- limits concurrent router workers
- creates a separate command clone per router
- runs router processing in separate goroutines
- waits for all started routers before exiting

The new runtime context must follow the same per-router boundary. No extracted
values may be stored in package-level variables or shared command definitions.

## 6. Proposed user-facing configuration

### 6.1 Backward compatibility

Existing files using:

~~~yaml
commands:
  - command: show platform
~~~

must continue to work exactly as they do today.

The new pipeline mode should be optional. A command profile should contain
either:

- commands, for the existing static mode; or
- pipeline, for the new dynamic mode.

For the first version, defining both should be rejected with a clear
configuration error. This avoids ambiguity about execution order.

The existing collect section remains valid in both modes:

~~~yaml
collect:
  process_result: false
~~~

Router processing remains best-effort across the inventory. A failure for one
router is recorded and does not prevent other routers from starting or
finishing. The process returns a non-zero status if any router fails.

Recursive pipelines should support profile-level safety limits:

~~~yaml
pipeline_limits:
  max_depth: 8
  max_commands: 5000
~~~

The default depth should cover the expected four-to-six-level troubleshooting
cases while still protecting against accidental runaway configuration.

### 6.2 Conceptual pipeline example

The following is a proposed configuration shape. Exact field names can be
finalized during implementation, but the nesting and execution semantics
should remain similar.

~~~yaml
collect:
  process_result: false

pipeline:
  - id: discover_ipv4_unresolved
    run:
      command: show cef vrf all ipv4 unresolved
      location:
        - 0/RP0/CPU0
      process_result: true

    extract:
      context:
        - pattern: '^VRF:\s+(?P<vrf>[A-Za-z0-9_.:-]+)\s*$'

      records:
        name: route
        pattern: '^\s*(?P<prefix>[0-9A-Fa-f:.]+/\d+)\s+(?P<next_hop>[0-9A-Fa-f:.]+/\d+)'
        inherit:
          - vrf
        deduplicate_by:
          - vrf
          - prefix
          - next_hop

    for_each: route
    steps:
      - id: inspect_prefix_egress
        run:
          command: show cef vrf {{.vrf}} {{.prefix}} hardware egress detail
          location:
            - all
        on_error: continue_record

      - id: inspect_next_hop_egress
        run:
          command: show cef {{.next_hop}} hardware egress detail
          location:
            - all
        on_error: continue_record
~~~

The example intentionally processes every matching route. The initial
implementation must also support semantic filtering after extraction, so a
workflow can restrict records by VRF, prefix, address family, or another
captured field without changing the extraction regular expression.

### 6.3 Step types

The core implementation should support three logical step types. All steps
must be recursively composable through a shared steps collection:

#### Command step

Runs one router command and optionally returns raw results for extraction.

It should reuse the existing command options where possible:

- command
- command_timeout
- times
- interval
- wait_before
- wait_after
- location
- location_fmt_tmpl
- location_customized
- pipe_modifier
- debug
- process_result

#### Extraction step

Reads the output of a command and produces:

- scalar variables, or
- a named collection of records

The first version needs context captures and record captures to support
headers such as VRF: followed by rows that inherit the current VRF.

#### For-each step

Iterates over a named record collection and executes nested steps with the
current record available as template data.

For-each steps may contain further command, extraction, for-each, and branch
steps. This recursive composition is required from the beginning because
router troubleshooting workflows can require four, five, or six dependent
levels.

The executor must enforce a maximum nesting depth and a maximum generated
command count. These limits protect against accidental recursive expansion
without imposing a fixed two-level model.

Conceptually:

~~~text
level 1: discovery command
  level 2: command using discovery values
    level 3: command using level 2 values
      level 4: command using level 3 values
        level 5: command using level 4 values
          level 6: command using level 5 values
~~~

#### Branch step (stretch goal)

A branch step evaluates captured values from the current runtime context and
executes the steps belonging to the first matching case. It should be usable
at the top level of a pipeline or inside a for-each step.

Conceptually:

~~~yaml
- id: classify_route
  branch:
    cases:
      - when:
          field: field_1
          equals: blah
        steps:
          - run:
              command: show command-two

      - when:
          field: field_1
          equals: halb
        steps:
          - run:
              command: show command-three

    default:
      - run:
          command: show command-default
~~~

The first branch implementation should use structured predicates rather than
evaluating arbitrary expressions. Candidate predicates are:

- equals
- not_equals
- contains
- matches
- exists
- in

Compound predicates such as all and any can be added after the simple
predicates are stable.

### 6.4 Recursive pipeline example

The configuration model must permit a step to contain another step group at
any depth. The following abbreviated example shows a child command producing
records for a deeper child command:

~~~yaml
pipeline:
  - id: level_one_discovery
    run:
      command: show discovery-one
    extract:
      records:
        name: first_records
        pattern: '^(?P<first_value>\S+)'
    for_each: first_records
    steps:
      - id: level_two_command
        run:
          command: show discovery-two {{.first_value}}
        extract:
          records:
            name: second_records
            pattern: '^(?P<second_value>\S+)'
        for_each: second_records
        steps:
          - id: level_three_command
            run:
              command: show discovery-three {{.second_value}}
            extract:
              records:
                name: third_records
                pattern: '^(?P<third_value>\S+)'
            for_each: third_records
            steps:
              - id: level_four_command
                run:
                  command: show detail {{.third_value}}
~~~

The example is intentionally generic. The actual router troubleshooting
workflow may continue to level five or six using the same structure. No new
Go type or executor branch should be required for each additional level.

## 7. Runtime data model

The runtime state should be per router and per pipeline invocation.

Conceptually:

~~~go
type Record map[string]string

type RunContext struct {
    Variables   map[string]string       // pipeline-global scalar values
    Collections map[string][]Record
    Current     Record                  // current record at the active for_each level
    RouterName  string
    Depth       int
    StepPath    []string
    MaxDepth    int
    CommandsRun int
    MaxCommands int
}
~~~

Recommended rules:

- Variables contains pipeline-global scalar values. It is per router and per
  pipeline invocation, not process-global and not shared between routers.
- Collections contains named record lists.
- Current is the record visible inside a for-each.
- RouterName is metadata, not extracted user data.
- A child record context should inherit parent variables.
- A child record should not mutate the parent record.
- A new context must be created for every router.
- Context should be passed explicitly to execution functions.
- Depth should increase when entering a nested step group and decrease when
  leaving it.
- StepPath should identify the current nested path for diagnostics and logs.
- MaxDepth and MaxCommands should be checked before expanding nested work.

Avoid using map[string]interface{} everywhere. Use string values for captured
command tokens initially. This keeps rendering predictable and avoids type
ambiguity in templates.

### 7.1 Recursive step execution

The executor should operate on a list of steps through one recursive function,
conceptually:

~~~text
executeSteps(context, steps):
    for step in steps:
        executeStep(context, step)

executeStep(context, step):
    if step is command:
        run command
    if step is extract:
        extract values
    if step is for_each:
        for record in collection:
            childContext = context.withCurrent(record)
            executeSteps(childContext, step.steps)
    if step is branch:
        select case
        executeSteps(context, selectedCase.steps)
~~~

The implementation should not create separate executor types for level 1,
level 2, level 3, and so on. A recursive step tree allows four-to-six-level
troubleshooting workflows without another architectural change.

Every recursive entry must:

1. check the configured maximum depth;
2. append the step ID to StepPath;
3. execute child steps;
4. restore the parent context on return.

The first configuration may use a default maximum depth such as 8, with an
explicit profile override. The value should be high enough for real
troubleshooting while still catching accidental cycles or runaway nesting.

### 7.2 Global scalar values, collections, and correlation

Dynamic troubleshooting frequently produces more than one independent value
that must be used by a later command. The ACL/BVI investigation is a concrete
example:

~~~text
ACL usage
  -> affected BVI
    -> BVI IFH
      -> filtered extlif object
physical interface
  -> hosting NPU
internal TCAM
  -> INGRESS_ACL_L3_IPV4 DB ID
hosting NPU + DB ID
  -> FIA diagshell command
~~~

The pipeline must distinguish three different kinds of runtime data:

1. A global scalar is one value that is valid throughout the current router
   pipeline, such as a router name or a uniquely selected NPU. It belongs in
   `RunContext.Variables`.
2. A collection is zero or more records, such as affected BVIs, IFHs, NPUs,
   or DB IDs. It belongs in `RunContext.Collections` and must be iterated with
   `for_each`.
3. A correlated record contains two or more values that belong to the same
   execution target, such as `{npu: 0, db_id: 32}`. It must be represented as
   one record before the final command is rendered.

The executor must not automatically promote every capture to
`RunContext.Variables`. Automatic promotion is ambiguous when a command
produces multiple locations or multiple records, and a later record could
silently overwrite an earlier value. Promotion should be explicit and should
define cardinality and overwrite behavior.

A future scalar-export step can use a shape similar to:

~~~yaml
- id: publish_hosting_npu
  export:
    from: hosting_npus
    field: npu
    as: hosting_npu
    require: exactly_one
~~~

The export operation should fail if `exactly_one` is not satisfied. Other
possible policies (`first`, `last`, or `all_equal`) should not be implicit;
they need to be selected in configuration because they change troubleshooting
semantics. Values exported inside a `for_each` should normally be rejected,
unless the operation explicitly proves that all iterations produce the same
value. This prevents nondeterministic last-writer-wins behavior.

Scalar export alone does not solve a multi-value dependency. If several NPUs
and several DB IDs exist, the pipeline needs an explicit correlation step or
an equivalent multi-input iteration construct. The preferred result is a
collection such as:

~~~yaml
diagnostic_targets:
  - npu: "0"
    db_id: "32"
~~~

which can be consumed by:

~~~yaml
- id: inspect_fia_table
  for_each:
    in: diagnostic_targets
    steps:
      - id: inspect_fia_db
        run:
          command: show controllers fia diagshell {{.npu}} "diag field T db={{.db_id}}"
~~~

The correlation operation must define how records are paired. Supported
initial forms may include:

- a `combine`/`join` step with an explicit key;
- a `for_each` step that accepts multiple named collections and produces a
  Cartesian product only when explicitly requested; or
- an extraction that captures all fields from one command output when the
  source output already contains the relationship.

The implementation must not silently pair records by slice index. If the
relationship is one-to-one, one-to-many, or keyed by a field, that relationship
must be declared and validated. A Cartesian product must also be explicit
because it can multiply command count rapidly.

For the supplied ACL/BVI fixture, the current implementation can execute the
workflow with NPU `0` explicitly selected from the VOQ result and the DB ID
rendered dynamically. This is a valid fixed-box workaround, but it is not the
generic solution. The reusable version must discover the physical interface,
hosting NPU, and DB ID, then produce correlated `{npu, db_id}` targets before
running `diagshell`.

Scope and rendering rules for this design are:

- `Variables` are visible to every later step in the same router pipeline and
  to recursively-created child contexts.
- `Collections` are named runtime data and are visible according to the
  collection-scope rules of the enclosing step group.
- `Current` is the active record for the current `for_each` level.
- A child record may use its own fields and explicitly inherited parent
  values, but a child must not accidentally replace a needed parent field.
- If a command needs values from two collections, the pipeline must first
  create a combined record or explicitly export a scalar; templates must not
  reach into arbitrary collections by hidden side effects.

This distinction is required for the common pattern where one command
discovers a physical interface/NPU relationship while another discovers a DB
ID. It also applies to route/next-hop, bundle/member, interface/VRF, and
similar troubleshooting workflows.

### 7.3 Repeated command groups

The existing `times` and `interval` fields repeat one command. Troubleshooting
procedures may eventually need to repeat a group of commands as one unit, for
example:

~~~yaml
- id: sample_fia_state
  repeat:
    times: 5
    interval: 2
    steps:
      - id: read_signature_block
        run:
          command: show controllers fia diagshell 0 "diag pp sig block=irpp"
      - id: read_last_field
        run:
          command: show controllers fia diagshell 0 "diag field last"
~~~

The intended semantics are sequential commands within one group iteration,
with the interval applied between complete group iterations. This differs from
two independent commands each using `times: 5`, which may interleave or finish
at different times depending on the executor. Group repetition must preserve
the normal recursive step scope, collect each command's results under its step
ID, and count every generated command against `max_commands`.

This is a future construct, not part of the current `PipelineStep` model. The
two FIA commands in the ACL/BVI fixture currently use independent `times: 5`
and `interval: 2` settings as a compatible approximation.

## 8. Extraction design

### 8.1 Named captures

Use named regular expression groups rather than relying only on numeric field
positions:

~~~text
(?P<prefix>[0-9A-Fa-f:.]+/\d+)
~~~

In this expression, prefix is the name of a regular-expression capture
group. It becomes a field in the extracted record. It is not substituted into
the regular expression itself.

Given this input line:

~~~text
42.99.250.22/32     10.169.23.89/32 (?) <recursive>
~~~

This record pattern:

~~~text
^\s*(?P<prefix>[0-9A-Fa-f:.]+/\d+)\s+(?P<next_hop>[0-9A-Fa-f:.]+/\d+)
~~~

produces:

~~~yaml
prefix: 42.99.250.22/32
next_hop: 10.169.23.89/32
~~~

The extracted prefix can then be used as a template variable in a dependent
command:

~~~yaml
steps:
  - id: inspect_prefix
    run:
      command: show cef {{.prefix}} hardware egress detail
      location:
        - all
~~~

If the record also inherits the current VRF context, the complete record and
dependent command become:

~~~yaml
record:
  vrf: METROE-E
  prefix: 42.99.250.22/32
  next_hop: 10.169.23.89/32

command: show cef vrf {{.vrf}} {{.prefix}} hardware egress detail
~~~

The implementation should preserve captured values as strings, validate them
before rendering, and fail clearly if a required capture is missing.

### 8.1.1 Go extraction example

The following example shows how the pipeline extractor can process the named
capture expression in Go. Go's regexp package exposes the capture names
through SubexpNames and the corresponding values through
FindStringSubmatch.

~~~go
package pipeline

import (
    "bytes"
    "fmt"
    "regexp"
    "strings"
    "text/template"
)

type Record map[string]string

var routePattern = regexp.MustCompile(
    `^\s*(?P<prefix>[0-9A-Fa-f:.]+/\d+)\s+` +
        `(?P<next_hop>[0-9A-Fa-f:.]+/\d+)`,
)

func extractNamedRecord(line string, pattern *regexp.Regexp) (Record, bool) {
    matches := pattern.FindStringSubmatch(line)
    if matches == nil {
        return nil, false
    }

    record := make(Record)
    names := pattern.SubexpNames()
    for index, name := range names {
        if index == 0 || name == "" || index >= len(matches) {
            continue
        }
        record[name] = strings.TrimSpace(matches[index])
    }

    return record, true
}

func example() error {
    line := "42.99.250.22/32     10.169.23.89/32 (?) <recursive>"

    record, ok := extractNamedRecord(line, routePattern)
    if !ok {
        return fmt.Errorf("line did not contain a route record")
    }

    // A context value captured from an earlier VRF header is inherited by
    // the route record before the record is passed to child steps.
    record["vrf"] = "METROE-E"

    commandTemplate, err := template.New("command").
        Option("missingkey=error").
        Parse("show cef vrf {{.vrf}} {{.prefix}} hardware egress detail")
    if err != nil {
        return err
    }

    var rendered bytes.Buffer
    if err := commandTemplate.Execute(&rendered, record); err != nil {
        return err
    }

    fmt.Println(record["prefix"])
    fmt.Println(record["next_hop"])
    fmt.Println(rendered.String())
    return nil
}
~~~

Expected values:

~~~text
record["prefix"]   = "42.99.250.22/32"
record["next_hop"] = "10.169.23.89/32"
record["vrf"]      = "METROE-E"
rendered command   = "show cef vrf METROE-E 42.99.250.22/32 hardware egress detail"
~~~

The production extractor should generalize extractNamedRecord rather than
implementing a separate function for each router command. It should also
validate captured values and enforce the pipeline's maximum record and
command-expansion limits.

The extractor should discover capture names from the compiled regular
expression and build a Record for every match.

This is more maintainable than requiring users to count whitespace-separated
fields, especially when router output changes column spacing.

### 8.1.2 Record filtering

The record regular expression performs structural filtering: it determines
which lines have the expected record shape. The initial implementation must
also support semantic filtering after a record has been extracted and its
inherited context has been applied.

For example:

~~~yaml
records:
  name: route
  pattern: '^\s*(?P<prefix>[0-9A-Fa-f:.]+/\d+)\s+(?P<next_hop>[0-9A-Fa-f:.]+/\d+)'
  inherit:
    - vrf
  where:
    - field: vrf
      equals: NMNET
    - field: prefix
      matches: '^10\.'
~~~

All predicates in `where` are combined with AND semantics. A record must
match every predicate to enter the named collection. The predicates may refer
to fields captured by the record pattern or inherited from the current
context.

Filtering should occur in this order:

1. match and extract the record;
2. apply inherited context values;
3. evaluate `where` predicates;
4. apply deduplication;
5. add the accepted record to the collection.

The existing structured predicate model should be reused for filtering and
branching. Compound `all` and `any` predicates remain available for a later
extension if simple lists of AND predicates are insufficient.

### 8.2 Context inheritance

The extractor must support a current context value:

1. Scan output in order.
2. When a context pattern matches, update the current context.
3. When a record pattern matches, copy inherited context values into the new
   record.
4. Add the record-specific captures.
5. Continue scanning.

For the example:

~~~text
VRF: METROE-E                 -> current vrf = METROE-E
42.99.250.22/32 ...           -> record inherits vrf
42.189.250.22/32 ...          -> record inherits vrf
51.189.255.184/29 ...         -> record inherits vrf
~~~

Context should reset when a new context header is encountered. A blank line
may optionally reset context if configured, but should not do so implicitly in
the first version unless the output format requires it.

### 8.3 IPv4 and IPv6

Captured values should remain strings. The extractor should not normalize or
reinterpret addresses unless explicitly configured.

The initial token validation should allow:

- IPv4 prefixes
- IPv6 prefixes
- IPv4 next hops
- IPv6 next hops
- VRF names containing letters, numbers, underscores, dots, hyphens, and
  colons where appropriate

Separate validators can be added later if a workflow needs strict address
validation.

### 8.4 Multiple matches

Record order should follow output order. This gives deterministic command and
log order.

The extractor should support optional deduplication:

~~~yaml
deduplicate_by:
  - vrf
  - prefix
  - next_hop
~~~

The pipeline should have a global record-safety ceiling, and each extraction
step may optionally define a tighter local limit:

~~~yaml
pipeline_limits:
  max_records: 500

records:
  max_records: 100
~~~

`pipeline_limits.max_records` is the hard per-router ceiling for records
produced by extraction and collection transformations such as `join`. A
`record_spec.max_records` value is an optional per-extraction limit applied
after filtering and deduplication. The effective limit is the smaller of the
two values. A zero or omitted per-extraction value inherits the global limit.
This prevents one-to-many joins or unusually large extractions from creating
unbounded collections while still allowing a noisy extraction to use a
tighter local limit. The global `max_commands` limit remains the protection
against total recursive command expansion across all records and locations.

### 8.5 No-match behavior

The pipeline must make no-match behavior explicit:

- skip: log that no records were found and complete successfully
- fail_router: mark the current router as failed
- fail_workflow: stop the current workflow according to the configured
  router-level policy

The recommended default for a discovery step is skip, because some routers
may legitimately have no matching entries.

### 8.6 Successful no-result stop

The pipeline must distinguish a command that successfully returned no result
entries from a command execution failure. Zero returned results are a valid
control signal: they mean that the parent discovery path has nothing to
process, so dependent work must not be generated.

This behavior is separate from extractor no-match behavior:

- **Zero command results:** the `Run` step completed successfully, but
  `ProcessCommand` returned no `CmdResult` entries. Stop the current router's
  pipeline without marking the router as failed.
- **Non-empty command output with no matching records:** the command did
  return data, but the extractor found no records. Store an empty collection
  and apply the configured extraction/no-match policy. A later `for_each` over
  that collection naturally performs zero iterations.
- **Command error:** the router returned an execution error. Apply the
  normal `on_error` policy and report the failure according to that policy.

The pipeline does not inspect or interpret the contents of `CmdResult.Result`.
The command execution layer removes nil and zero-length result entries before
returning collected results. Whitespace-only payloads are therefore retained
as results and are not treated specially. When a command runs against multiple
locations or iterations, any retained result is sufficient for the step to
continue; locations or iterations with zero-length results are omitted.

The recommended propagation scope is:

- at pipeline scope, stop the current router's remaining pipeline steps and
  complete the router successfully;
- inside a `for_each`, stop the complete current router pipeline rather than
  continuing with another record; and
- never stop processing on other routers because one router reached this
  successful stop condition.

The signal is control flow, not an error. It must not be passed through
`on_error`, must not be wrapped in a router-failure diagnostic, and must not
be reported as a partial command failure. The implementation may represent it
with an internal sentinel or a small typed control-flow result, but callers
must be able to distinguish it from `ErrPipelineSkipRecord` and ordinary
execution errors.

## 9. Command rendering

Dynamic command rendering should use Go's text/template, not html/template,
because the target is CLI text rather than HTML.

Recommended behavior:

~~~go
template.New("command").Option("missingkey=error")
~~~

A missing variable must fail before sending a command to the router. The error
should include:

- router name
- pipeline step ID
- record identifier
- missing variable name

The rendered command should be produced on a temporary command copy. Do not
mutate the shared parsed command definition, because the same base profile may
be used to start multiple router workers.

Reserved template values may include:

- .RouterName
- .Location
- current record fields such as .vrf, .prefix, and .next_hop

Reserved names should be documented and protected from accidental overwrite.

## 10. Location execution

The pipeline should reuse the existing location behavior:

~~~yaml
location:
  - all
~~~

For each rendered command, the router layer expands all to the router's known
locations and executes the command for each location.

The pipeline engine should not manually append location text. It should pass
the rendered command through the existing ProcessCommand path.

The implementation should verify the following cases:

- dynamic command with location: [all]
- dynamic command with explicit locations
- dynamic command using location_customized
- dynamic command with location_fmt_tmpl
- router with no discovered locations

If deterministic logs are required, the pipeline should sort expanded
locations before execution or the router location getters should be made
deterministic. Existing location collections are backed by maps, so their
iteration order should not be assumed.

## 11. Execution semantics

Within one router, execution should be sequential:

~~~text
discovery command
  -> extraction
  -> record 1:
       dependent command A
       dependent command B
  -> record 2:
       dependent command A
       dependent command B
~~~

The dependency tree may be deeper than the example above:

~~~text
discover
  -> extract route
    -> inspect route
      -> extract next-hop details
        -> inspect next-hop details
          -> branch on result
            -> collect remediation evidence
~~~

Each child step executes only after its parent step has completed and produced
the context required by that child. A child may create a new collection that
is consumed by a nested for-each at the next level.

Across routers, existing concurrency remains active. The pipeline should not
create additional goroutines inside one router during the first version.

This provides:

- predictable command order
- simple log correlation
- no race conditions in the runtime context
- bounded concurrency through the existing router worker limit
- bounded recursion through maximum depth and command-expansion limits

The process should still read stdin once and run all routers in one
application invocation.

### 11.1 Conditional branching

Branching is a planned stretch goal, not a requirement for the first vertical
slice. The runtime architecture should nevertheless treat it as a normal
pipeline step rather than as a special case of if_triggered_commands.

Branch evaluation should follow these rules:

1. Evaluate cases in configuration order.
2. Execute only the first matching case.
3. Execute default steps when no case matches and a default is configured.
4. Do nothing, with a diagnostic log, when no case matches and no default is
   configured.
5. Allow branch steps to contain command, extraction, and for-each steps.
6. Make branch variables available to later steps in the same sequential
   context.
7. Keep branch state local to the current router and current record.

Branching inside a for-each step should evaluate independently for every
record. Branching outside a for-each step should evaluate once for the
pipeline context.

The initial condition evaluator should compare strings and should not execute
arbitrary Go, shell, or template expressions. This keeps behavior
predictable and prevents command-profile authors from unintentionally
creating an unsafe expression language.

## 12. Error handling

There are three separate error scopes.

### 12.1 Router-level errors

Router-level failures are handled independently from step and record failures.

Required behavior:

- continue starting and processing other routers
- return a non-zero process status if any router failed
- report a final summary with successful and failed router counts

### 12.2 Record-level errors

If a dependent command fails for one extracted record, the workflow should be
able to choose whether to:

- stop the current router
- skip the remaining steps for the current record and continue with the next
  record
- continue the remaining steps for the current record

Recommended first-version policy:

- default stop_router for unexpected command failures
- allow continue_record for investigative follow-up commands

### 12.3 Step-level errors

Rendering failures, invalid templates, invalid extracted values, and missing
variables should normally stop the current router. They indicate a workflow
configuration or parser problem rather than a transient router failure.

Every error should include:

- router
- pipeline ID
- step ID
- record index or record key where applicable
- command text or command template
- underlying error

## 13. Safety and limits

Values extracted from a router are inserted into command text. The first
version must not treat arbitrary output as trusted command syntax.

Recommended controls:

1. Validate captured values by type or allowlist before rendering.
2. Reject whitespace and command separators in values intended to be one CLI
   token.
3. Reject empty required values.
4. Use missingkey=error.
5. Limit record count.
6. Limit nesting depth.
7. Limit total generated commands per router.
8. Detect and reject cyclic or self-referential configuration.
9. Log the rendered command only after masking any future sensitive fields.
10. Never make password or credential values available to templates.

The initial use case only needs VRF names and IP prefixes, so conservative
token validation is practical.

## 14. Logging and result reporting

Each router should continue to receive its own log file. Dynamic execution
should add structured breadcrumbs around command output:

~~~text
[pipeline=discover_ipv4_unresolved step=discover]
[router=example-router]
executing discovery command

[pipeline=discover_ipv4_unresolved record=1 vrf=METROE-E prefix=42.99.250.22/32]
executing: show cef vrf METROE-E 42.99.250.22/32 hardware egress detail location 0/0/CPU0
~~~

At the end of the process, the application should report:

- total routers selected
- routers completed successfully
- routers failed
- records discovered per router
- dependent commands attempted
- dependent commands failed

The detailed output remains in the existing per-router log files.

## 15. Suggested implementation structure

### 15.1 Configuration models

Likely files:

- pkg/types/types.go
- pkg/types/commands.go
- new pkg/types/pipeline.go

Add models for:

- pipeline steps
- command step configuration
- extraction configuration
- context captures
- record captures
- error policies
- branch steps and structured predicates
- recursive child steps
- pipeline depth and command-expansion limits

Keep the existing Command, Commander, Collect, and test models intact where
possible.

### 15.2 Runtime engine

Recommended location:

- new pkg/pipeline package

Responsibilities:

- execute pipeline steps
- recursively execute nested child steps
- manage RunContext
- render commands
- call types.Router.ProcessCommand
- extract records
- evaluate branch predicates
- apply error policies

The cmd package should remain responsible for:

- flags
- router inventory
- password handling
- worker concurrency
- notifier setup
- process exit status

The existing cmd/pipeline.go can continue to support legacy command mode and
delegate new profiles to the pipeline package.

### 15.3 Router integration

Do not duplicate SSH or location execution in the new engine. Use the existing
types.Router interface and ProcessCommand method.

The engine should receive a router interface and a logger/context rather than
constructing SSH sessions itself.

### 15.4 Parser validation

Configuration validation should happen before any router sessions are started.
Validate:

- exactly one of commands and pipeline
- unique pipeline IDs
- unique extraction collection names
- valid regular expressions
- valid templates
- valid error policy values
- valid branch predicates
- valid location configuration
- positive expansion limits
- positive recursion depth limits
- recursively valid child step definitions

This avoids discovering configuration errors halfway through a 100-router run.

## 16. Development plan

The work is intentionally divided into small, independently testable slices.
Each slice should leave the repository buildable and the existing tests
passing.

### Phase 0: Requirements and fixture

Estimated effort: 1–2 hours.

Tasks:

- Decide whether all matching records are processed by default.
  - [SB] all matching records are processed by default.
- Decide whether the first release needs record filters.
  - [SB] Required in the initial implementation. Filtering is applied after
    extraction and inherited-context application, using structured predicates.
- Add the attached output as a reduced test fixture.
  - [SB]
```shell
show cef vrf all ipv4 unresolved location 0/RP0/CPU0

Wed Sep 23 20:10:31.620 GMT

VRF: NMNET
_____________

Prefix              Next Hop            Interface
------------------- ------------------- ------------------
10.240.16.84/31     10.169.18.75/32 (?) 
10.240.16.112/31    10.169.18.75/32 (?) 
10.244.162.194/31   10.169.18.75/32 (?) 
10.244.162.198/31   10.169.18.75/32 (?) 

```

```shell
show cef vrf all retry-db location 0/RP0/CPU0

Wed Sep 23 20:10:32.214 GMT
 --------------------    ----------   -----   ----------   ---------------------
 Obj-Type                Retry        Retry   Scheduling   Timestamp       
                         Flags        Count   Class                        
 --------------------    ----------   -----   ----------   ---------------------
 PATHLIST                0x4000000    161     Slow         Not Yet Recorded    
    Error code: 0x4ff30200, 'Subsystem(8166)' detected the 'warning' condition 'Code(1)'
    PATHLIST pl:0x309f9e19f0 paths:1 pl-type:Encap-shared
    1st prefix dependent: NMNET 0xe0000004 10.240.16.84/31 leaf:0x30a4ae9628 
 --------------------    ----------   -----   ----------   ---------------------
 Obj-Type                Retry        Retry   Scheduling   Timestamp       
                         Flags        Count   Class                        
 --------------------    ----------   -----   ----------   ---------------------
 PATHLIST                0x4000000    161     Slow         Not Yet Recorded    
    Error code: 0x4ff30200, 'Subsystem(8166)' detected the 'warning' condition 'Code(1)'
    PATHLIST pl:0x309f9e19f0 paths:1 pl-type:Encap-shared
    1st prefix dependent: NMNET 0xe0000004 10.240.16.84/31 leaf:0x30a4ae9628 
 --------------------    ----------   -----   ----------   ---------------------
 Obj-Type                Retry        Retry   Scheduling   Timestamp       
                         Flags        Count   Class                        
 --------------------    ----------   -----   ----------   ---------------------
 PATHLIST                0x4000000    161     Slow         Not Yet Recorded    
    Error code: 0x4ff30200, 'Subsystem(8166)' detected the 'warning' condition 'Code(1)'
    PATHLIST pl:0x309f9e19f0 paths:1 pl-type:Encap-shared
    1st prefix dependent: NMNET 0xe0000004 10.240.16.84/31 leaf:0x30a4ae9628 
```
- Write expected extracted []Records.

  - [SB] For "show cef vrf all ipv4 unresolved location 0/RP0/CPU0"
    - VRF: NMNET, Prefix:  10.240.16.84/31, NextHop: 10.169.18.75/32
    - VRF: NMNET, Prefix:  10.240.16.112/31, NextHop: 10.169.18.75/32
    - VRF: NMNET, Prefix:  10.244.162.194/31, NextHop: 10.169.18.75/32
    - VRF: NMNET, Prefix:  10.244.162.198/31, NextHop: 10.169.18.75/32
  - [SB] For "cef vrf all retry-db location 0/RP0/CPU0"
    - VRF: NMNET, VRFID: 0xe0000004, ErrorCode: 0x4ff30200, PL: 0x309f9e19f0, PLType: Encap-shared, PrefixDependent:  10.240.16.84/31, Leaf: 0x30a4ae9628

- Write expected generated commands for one and multiple records.
  - [SB] Generated commands:
     - show bgp vrf NMNET 10.240.16.84/31  detail
     - show cef vrf NMNET 10.240.16.84/31  detail
     - show cef 10.169.18.75/32 det

Deliverable:

- fixture and acceptance examples, with no runtime code changes.

### Phase 1: Baseline regression coverage

Estimated effort: 1–2 hours.

Tasks:

- Run existing tests.
- Add a legacy command profile regression test.
- Add a location expansion regression test.
- Add a test proving per-router command state is isolated.

Deliverable:

- a baseline that protects current behavior before adding pipeline logic.

### Phase 2: Pipeline configuration model

Estimated effort: 2–3 hours.

Tasks:

- Add the new YAML structures.
- Parse a pipeline profile.
- Reject ambiguous profiles containing both commands and pipeline.
- Validate IDs, policies, regexes, templates, and recursively nested steps.
- Parse and validate maximum depth and maximum command-expansion limits.
- Add parser unit tests.

Deliverable:

- a parsed pipeline model with no execution yet.

### Phase 3: Template rendering

Estimated effort: 2–3 hours.

Tasks:

- Implement context-aware command rendering in the following order:

  1. Define the render input and scope contract. Build a render data object
     from the current `RunContext`, including scalar context variables,
     fields from the current record, router metadata, and the current
     location. The first implementation should support the existing examples
     such as `{{.vrf}}`, `{{.prefix}}`, and `{{.next_hop}}`, plus documented
     reserved values such as `.RouterName` and `.Location`. Repro iteration
     metadata is intentionally excluded from the pipeline render contract.
     Document whether a record field shadows a context field with the same
     name; the recommended rule is to reject collisions during validation or
     provide an explicit namespace rather than silently choosing one.

  2. Establish scope lookup and lifetime. A top-level step can use values
     collected earlier in the same router pipeline. A `for_each` child gets a
     copy of the parent context plus the current record fields. Nested
     `for_each`, branch, and recursively executed steps must receive the scope
     that the executor defines for them; rendering must not read global or
     another router's state. An absent field must remain distinguishable from
     an intentionally empty string.

  3. Render from a per-execution command copy. Never replace `Command.Cmd` in
     the parsed profile or in a command shared by another router worker. Clone
     the command, render the clone, and preserve the original timeout,
     iteration, pipe, debug, and location settings. This is required because
     router processing is concurrent and because a single pipeline step may be
     executed once per record.

  4. Parse templates with `text/template` and fail on missing values. Use a
     cached parsed template when available, configured with
     `Option("missingkey=error")`. Template syntax errors should be found by
     configuration validation; missing fields are runtime errors because the
     available record and context depend on the path taken through the
     pipeline. Do not silently render a missing field as `<no value>`.

  5. Apply the render operation at the command boundary. The sequence should
     be: select the step and scope, construct render data, render the command,
     validate the resulting command text, then call the existing command
     execution path. Extraction must receive the raw command result and must
     not parse the template text itself. This keeps command execution,
     response collection, and extraction responsibilities separate.

  6. Define command-value validation after rendering. Reject values that are
     missing, or that would create an invalid command according to the
     platform-independent rules selected for the pipeline (for example an
     empty required token). Do not impose platform-specific location syntax
     in the generic model layer. Keep shell/CLI quoting rules explicit; a
     value containing spaces or separators must either be valid by contract or
     be escaped by a later, platform-aware layer.

  7. Reconcile command templates with location expansion. A regular
     `location: all`, `all-rp`, `all-lc`, or explicit location must continue
     through the existing location expansion path. For
     `location_customized` and `location_fmt_tmpl`, render the command once
     per concrete location with both the pipeline data and `.Location`, and
     ensure the location is not appended a second time. The behavior for a
     dynamic command must be identical whether it expands to one location or
     many locations.

  8. Add actionable diagnostics. A rendering error should identify the
     router, pipeline step ID, step path, record collection/record identity
     when applicable, missing field, and the original template. Distinguish a
     configuration/template-parse error from a runtime missing-value error so
     `on_error` can apply the intended policy.

  9. Verify legacy compatibility before connecting the pipeline executor.
     Static legacy commands must produce exactly the same command text and
     location behavior as before. The new renderer should be introduced at a
     narrow boundary so the legacy `MainCommandGroup` path remains unchanged
     unless it explicitly opts into templating.

  10. Add tests before moving to extraction/execution integration. Cover
      static text, scalar context, current-record fields, nested scopes,
      reserved values, missing keys, malformed templates, empty rendered
      values, command-copy immutability, concurrent independent renders, and
      all location modes. Assert the exact command sent to the router and the
      exact error text or structured error fields.

Deliverable:

- a renderer with an explicit scope contract, deterministic tests that
  produce the exact expected CLI text, and proof that rendering does not
  mutate the shared parsed command definition.

### Phase 4: Extraction engine

Estimated effort: 5–8 hours, preferably split into two or three small
implementation sessions.

The purpose of this phase is to convert the raw output already stored in
`RunContext.StepResults` into a named `RunContext.Collections` entry. It does
not execute commands and it does not render templates; Phase 5 will connect
the extractor to the recursive executor. Keeping this boundary explicit makes
the extraction logic testable with router output fixtures and prevents it
from depending on a live router.

The data flow for one extraction step is:

~~~text
PipelineExtract.FromStepID
        |
        v
RunContext.StepResults[from_step_id]
        |
        |  for each StepResult, in stored order
        v
split output into lines
        |
        +--> context pattern match --> update current scan context
        |
        +--> record pattern match --> captures + inherited context
                                      |
                                      v
                              where predicates
                                      |
                                      v
                              deduplication
                                      |
                                      v
                              max_records
                                      |
                                      v
RunContext.Collections[records.name]
~~~

Implement the phase in the following order.

1. Define the extractor boundary.

   Add the extraction implementation in the pipeline package, for example
   `pkg/pipeline/extract.go`. Use a small function with an explicit input and
   output contract, such as:

   ~~~go
   func extractRecords(
       ctx *types.RunContext,
       extraction *types.PipelineExtract,
   ) ([]types.Record, error)
   ~~~

   The function should:

   - reject a nil runtime context or extraction specification;
   - look up `extraction.FromStepID` in `ctx.StepResults`;
   - process every stored `StepResult`, not only the first location or first
     command iteration;
   - preserve the order of `StepResults` and the order of matching lines;
   - return records without mutating the source command output; and
   - leave storing the returned collection to the caller, or document clearly
     if the extractor itself performs that assignment.

   A missing source step is a runtime execution error, not an empty match. A
   source step that exists but has no output or no matching lines produces an
   empty collection according to the configured no-match policy. The extractor
   must never silently parse a different step because a step ID was misspelled.

   Done when a unit test can pass fabricated `StepResults` to the extractor
   without constructing a router or opening an SSH session.

2. Reuse the validated, compiled patterns.

   `Context.Pattern` and `RecordSpec.Pattern` are compiled during model
   validation. The extraction phase should use their compiled `RegExp` values
   and should not compile the same pattern for every line. If a compiled
   expression is unexpectedly missing, return a descriptive error rather than
   panicking. Do not interpret the command template as a regular expression;
   extraction always operates on the raw command result.

   For each compiled expression, obtain named capture positions with
   `SubexpNames()` and pair them with the values returned by
   `FindStringSubmatch`. Ignore unnamed capture groups unless the model later
   gives them explicit semantics. Preserve every captured value as a string,
   including IPv4 prefixes, IPv6 prefixes, labels, handles, and hexadecimal
   values.

   Done when a test proves that named captures are mapped by name rather than
   by a hard-coded numeric column position, including a pattern with an
   unnamed helper group.

3. Process each command result independently and line by line.

   Iterate through each `StepResult` in its existing order. Reset the local
   scan context at the beginning of each result because output from a
   different location or command iteration is a separate output document. A
   VRF header from one result must not be inherited by the first record from a
   later result that has no header.

   Split output using a line-oriented reader so large responses do not require
   an unnecessary collection of transformed lines. Keep the original line
   text, including leading whitespace, available to the regular expression.
   Blank lines and separator lines should simply produce no match unless a
   configured pattern explicitly matches them. They must not implicitly reset
   context in the initial implementation.

   Done when a fixture containing several locations and repeated command
   iterations produces records in the same order as the input results and
   lines.

4. Maintain context captures while scanning.

   For each input line, evaluate the configured context patterns and update a
   local `currentContext map[string]string` when one matches. A context
   pattern may capture one or more named values. If multiple context patterns
   match the same line, apply them in YAML order and reject duplicate field
   names during validation so the result is not ambiguous.

   Context values are the current scan state. They become useful to records
   through `RecordSpec.Inherit`; they should not silently overwrite unrelated
   values already in the router's `RunContext.Variables`. If a future feature
   needs a context capture to remain available as a top-level scalar after
   extraction, it should define an explicit promotion rule rather than making
   that side effect implicit in this phase.

   Example state transition:

   ~~~text
   line: VRF: NMNET       currentContext = {vrf: NMNET}
   line: route-A          record inherits vrf=NMNET
   line: route-B          record inherits vrf=NMNET
   line: VRF: GI          currentContext = {vrf: GI}
   line: route-C          record inherits vrf=GI
   ~~~

   A context header replaces the value for the same field. Fields not present
   in the new header remain available only if the format and configuration
   intentionally use them; the safest initial behavior is to replace the
   current context with the captures from the new header when the header
   represents a new logical block. Document and test whichever behavior is
   selected—do not let map merge behavior decide accidentally.

   Done when multiple VRFs in one response attach every route to the correct
   most-recent context and a blank line does not accidentally attach a route
   to a different VRF.

5. Build a record from captures and inherited values.

   When the record pattern matches, create a fresh `types.Record`. First copy
   the fields named by `RecordSpec.Inherit` from `currentContext`, then add
   the named captures from the record pattern. Do not reuse the same map for
   multiple records; each record must be independently mutable by a child
   `for_each` scope.

   Missing inherited fields must be handled explicitly. The recommended
   behavior is to return an extraction error identifying the collection,
   field, source step, and result location, because silently inserting an
   empty VRF can generate a valid-looking but incorrect command. Record
   capture names must not silently overwrite inherited names; Phase 3 model
   validation should reject that collision, and the extractor should retain a
   defensive runtime check as well.

   Done when a route record contains both its own captures and the expected
   inherited fields, and tests prove that two records do not share map state.

6. Apply semantic record filtering.

   The regular expression supplies structural filtering by deciding which
   lines have the expected shape. After inheritance has been applied, evaluate
   every `RecordSpec.Where` predicate against the complete record. The initial
   list form uses AND semantics: one false predicate rejects the record.

   Reuse the existing `Predicate` model and its validated operators:
   `equals`, `not_equals`, `contains`, `matches`, `exists`, and `in`. If
   `all`/`any` is already supported by the predicate evaluator, exercise it in
   this phase too; otherwise keep the evaluator isolated so compound logic can
   be added without changing extraction order. A predicate referring to a
   missing field must have one defined behavior, preferably `exists: false`
   matching while value comparisons fail, rather than treating missing and
   empty as identical.

   Apply filtering after captures and inheritance but before deduplication and
   `max_records`. This allows filters to use `vrf` and other inherited fields.

   Done when tests cover an accepted record, a rejected record, an inherited
   field, a missing field, and each supported predicate operator.

7. Deduplicate accepted records deterministically.

   If `RecordSpec.DeduplicateBy` is non-empty, build a stable key from the
   listed field values in YAML order. Preserve the first occurrence and keep
   the output order; do not use map iteration order. Include field boundaries
   in the key construction so values such as `a|bc` and `a|b|c` cannot collide
   merely because a separator was chosen carelessly.

   Missing deduplication fields must not silently collapse unrelated records.
   Prefer a descriptive runtime error naming the collection and missing field;
   if the implementation instead defines missing as an empty value, document
   that choice and test it. Validation should eventually confirm that every
   deduplication field is either a named record capture or an inherited field.

   Done when duplicate records are removed, the first record wins, and a test
   verifies stable ordering across repeated extraction runs.

8. Enforce the layered record limits.

   `PipelineLimits.MaxRecords` is the global hard ceiling. `RecordSpec.MaxRecords`
   is optional and is a tighter local limit when non-zero. Preserve zero as
   "unspecified" during model validation; at runtime, use the global limit for
   zero and clamp a larger local value down to the global ceiling. Join uses
   the global ceiling because it has no separate `max_records` field.

   Count records only after filtering and deduplication. Retain the first
   effective-limit accepted records in input order. Once the limit is reached,
   the extractor may stop scanning because no later record can be emitted;
   this is safe only if context side effects are local to this extraction and
   no later output is needed for another purpose. Otherwise continue scanning
   but do not append more records. Make the choice explicit in code and tests.

   Done when a fixture containing rejected rows and duplicates proves that the
   limit applies to final accepted records rather than raw regex matches.

9. Store the collection with the correct runtime scope.

   After extraction succeeds, assign the resulting slice to
   `ctx.Collections[extraction.RecordSpec.Name]`. Use the current runtime
   context, not a package-level map and not another router's context. A child
   record context may clone the collection map for isolation, as defined by
   `ChildForRecord`; the implementation must not accidentally write records
   into the parent when the intended scope is child-local.

   Re-extracting into the same collection name should have one explicit
   policy. The recommended initial policy is replacement within the current
   scope, because the collection represents the latest extraction result and
   this avoids accidental duplicate accumulation. If append semantics are
   required later, add it as an explicit model option.

   Done when a test confirms that two router contexts cannot see each other's
   collections and that a repeated extraction has the documented replacement
   behavior.

10. Define error and no-match behavior at the integration boundary.

   The extractor should return errors for malformed runtime state, missing
   source step results, missing required inherited fields, capture collisions,
   and predicate/evaluation failures. It should not treat a valid command with
   no matching records as a parser error. The calling step executor will apply
   `on_error` and logging policy, including `continue_record` where applicable.

   Include the router name, extraction step ID, `from_step_id`, collection
   name, source location, and record identity where available. This is
   especially important when one extraction consumes output from several
   locations.

   Done when negative tests assert actionable errors and a no-match fixture
   produces an empty, successful collection under the default discovery
   behavior.

11. Add focused unit tests before Phase 5 integration.

   Keep extraction tests independent from command execution. At minimum add:

   - one named capture and multiple named captures;
   - an unnamed helper capture alongside named captures;
   - no matches and empty output;
   - IPv4 and IPv6 prefixes/next hops;
   - multiple VRFs with inherited context;
   - blank lines, separators, and malformed-looking non-record lines;
   - multiple locations and repeated command results;
   - every supported `where` predicate, including missing fields;
   - duplicate records with deterministic first-wins ordering;
   - `max_records` after filtering and deduplication;
   - missing source step ID;
   - missing inherited and deduplication fields;
   - capture/inheritance name collisions; and
   - isolation between parent, child, and separate router contexts.

   Use the reduced fixtures already documented for unresolved CEF routes and
   retry-db output. Assert the complete records, not only their count:

   ~~~yaml
   - vrf: NMNET
     prefix: 10.240.16.84/31
     next_hop: 10.169.18.75/32
   ~~~

   Also assert that output from the same source step at multiple locations is
   retained in the expected order and is not silently replaced by the last
   `StepResult`.

Phase 4 is complete when the extractor can produce a correctly scoped,
ordered, filtered, deduplicated, and bounded collection from fabricated
`StepResults`, with negative tests covering invalid runtime references. Phase 5
can then focus on invoking this component after `Run`, preserving the
per-router `RunContext`, and recursively executing `ForEach` over the
collection.

Deliverable:

- an extraction component with a clear input/output contract;
- the unresolved-CEF and retry-db fixtures producing the expected records;
- focused positive, regression, and negative tests; and
- no dependency on a live router or on template rendering.

### Phase 5: Pipeline executor

Estimated effort: 3–5 hours.

Tasks:

- Execute command steps.
- Pass command output to extractors.
- Create one `RunContext` per router pipeline invocation and preserve it for
  the entire top-level step sequence; do not recreate it for each top-level
  step.
- Store values in that per-router context.
- Execute recursively nested steps sequentially.
- Implement for-each.
- Support a child step producing records for a deeper nested for-each.
- Treat a successful `Run` that returns zero result entries as a non-error stop
  signal, with propagation rules defined in section 8.6.
- Enforce maximum depth and maximum generated-command limits.
- Reuse ProcessCommand.

Deliverable:

- a fake-router test showing discovery followed by dynamically rendered
  dependent commands at multiple nesting levels.
- a fake-router test showing that an empty parent result executes no dependent
  commands and completes successfully.

The no-result check belongs directly after the successful
`ProcessCommand` call in `pkg/pipeline/pipeline.go`, before any extraction or
dependent step can run. The current `len(results) == 0` check near that point
is therefore the correct detection location, but returning ordinary `nil`
there is insufficient: `executePipeline` would continue with later sibling
steps. The executor must return a dedicated successful-stop signal and allow
it to bubble through nested `for_each` execution to the router-level pipeline
entry point.

Command accounting currently counts the result entries retained by
`ProcessCommand`. A zero-length response is removed before the pipeline sees
it, so the successful-stop signal is raised without adding a result entry to
`CommandsRun`. The command has already completed by the time the pipeline
checks `max_commands`; exact accounting of executed commands, including
commands that returned no data, would require a separate execution-count
value from `ProcessCommand`.

The intended executor order is:

~~~text
ProcessCommand
  -> execution error: apply normal error policy
  -> zero result objects: successful stop
  -> enforce max_commands and count returned command results
  -> retain converted StepResults
  -> continue to the next pipeline operation
~~~

### Phase 5A: Global scalar export and correlated dependencies

Estimated effort: 5–8 hours, preferably split into two implementation
sessions.

This phase addresses workflows in which two independent discovery paths must
meet before a command can be rendered. The ACL/BVI troubleshooting procedure
is the reference case:

~~~text
ACL usage -> BVI -> IFH -> extlif evidence
physical interface -> hosting NPU
internal TCAM -> ACL DB ID
hosting NPU + ACL DB ID -> FIA diagshell
~~~

The phase should be implemented after the basic recursive executor works, but
the scope contract should be agreed before adding syntax. The work is:

1. Make the scalar scope explicit.

   Keep the existing per-router `RunContext.Variables` map as the pipeline
   global scalar pool. Document that it is not process-global, not shared
   between routers, and not automatically populated by every extraction.
   Define template precedence as:

   ~~~text
   current record -> pipeline Variables -> reserved values
   ~~~

   Reject or explicitly namespace name collisions; do not let map assignment
   order decide which value a command receives.

2. Add explicit scalar publication.

   Add an `export`/`publish` step or equivalent operation with:

   - source collection name;
   - source field;
   - destination variable name;
   - cardinality policy; and
   - overwrite policy.

   The initial implementation should support `require: exactly_one` and
   reject publication from a `for_each` unless the configuration explicitly
   proves that all iterations produce one consistent value. Add parser tests
   for missing collections, missing fields, zero records, multiple records,
   duplicate destination names, and invalid policies.

#### 5A.1 Proposed step model and recursive dispatch

`export` and `join` are pipeline operations, not command modifiers. They
should be represented as additional mutually exclusive operations on
`PipelineStep`:

~~~go
type PipelineExport struct {
    From      string `yaml:"from"`
    Field     string `yaml:"field"`
    As        string `yaml:"as"`
    Require   string `yaml:"require"`
    Overwrite string `yaml:"overwrite,omitempty"`
}

type PipelineJoin struct {
    Left      string   `yaml:"left"`
    Right     string   `yaml:"right"`
    On        []string `yaml:"on"`
    Output    string   `yaml:"output"`
    Unmatched string   `yaml:"unmatched,omitempty"`
}

type PipelineStep struct {
    ID      string           `yaml:"id"`
    Run     *Command         `yaml:"run,omitempty"`
    Extract *PipelineExtract `yaml:"extract,omitempty"`
    ForEach *PipelineForEach `yaml:"for_each,omitempty"`
    Branch  *PipelineBranch  `yaml:"branch,omitempty"`
    Export  *PipelineExport  `yaml:"export,omitempty"`
    Join    *PipelineJoin    `yaml:"join,omitempty"`
    OnError string           `yaml:"on_error,omitempty"`
}
~~~

The existing `PipelineStep.Validate` and `isOnlyOneFunction` logic should be
extended so that exactly one of `run`, `extract`, `for_each`, `branch`,
`export`, or `join` is present. This keeps the model uniform: every operation
is a step with an ID, a scope, an error policy, and a place in `StepPath`.

The recursive executor does not need a separate executor for each nesting
level. `executeStep` should classify the already-validated step and dispatch
to the corresponding operation:

~~~text
executeStep(ctx, step):
    enter depth and StepPath

    if step.run       -> executeRun
    if step.extract   -> executeExtract
    if step.for_each  -> executeForEach
    if step.branch    -> executeBranch
    if step.export    -> executeExport
    if step.join      -> executeJoin

    restore depth and StepPath
~~~

The operation handlers should not call one another through special-case
level-specific code. `executeExport` and `executeJoin` are ordinary handlers
called by the same recursive `executeStep` path as `run`, `extract`, and
`branch`. They do not open router sessions and do not add `StepResults`, but
they do participate in step-path diagnostics and `on_error` handling.

The dispatch layer must also protect the scope rule at runtime. Even though
validation rejects illegal placement, `executeExport` and `executeJoin`
should return a descriptive error if `ctx.InRecordScope` is true. This is a
defensive check for callers that construct a model programmatically without
calling the full commander validator.

#### 5A.2 Scope rule: pipeline-level only

The initial implementation should allow `export` and `join` only in the
pipeline step list that owns the relevant collections. They must not appear
inside a `for_each` child list. This restriction is intentional:

- an export inside `for_each` could overwrite one scalar repeatedly;
- a join inside `for_each` could accidentally multiply work for every record;
- a child operation could observe a partially populated collection from a
  different record; and
- the meaning of publishing a value from zero, one, or many iterations would
  be ambiguous.

Validation should reject the placement and identify the complete step path.
The first implementation should use the simple rule: no export or join under
`for_each`.

#### 5A.3 Export execution contract

Implement `executeExport(ctx, spec)` with the following sequence:

1. Verify that the step is outside `for_each` scope.
2. Look up `spec.From` in `ctx.Collections`.
3. Distinguish a missing collection from an existing empty collection.
4. Apply the selected cardinality policy. The first supported policy is
   `exactly_one`.
5. For `exactly_one`, evaluate only records that contain `spec.Field`; the
   total number of records in the source collection is not itself the
   cardinality check. Require exactly one occurrence of the requested field.
6. Verify that the selected record contains `spec.Field` and export its
   value.
7. Reject reserved destinations such as `RouterName` and `Location`.
8. Reject an existing destination unless `overwrite` explicitly permits it.
9. Store the string value in `ctx.Variables[spec.As]`.

The recommended initial YAML is:

~~~yaml
- id: publish_hosting_npu
  export:
    from: hosting_npus
    field: npu
    as: hosting_npu
    require: exactly_one
~~~

The exported value is visible to every later step in the same router
pipeline. `buildRenderData` already overlays `Variables` before `Current`, so
a record field has the recommended precedence over a pipeline scalar. Name
collisions should nevertheless be rejected during validation rather than
being left to precedence alone.

Do not automatically export every capture. Multiple records, multiple
locations, and repeated commands make implicit promotion unsafe and can cause
silent last-writer-wins behavior.

The `exactly_one` policy is field-scoped rather than collection-scoped. Records
that do not contain the requested field are ignored for this cardinality check,
so a collection such as the following may still export `npu=0`:

~~~text
[{npu: "0", interface: "Te0"}, {db_id: "42"}]
~~~

The following cases have different outcomes:

- `[{npu: "0"}]` succeeds;
- `[{npu: "0"}, {npu: "1"}]` fails because the requested field occurs twice;
- `[{npu: "0"}, {npu: "0"}]` also fails because there are two occurrences,
  even though the values are identical; and
- a collection with no `npu` field fails because no value can be exported.

This policy must not silently deduplicate repeated values. A future
`all_equal` policy may explicitly accept one or more occurrences when every
requested-field value is identical and export that common value. `all_equal`
is distinct from `exactly_one` and must not be inferred automatically.

#### 5A.4 Keyed join execution contract

Implement `executeJoin(ctx, spec)` as a stable keyed inner join:

1. Verify that the step is outside `for_each` scope.
2. Look up both source collections.
3. Verify that every `on` field exists in every source record.
4. Build an index for the right collection without relying on map iteration
   order.
5. Walk the left collection in source order.
6. For every matching right record, create a fresh output record.
7. Preserve the join fields once and merge non-key fields according to the
   collision policy.
8. Store the result in `ctx.Collections[spec.Output]`.

Both source collections are required runtime dependencies. The join must fail
with an actionable error when either collection is missing, nil, or empty.
Likewise, every record in both collections must contain every field named by
`on`; a record missing a declared join field is a malformed input and must
fail the join rather than being treated as an unmatched record. These checks
are runtime checks because collection contents are produced while the
pipeline executes; model validation can verify only the statically declared
collection names and fields.

This strict source requirement is separate from the unmatched-key policy. If
both source collections are present and non-empty, but no left composite key
matches a right composite key, the `unmatched` policy applies: the initial
`ignore` policy produces an empty output collection and records diagnostics;
an explicitly selected `error` policy fails the join. An empty source is not
an unmatched-key case and is always an execution error.

The first implementation should support one or more equality keys and an
inner join. It should not support implicit index pairing or a Cartesian
product. A one-to-many match may produce multiple output records, but this
must be counted against the normal record and command expansion limits.

The initial collision policy should be conservative:

- identical values for a shared join field are accepted once;
- non-key field collisions are rejected; and
- source collections remain unchanged.

The initial unmatched policy should be `ignore`, meaning that unmatched
records are omitted from the output collection and reported in diagnostics.
Later policies may include `error` or `retain_left`, but they must be
explicitly selected in YAML.

Example:

~~~yaml
- id: correlate_fia_targets
  join:
    left: hosting_npus
    right: ingress_acl_entries
    on:
      - npu
    output: diagnostic_targets
    unmatched: ignore
~~~

The output is then consumed by an ordinary `for_each`:

~~~yaml
- id: inspect_fia_targets
  for_each:
    in: diagnostic_targets
    steps:
      - id: inspect_fia_db
        run:
          command: show controllers fia diagshell {{.npu}} "diag field T db={{.db_id}}"
          location:
            - 0/0/CPU0
~~~

#### 5A.5 Validation and symbol collection

Extend the existing pipeline symbol collector with scalar and transformation
symbols:

- `export.from` must refer to a previously defined collection;
- `export.field` must be a known field of that collection;
- `export.as` must be unique and must not be reserved;
- `join.left` and `join.right` must refer to previously defined collections;
- every join key must be known in both collections;
- `join.output` must be a new collection name; and
- join output fields must be computable without an unapproved collision.

The validator cannot determine whether a runtime collection will be empty or
whether every runtime record will contain all join fields. The executor must
therefore enforce the strict source preconditions described in section 5A.4:
both collections must exist and be non-empty, and every record must contain
every declared join key. These runtime failures must identify the join step,
source collection, and missing condition.

The validator should add exported names to a `variableFields` symbol set so
later templates and branch predicates can reference them. It should add join
output fields to `collectionFields[join.output]` so a later `for_each` can use
the generated records.

References must be validated in configuration order. A collection or variable
created later in another branch is not visible to the current step. This is
particularly important for recursive validation because `RunContext` does not
exist during configuration validation.

#### 5A.6 Scope boundary for child-produced collections

The current `ChildForRecord` behavior clones `Collections` and
`StepResults`. Consequently, a collection created inside a `for_each` is
child-local and is not automatically available to a later pipeline-level
`join` or `export`.

This is the correct default for record isolation, but it creates an important
constraint for the ACL/BVI workflow. If a later pipeline-level join needs data
discovered inside a `for_each`, the workflow must eventually provide an
explicit collection-aggregation boundary. Possible future designs include:

- a `for_each` output declaration that appends selected child records to a
  named parent collection;
- a dedicated `collect`/`publish_records` operation outside the loop; or
- restructuring the workflow so related values are extracted into one
  combined record before entering `for_each`.

The first export/join implementation must not silently merge child
collections into the parent. Until an explicit aggregation operation exists,
validation should reject a join whose input collection is produced only in a
child record scope. This preserves isolation and gives the user an actionable
diagnostic instead of an empty or partial correlation.

#### 5A.7 Streamlined ACL/BVI composition

With export, join, and an explicit aggregation boundary, the ACL/BVI profile
can follow one consistent dynamic pattern:

~~~text
discover ACL usage
    -> extract affected BVI records
discover bundle/member relationships
    -> extract physical-interface records
discover hosting NPU
    -> extract {interface, npu} records
discover ingress ACL database
    -> extract {npu, db_id} records
aggregate records from the relevant discovery scopes
    -> hosting_npus + ingress_acl_entries
keyed join on npu
    -> diagnostic_targets {npu, db_id}
for_each diagnostic_targets
    -> run final FIA diagnostic command
~~~

For the fixed-box fixture, the final dynamic portion becomes:

~~~yaml
- id: correlate_fia_targets
  join:
    left: hosting_npus
    right: ingress_acl_entries
    on: [npu]
    output: diagnostic_targets

- id: inspect_fia_targets
  for_each:
    in: diagnostic_targets
    steps:
      - id: inspect_fia_db
        run:
          command: show controllers fia diagshell {{.npu}} "diag field T db={{.db_id}}"
          location: [0/0/CPU0]
~~~

No literal NPU or DB ID is required. The final command consumes one
correlated record, so it remains compatible with the existing recursive
rendering path and does not need special command code.

#### 5A.8 Tests, diagnostics, and failure behavior

Add unit tests for the operation handlers and validation before the live
workflow test. The tests should cover:

- export from exactly one record;
- export failure for a missing collection, empty collection, multiple records,
  missing field, reserved destination, and duplicate destination;
- rejection of export and join inside `for_each`;
- one-key and multi-key joins;
- stable left-side order;
- one-to-many matches;
- missing join fields;
- non-key field collisions;
- unmatched records under the configured policy;
- preservation of source collections; and
- isolation of variables between two router contexts.

The ACL/BVI fake-router test should verify that the pipeline:

- selects only `BVI3318` from ACL usage output;
- renders the BVI-specific IM command;
- extracts IFH `0x200080a4`;
- renders the filtered extlif command using that IFH;
- extracts the relevant ACL evidence;
- discovers the hosting NPU from VOQ output;
- extracts DB ID `32` for `INGRESS_ACL_L3_IPV4`;
- creates the correlated `{npu: 0, db_id: 32}` target; and
- renders the final diagshell command exactly once for this fixture.

Include a multi-NPU fixture to prove that non-hosting NPUs do not produce
final commands, and a multiple-DB fixture to prove that each matched
`(npu, db_id)` pair is rendered once.

Diagnostics should report source collection names, join keys, input and
output record counts, unmatched keys, and the final record used for
rendering. A missing match must not silently run a command with an empty NPU
or DB ID. The normal `on_error` policy should be applied after the operation
has produced its diagnostic context.

#### 5A.9 Recommended implementation sequence

Implement the feature in small, independently testable increments:

1. Add `PipelineExport` and `PipelineJoin` types, YAML tags, comments, and
   clone support in `CloneForRun`.
2. Extend exactly-one-operation validation and add operation-specific syntax
   validation.
3. Extend the validation context with exported variable symbols and join
   output collection symbols.
4. Add scope-aware validation that rejects export/join below `for_each` and
   rejects references to child-only collections from a parent scope.
5. Implement `executeExport` without router access.
6. Implement `executeJoin` as a pure collection transformation without router
   access.
7. Add step-path diagnostics and apply the existing error-policy wrapper to
   both operations.
8. Add exact-rendering tests using a fake router that records the rendered
   command rather than the original template.
9. Add the ACL/BVI fixture and confirm the final FIA command is generated from
   extracted values only.
10. Add an explicit collection-aggregation operation only if the ACL/BVI
    workflow requires data discovered inside a `for_each` to be consumed by a
    later pipeline-level join. Do not weaken `ChildForRecord` isolation as a
    shortcut.

The first implementation should therefore deliver scalar export and keyed
join as separate milestones. They share symbol validation and recursive step
dispatch, but neither needs to know how router commands are executed.

Deliverable:

- explicit per-router global scalar semantics;
- a tested scalar export operation;
- a tested keyed correlation operation producing fresh records;
- a fake-router ACL/BVI workflow with dynamic NPU and DB-ID rendering; and
- documented fixed-box behavior for profiles that intentionally use a literal
  NPU while correlation support is unavailable.

### Phase 6: Location fan-out

Estimated effort: 1–2 hours.

Tasks:

- Verify dynamic command plus location: [all].
- Verify explicit locations.
- Verify location customization.
- Ensure command order is deterministic.

Deliverable:

- each generated dependent command runs against every expected location.

### Phase 7: Error policies and reporting

Estimated effort: 2–4 hours.

Tasks:

- Implement step and record error policies.
- Preserve best-effort processing across routers.
- Aggregate router and record errors.
- Add final execution summary.
- Ensure one router's failure does not stop the remaining routers.

Deliverable:

- deterministic failure behavior with tests for all supported scopes.

### Phase 8: Logging

Estimated effort: 1–2 hours.

Tasks:

- Add pipeline and step IDs to logs.
- Add record identifiers.
- Log discovery and generated command boundaries.
- Avoid logging credentials.

Deliverable:

- a multi-record router log that can be followed without debug tracing.

### Phase 9: Documentation and example profile

Estimated effort: 2–3 hours.

Tasks:

- Update README.md.
- Extend testdata/commands_v2.md.
- Add a complete pipeline example.
- Document extraction, inheritance, templates, fan-out, limits, and errors.
- Document the legacy/static versus pipeline modes.

Deliverable:

- a user-facing example that can be adapted for future workflows.

### Phase 10: End-to-end verification

Estimated effort: 2–4 hours.

Tasks:

- Run all existing tests.
- Run pipeline unit tests.
- Run with one router and multiple records.
- Run with multiple routers having different records.
- Run a workflow with four-to-six nested dependency levels.
- Test no-match behavior.
- Test a successful `Run` with no result objects.
- Test zero-length results from commands without locations and with explicit
  locations.
- Test mixed location/iteration results where retained results allow
  processing to continue.
- Test that zero results stop dependent steps without producing a router
  failure, including propagation through `for_each`.
- Test one failed dependent command.
- Test IPv4 and IPv6.
- Run the race detector where practical.

Deliverable:

- implementation ready for a controlled live-router test.

### Phase 11: Conditional branching stretch goal

Estimated effort: 4–8 hours when the existing predicate evaluator is reused,
or 1–2 additional days if full symbol validation, diagnostics, and compound
predicate coverage are added at the same time.

This phase should begin only after the sequential, extraction, rendering, and
recursive `for_each` paths are stable. The current model already contains
`PipelineBranch` and `PipelineCase`; this phase should implement those types,
not introduce a second `BranchStep` representation.

The purpose of branching is to select exactly one ordered child step list based
on values already available in the current runtime scope:

~~~text
current RunContext
        |
        v
evaluate cases in YAML order
        |
        +--> first matching case --> execute its steps
        |
        +--> no match + default --> execute default steps
        |
        +--> no match + no default --> controlled no-op/skip
~~~

#### 11.1 Freeze the branch semantics first

Document these rules before writing executor code:

- `PipelineBranch.Cases` is ordered and the first matching case wins.
- Cases are mutually exclusive at runtime because later cases are not
  evaluated after a match.
- `Default` is optional and runs only when no case matches.
- A branch does not create a new record scope. It evaluates and executes in
  the current `RunContext`.
- A branch inside `for_each` evaluates once for each current record.
- Branch child steps execute sequentially through the same recursive executor
  used by `for_each`.
- Branches may contain `run`, `extract`, `for_each`, and nested `branch`
  steps.
- Branching does not create additional goroutines.
- The existing depth and command-expansion limits apply to branch child steps.

The recommended no-match behavior is:

- with a default: execute the default steps;
- without a default inside `for_each`: return a controlled record skip so the
  next record can be evaluated; and
- without a default at pipeline scope: complete the branch successfully with
  no child execution.

This distinction prevents an intentionally unmatched branch from being
treated as a router failure while still allowing a record-level workflow to
skip records that do not belong to any case.

#### 11.2 Define the predicate scope

Branch predicates must use the same field-resolution contract as command
rendering and record filtering. Build a read-only predicate scope from the
current `RunContext`:

1. start with visible scalar `Variables`;
2. overlay fields from `Current` when processing a record; and
3. expose only explicitly documented runtime metadata, if any.

The precedence rule must be explicit. The recommended rule is that current
record fields take precedence over inherited scalar variables, while reserved
metadata such as `RouterName` is not silently overwritten.

Do not evaluate branch predicates against the raw YAML command or against a
global variable map. A branch inside a `for_each` must see the current record,
and one router must never see another router's values.

The predicate evaluator should accept a string-field map or a small read-only
scope interface so it can be reused by:

- `RecordSpec.Where` filtering;
- branch cases; and
- future conditional features.

The evaluator must support the predicate operators already accepted by the
model: `equals`, `not_equals`, `contains`, `matches`, `exists`, `in`, `all`,
and `any`. Missing fields must retain the established semantics: value
comparisons fail, `exists: false` matches, and `exists: true` requires the
field to be present even when its value is an empty string.

#### 11.3 Validate branch configuration

Use the existing model validation path before execution.

For every branch:

- reject a nil branch or nil case;
- require a valid `when` predicate for every case;
- require at least one child step in every case;
- validate all default steps recursively;
- validate nested branches recursively;
- enforce unique step IDs across the complete pipeline tree;
- enforce the configured maximum nesting depth; and
- reject malformed or conflicting predicates before a router session starts.

Validate predicate field references against symbols visible at the branch
location. A branch outside `for_each` can use fields produced by preceding
extracts. A branch inside `for_each` can additionally use fields from the
iterated collection. `exists: false` should not allow arbitrary misspelled
field names; the field must still be a known visible symbol.

Compound predicates require recursive symbol validation:

~~~yaml
when:
  all:
    - field: vrf
      equals: NMNET
    - any:
        - field: state
          equals: unresolved
        - field: state
          equals: retrying
~~~

The validator should report the branch step ID, case index, predicate path,
and unknown field, for example:

~~~text
branch step "classify_route", case 1, predicate "all[1].any[0]":
field "state" is not available in this scope
~~~

#### 11.4 Implement case selection

Add a branch arm to `executeStep` after the step path and depth have been
entered:

~~~text
executeStep(context, branchStep):
    selected = none

    for caseIndex, case in branch.cases:
        scope = predicateScope(context)
        matched = evaluate(case.when, scope)
        log caseIndex and matched result
        if matched:
            selected = case.steps
            break

    if selected is none and branch.default exists:
        selected = branch.default

    if selected is none:
        apply no-match behavior
        return

    executePipeline(router, selected, context)
~~~

The branch must reuse the existing recursive `executePipeline` function. This
ensures that branch children automatically support command rendering,
extraction, nested `for_each`, deeper branches, `StepPath`, maximum depth, and
command counting.

Do not clone `RunContext` merely because a branch was selected. A branch is a
control-flow decision, not a new record scope. Child record isolation remains
the responsibility of `ChildForRecord`.

#### 11.5 Apply error policies consistently

There are three distinct failure categories:

1. **Configuration errors** — malformed predicates, unknown fields, invalid
   nested steps, or duplicate IDs. These fail before execution.
2. **Predicate evaluation errors** — for example a runtime-invalid matches
   expression or invalid scope. Apply the branch step's `on_error` policy and
   include branch/case/predicate details.
3. **Child-step errors** — failures while executing the selected child steps.
   Propagate them through the existing recursive error handling and preserve
   the selected branch in the diagnostic path.

If `continue_record` is used inside a `for_each`, a branch failure should skip
only the current record. A child step explicitly configured with
`stop_router` must not be silently converted into a record continuation by a
parent branch or `for_each`; the error-policy precedence must be documented
and tested.

#### 11.6 Add branch diagnostics and logging

Branch logs should include:

- router name;
- branch step ID;
- current `StepPath`;
- case index, or `default`;
- predicate result;
- current record when inside `for_each`; and
- selected child step IDs.

Do not log sensitive command output merely to explain a predicate. Log the
field name and comparison outcome at the configured debug level, with normal
logs containing only the selected branch and workflow identity.

Errors should identify the branch step and selected case, for example:

~~~text
router "R1", pipeline step "classify_route", case 0,
path "inspect_routes -> classify_route -> inspect_retry":
command failed: ...
~~~

#### 11.7 Test implementation in increasing scope

Start with unit tests for the evaluator and selector:

- equals selects a matching case;
- not-equals selects a non-matching case;
- contains, matches, exists, and in work as specified;
- all requires every child predicate;
- any requires one child predicate;
- missing fields follow the defined semantics;
- the first matching case wins when several cases match; and
- an invalid predicate returns a useful error.

Then add executor tests using a fake router:

- branch at top-level selects only the expected command;
- default steps run when no case matches;
- no default follows the controlled no-match behavior;
- branch inside `for_each` evaluates independently for every record;
- branch child steps can run commands and extract records;
- a nested branch can select another branch or `for_each`;
- location fan-out works inside a selected branch;
- branch errors follow `on_error` policy;
- branch state does not leak between records; and
- branch state does not leak between routers.

Add regression tests proving that adding branch support does not alter legacy
static command execution or non-branch pipeline behavior.

#### 11.8 Version and feature reporting

The feature matrix used by the AI-prompt generator must report after Phase 11
implementation:

~~~text
branch validation: supported
branch execution: supported
~~~

Once this phase is complete, change only the execution feature flag and
increment the pipeline model version if branch semantics or YAML structure
changed. The generated AI instructions must then describe first-match order,
default behavior, supported predicate operators, and no-match behavior.

Deliverable:

- branch execution using the existing `PipelineBranch` and `PipelineCase`
  types;
- deterministic first-match selection;
- optional default handling;
- branching at pipeline and `for_each` scope;
- recursive branch child execution;
- validated predicate field references;
- diagnostics and logs containing branch context; and
- unit, executor, and regression tests.

Example acceptance scenario:

~~~text
field_1 = blah  -> execute command 2 only
field_1 = halb  -> execute command 3 only
field_1 = other -> execute default steps, if configured
~~~

## 17. Testing strategy

### 17.1 Parser tests

Verify:

- valid pipeline YAML
- invalid regular expressions
- invalid templates
- duplicate IDs
- invalid policies
- both commands and pipeline
- neither commands nor pipeline

### 17.2 Extractor tests

Use static fixtures and verify:

- one VRF and one route
- one VRF and multiple routes
- multiple VRFs
- IPv4 routes
- IPv6 routes
- inherited context
- duplicate records
- malformed rows
- no matches

### 17.3 Renderer tests

Verify:

- scalar substitution
- record substitution
- multiple values in one command
- missing variables
- invalid values
- location values
- command immutability after rendering

### 17.4 Executor tests

Use a fake implementation of types.Router that records every command it
receives and returns fixture output for selected commands.

Verify:

- command ordering
- record ordering
- location ordering
- nested step behavior
- four-to-six-level recursive step behavior
- child extraction feeding a deeper nested step
- maximum-depth rejection
- maximum-command-expansion rejection
- record isolation
- router isolation
- error policy behavior

### 17.5 Regression tests

Existing static profiles must still:

- parse
- execute in the same order
- run location commands as before
- run tests and triggered commands as before

### 17.6 Branching tests

When the branching stretch goal is implemented, verify:

- equals selects the expected case
- not_equals selects the expected case
- first matching case wins
- default executes when no case matches
- no default produces a controlled skip
- branching inside for-each evaluates once per record
- branch steps can contain dependent commands
- branch steps can contain extraction
- branch steps can contain location fan-out
- a branch failure follows the configured error policy
- branch contexts remain isolated between routers
- invalid predicates fail during configuration validation

### 17.7 Global-scope and correlation tests

For the ACL/BVI dependency pattern, use a fake router and independent output
fixtures for each discovery command. Verify:

- `BVI3318` is selected while unrelated ACL users are filtered out;
- the BVI record renders the IM lookup command;
- the IFH record renders the filtered extlif lookup command;
- the VOQ fixture produces the hosting NPU collection;
- the internal-TCAM fixture produces DB-ID records with their NPU fields;
- a keyed correlation produces exactly the expected `{npu, db_id}` records;
- unmatched NPUs and DB IDs follow the configured policy;
- duplicate join keys are rejected or handled according to policy;
- source collections are unchanged after correlation;
- a correlated record renders both template fields in the final command;
- a scalar export succeeds for exactly one record;
- scalar export rejects zero and multiple records under `exactly_one`;
- scalar values are isolated between routers;
- a repeated export cannot silently overwrite a value; and
- the fixed-box NPU-0 workaround remains distinguishable from generic
  dynamically correlated execution.

## 18. Acceptance criteria

The first production-ready version should meet all of the following:

1. Existing static command files require no modification.
2. A single process can process the full router inventory.
3. The password is entered once through stdin.
4. Each router receives an independent runtime context.
5. The discovery command can produce multiple records.
6. A context value such as VRF can be inherited by later route records.
7. A dependent command can use values from the current record.
8. Dependent commands can run across all locations.
9. One router failure does not stop other routers when configured accordingly.
10. One record failure has configurable behavior.
11. Missing variables fail before command execution.
12. Record expansion is bounded.
13. Logs identify pipeline, step, router, and record context.
14. IPv4 and IPv6 values can be captured without special-case command code.
15. Unit and integration tests cover the attached-output workflow.
16. Documentation contains a complete working example.
17. A child step can extract data for a deeper nested child step.
18. Four-to-six-level workflows execute without requiring special-case code
    for each depth.
19. Maximum depth and command-expansion limits prevent runaway execution.
20. Pipeline-global scalar values are isolated per router and are published
    explicitly rather than being created by implicit last-writer-wins rules.
21. A correlated record can combine values discovered by independent command
    paths, such as `{npu, db_id}` for a final diagnostic command.
22. Keyed correlation preserves source order, reports unmatched records, and
    does not silently create a Cartesian product.
23. A final command can render fields from a correlated record while retaining
    the normal recursive `for_each` and error-policy behavior.
24. A successful `Run` returning zero `CmdResult` entries stops the current
    router's remaining pipeline path without being reported as a failure.
25. Zero results inside `for_each` stop the complete current router pipeline,
    including remaining records and later sibling steps.
26. Zero-length results from one location or iteration are omitted while a
    retained result from another location or iteration allows processing to
    continue.
27. `export` publishes a scalar only when its configured cardinality policy is
    satisfied, and ambiguous values are rejected rather than silently
    overwritten.
28. `join` creates fresh correlated records from explicitly named collections
    and equality keys while preserving source order.
29. `export` and `join` are rejected inside `for_each` until an explicit
    collection-aggregation boundary is implemented.
30. Missing collections, missing fields, unmatched join keys, and field
    collisions produce diagnostics containing the operation step and path.
31. The ACL/BVI fixture discovers NPU and DB ID values dynamically and renders
    the final FIA diagnostic command without hardcoded correlation values.

### 18.1 Stretch-goal acceptance criteria for branching

The branching extension is complete when:

1. A captured value can select one of multiple step groups.
2. Cases are evaluated in deterministic configuration order.
3. A default case is supported.
4. Branching works at pipeline scope and inside for-each.
5. At least equals and not_equals are implemented and tested.
6. Branch execution is visible in logs.
7. Branch errors include router, pipeline, step, record, and predicate
   context.
8. Branching does not change legacy command behavior.

## 19. Open decisions

These decisions should be made before implementation begins:

1. Should all extracted records be processed by default?
2. Is filtering by VRF or prefix required in the first release?
3. For non-empty command output that produces no records, should the
   configured no-match policy be successful skip or a router failure? Empty
   command output itself follows the successful-stop semantics in section 8.6.
4. Should record errors continue with the next record by default?
5. Should generated commands be logged in full?
6. What maximum record and command expansion limits are appropriate?
7. Should pipeline mode replace legacy commands mode in one file, or should
   both ever be allowed together?
8. Should conditional steps remain a stretch goal after the core pipeline, or
   is value-based branching required in the first release?
9. Should location order be explicitly sorted for deterministic logs?
10. Should the final process exit non-zero for partial router failure while
    still completing every possible router?
11. Should the scalar publication operation be named `export` or `publish`?
12. Which cardinality policies should scalar publication support beyond
    `exactly_one`?
13. Should keyed correlation be a standalone step, or should a multi-input
    `for_each` be added first?
14. Which unmatched-record policies should keyed correlation support?
15. Should ancestor record fields be explicitly inherited into a nested child,
    or must the pipeline always create a combined record first?
16. Should Cartesian products ever be supported, and what independent limit
    should protect them?

Recommended defaults:

- process all matching records
- skip a discovery step with no records
- stop the current router on configuration or rendering errors
- continue to the next record for a configured command execution failure
- continue processing other routers
- return non-zero when any router or record failed
- support sequential recursive steps from the beginning
- default to a maximum depth that supports four-to-six-level workflows
- reserve branch steps in the model, but implement them as a stretch goal
- keep scalar publication and cross-collection correlation explicit; do not
  promote every extracted field into global variables
- require a correlated record before a command consumes values from multiple
  independent collections

Recorded resolutions for the initial implementation:

- All matching records are processed by default.
- Semantic record filtering is required in the initial implementation. Regex
  matching remains responsible for structural extraction, while `where`
  predicates perform post-extraction filtering.
- `pipeline_limits.max_records` is the global per-router record ceiling;
  `record_spec.max_records` may provide a tighter per-extraction limit, and a
  zero or omitted local value inherits the global ceiling. Join uses the
  global ceiling. `max_commands` remains the global per-router command
  expansion limit.
- Location expansion order does not need to be deterministic for correctness;
  tests should compare expanded locations without depending on map iteration
  order.
- `export` and `join` are pipeline-level transformation steps in the initial
  implementation and are not allowed inside `for_each`.
- The initial correlation operation is a keyed inner join that preserves left
  collection order and never creates an implicit Cartesian product.
- Collections created inside a `for_each` remain child-local until an explicit
  collection-aggregation operation is designed and implemented.

The open-decision list above is intentionally retained as a historical
checklist and reminder for future workflow features.

## 20. Effort estimate

For the focused first version described here, including recursive execution:

- approximately 20–32 engineering hours
- roughly 3–5 calendar weeks when developed in short sessions

The global-scalar publication and keyed-correlation phase adds approximately:

- 5–8 engineering hours
- one additional implementation milestone, depending on whether correlation
  is implemented as a standalone step or as a multi-input iteration feature

For a more advanced version with conditional branches, nested loops, richer
filters, retries, persisted state, and workflow graphs:

- approximately 40–58 engineering hours or more

The branching stretch goal by itself is expected to add:

- 2–4 hours for ordered equals/default branching
- 1–3 additional days for structured predicates, nested branch validation,
  and full test coverage

The recommended first milestone is the smallest complete vertical slice:

- one discovery command
- inherited VRF context
- multiple route records
- dynamic command rendering
- for-each
- existing all-location expansion
- per-router isolation
- clear failure behavior

Once this slice works against the attached output, the architecture will be
validated for the broader class of dynamically generated router commands.
