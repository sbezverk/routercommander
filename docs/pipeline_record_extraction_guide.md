# Pipeline Record Extraction Guide

Status: Proposed

## 1. Initial scope

The first implementation uses one regular expression for each record
collection. The expression may match one line or several lines, but it is
stored as a single `pattern_string`.

This is intentionally narrower than a full block parser. It should cover a
useful portion of router output while the pipeline runtime is being built. A
more human-friendly block and field syntax can be added later after real
examples show where the regular-expression approach is insufficient.

The user-facing configuration is shaped like this:

~~~yaml
extract:
  from_step_id: discover_output
  records:
    name: route
    pattern:
      pattern_string: '...one Go/RE2-compatible expression...'
~~~

The runtime uses named capture groups to create fields in each `Record`.

## 2. Mental model

Think of the expression as a description of one record, not the entire
command output. For every matching occurrence, the runtime creates one record.

~~~text
command output
    record 1  ->  Record{field_a: ..., field_b: ...}
    record 2  ->  Record{field_a: ..., field_b: ...}
    record 3  ->  Record{field_a: ..., field_b: ...}
~~~

The runtime should use the equivalent of `FindAllStringSubmatch`, so the
expression must match exactly one logical record at a time.

Do not start a repeated-record expression with a greedy `.*`. That can consume
several records and produce only one match.

## 3. Five-step construction process

### Step 1: Save representative output

Save the command output exactly as returned by the router. Keep the command
echo, prompts, timestamps, headers, separators, and blank lines in the sample.

Use at least two records when possible. A pattern that works for one record
may accidentally depend on one particular value or spacing arrangement.

### Step 2: Identify one record boundary

Find text that reliably identifies the record itself. Prefer, in order:

1. a stable field label inside the record;
2. a stable line structure, such as several typed columns;
3. a command-specific value such as the requested prefix;
4. a fixed literal value only when it is genuinely constant.

Do not use a changing value such as a local label (`24011`) as the record
boundary. Do not use the command echo as the boundary when it contains the
same value as the record.

For this MPLS row:

~~~text
24011  24009  42.42.42.42/32  FH0/0/0/0  12.12.12.2  0
~~~

the local and imposed labels are dynamic, so the row should be recognized by
its structure rather than by `24011`.

### Step 3: Replace values with named capture groups

A named capture has this Go/RE2 form:

~~~text
(?P<field_name>value_pattern)
~~~

The capture name becomes the record field name. For example:

~~~text
(?P<local_label>[0-9]+)
~~~

creates `local_label = 24011`.

Use the narrowest practical pattern:

| Field kind | Recommended pattern |
| --- | --- |
| Decimal integer | `[0-9]+` |
| Any non-space value | `\S+` |
| Hexadecimal value | `0x[0-9A-Fa-f]+` |
| IPv4 or IPv6 address | `[0-9A-Fa-f:.]+` |
| IPv4 or IPv6 prefix | `[0-9A-Fa-f:.]+/[0-9]+` |
| Remaining text on a line | `[^\r\n]*` |

If a field can contain either a number or a label such as `Exp-Null-v4`, use
`\S+` rather than `[0-9]+`.

### Step 4: Match separators deliberately

Router output usually contains variable spaces. Use:

- `[ \t]+` for one or more spaces or tabs;
- `[ \t]*` when spacing is optional;
- `\r?\n` for a line break that may contain a carriage return.

Avoid literal runs of spaces. Escape punctuation when it has a special
regular-expression meaning, for example `\(`, `\)`, `\[`, `\]`, `\+`, `\?`,
and `\.`.

Inside a YAML single-quoted string, backslashes can be written directly:

~~~yaml
pattern_string: '^Prefix:\s+(?P<prefix>[0-9A-Fa-f:.]+/\d+)$'
~~~

### Step 5: Compile and inspect the matches

Before running the pipeline against many routers, verify:

1. the expression compiles using Go's `regexp` package;
2. every expected record is matched;
3. each named field contains the expected value;
4. the command echo and prompt are not matched;
5. a second record produces a second result;
6. optional or missing lines behave as expected.

The eventual implementation should provide a preview mode that prints records
without executing dependent commands.

## 4. Matching one-line table records

For a simple table where all required values are on one line, build the
expression around the row structure.

Input:

~~~text
24011  24009  42.42.42.42/32  FH0/0/0/0  12.12.12.2  0
~~~

Conceptual expression:

~~~text
^[ \t]*(?P<local_label>[0-9]+)[ \t]+(?P<imposed_label>\S+)[ \t]+(?P<prefix>[0-9A-Fa-f:.]+/[0-9]+)[ \t]+(?P<egress_interface>\S+)[ \t]+(?P<next_hop>[0-9A-Fa-f:.]+)[ \t]+[0-9]+[ \t]*$
~~~

The named fields become:

~~~text
local_label      = 24011
imposed_label    = 24009
prefix           = 42.42.42.42/32
egress_interface = FH0/0/0/0
next_hop         = 12.12.12.2
~~~

The bytes-switched column is matched but not captured because it is not needed
by this workflow.

## 5. Matching one record spread across several lines

Use the `m` and `s` regular-expression flags:

- `m` makes `^` and `$` work at line boundaries;
- `s` makes `.` match newline characters.

The combined flag is written as `(?ms)`.

Example input:

~~~text
PATHLIST  0x4000000  161  Slow  Not Yet Recorded
    Error code: 0x4ff30200, details...
    PATHLIST pl:0x309f9e19f0 paths:1 pl-type:Encap-shared
    1st prefix dependent: NMNET 0xe0000004 10.240.16.84/31 leaf:0x30a4ae9628
~~~

One-record expression:

~~~text
(?ms)^[ \t]*PATHLIST[ \t]+0x[0-9A-Fa-f]+[^\r\n]*\r?\n.*?^[ \t]*Error code:[ \t]*(?P<error_code>0x[0-9A-Fa-f]+),[^\r\n]*\r?\n.*?^[ \t]*PATHLIST[ \t]+pl:(?P<pl>0x[0-9A-Fa-f]+)[ \t]+paths:[0-9]+[ \t]+pl-type:(?P<pl_type>\S+)[^\r\n]*\r?\n.*?^[ \t]*1st prefix dependent:[ \t]+(?P<vrf>\S+)[ \t]+(?P<vrfid>0x[0-9A-Fa-f]+)[ \t]+(?P<prefix_dependent>[0-9A-Fa-f:.]+/[0-9]+)[ \t]+leaf:(?P<leaf>0x[0-9A-Fa-f]+)[ \t]*$
~~~

Important rules:

- begin at a line that uniquely identifies one record;
- use `.*?`, the non-greedy form, between known lines;
- anchor each important line with `^`;
- end at the last required line of the record;
- do not use a leading `.*` that can swallow multiple records.

If a required line is absent, the expression may fail to match that record.
A future parser can provide better missing-field diagnostics, but the first
version should make this behavior visible in preview output.

## 6. Complete pipeline example

The expression is placed in the extraction step after the command that
produces the output:

~~~yaml
pipeline:
  - id: discover_mpls_forwarding
    run:
      command: show mpls forwarding prefix ipv4 unicast 42.42.42.42/32 detail
      location:
        - 0/RP0/CPU0

  - id: extract_mpls_forwarding
    extract:
      from_step_id: discover_mpls_forwarding
      records:
        name: mpls_forwarding
        pattern:
          pattern_string: '^[ \t]*(?P<local_label>[0-9]+)[ \t]+(?P<imposed_label>\S+)[ \t]+(?P<prefix>[0-9A-Fa-f:.]+/[0-9]+)[ \t]+(?P<egress_interface>\S+)[ \t]+(?P<next_hop>[0-9A-Fa-f:.]+)[ \t]+[0-9]+[ \t]*$'

  - id: inspect_mpls_forwarding
    for_each:
      in: mpls_forwarding
      steps:
        - id: inspect_prefix
          run:
            command: show cef {{.prefix}} hardware egress detail
            location:
              - all
~~~

For a multiline expression, the YAML value remains one long line. This is
awkward to read, but it is the deliberate Phase 1 tradeoff. A helper tool can
generate this line later from marked sample output.

## 7. Context and inherited fields

Some output contains context on one line and records later:

~~~text
VRF: METROE-E

Prefix              Next Hop
42.99.250.22/32     10.169.23.89/32
~~~

Use a separate context expression and inherit the captured value into each
record:

~~~yaml
extract:
  from_step_id: discover_routes
  context:
    - pattern:
        pattern_string: '^VRF:\s+(?P<vrf>[A-Za-z0-9_.:-]+)\s*$'
  records:
    name: route
    pattern:
      pattern_string: '^[ \t]*(?P<prefix>[0-9A-Fa-f:.]+/[0-9]+)[ \t]+(?P<next_hop>[0-9A-Fa-f:.]+/[0-9]+)'
    inherit:
      - vrf
~~~

The context expression updates the current `vrf` while the output is scanned.
The record expression produces route records, and `inherit` copies the
current VRF into each route.

## 8. Common mistakes

### Matching a dynamic value literally

This is fragile:

~~~text
24011 24009 42.42.42.42/32
~~~

Use capture patterns for changing values:

~~~text
(?P<local_label>[0-9]+)[ \t]+(?P<imposed_label>\S+)
~~~

### Matching the command echo

If the command echo contains the requested prefix, matching only the prefix
can capture the wrong line. Add enough row structure to distinguish the actual
record.

### Capturing a complete block with greedy `.*`

This can consume several records:

~~~text
(?s).*PATHLIST.*prefix dependent: ...
~~~

Start at the record boundary and use `.*?` only between known lines.

### Assuming spaces are fixed

Use `[ \t]+` instead of literal runs of spaces.

### Using unsupported regex features

Go's regular expressions are RE2-based. Do not use lookahead, lookbehind,
backreferences, or PCRE-only features. Named groups use:

~~~text
(?P<name>...)
~~~

## 9. Marked-output helper

An offline helper can reduce the typing burden. A user could mark a sample:

~~~text
>>>|local_label|24011|<<<  >>>|imposed_label|24009|<<<  42.42.42.42/32
~~~

The helper can then:

1. escape the unmarked text;
2. replace marked values with named capture groups;
3. infer or request a value pattern;
4. compile the generated Go/RE2 expression;
5. preview the extracted fields against the original output;
6. emit the YAML `pattern_string`.

One sample cannot always reveal the correct general pattern. For example,
`24011` could be a decimal label, while `Exp-Null-v4` is a symbolic label.
The helper should allow the user to select the field pattern when inference is
ambiguous.

## 10. Deferred improvements

The following improvements remain future work:

- a structured multiline block parser;
- table-column extraction without regular expressions;
- field rules such as `line_contains`, `value_after`, and `token`;
- better missing-field diagnostics;
- automatic inference from multiple samples;
- an interactive regex-building helper.

The single-pattern implementation should first be completed, tested against
real router output, and used to identify which improvements provide the most
value.
