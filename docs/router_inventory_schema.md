# Router Inventory Schema

## Goal

`routercommander` already has two useful execution modes:

- connect directly to a single router with `--router-name`
- connect to multiple routers listed in `--routers-file`

For incident-time information collection, that is not enough because the event usually carries a logical router name such as `r1`, while the actual management endpoint might be:

- a different DNS name
- a shared jump host name
- a non-standard SSH port

This document proposes a YAML-based router inventory so `routercommander` can resolve a logical router name into real connection details.

## Proposed Inventory Format

Use a YAML map keyed by normalized router name.

Example:

```yaml
routers:
  r1:
    address: vxr-slurm-307
    port: 24965
    platform: iosxr
    username: root

  r2:
    address: vxr-slurm-307
    port: 24966
    platform: iosxr
    username: root

  n9kv_switch1:
    address: 172.23.164.9
    port: 22
    platform: nxos
    username: admin
```

## Schema

Top level:

```yaml
routers: <map[string]RouterTarget>
```

`RouterTarget` fields:

- `address`
  - required
  - DNS name or IP to connect to
- `port`
  - optional
  - SSH port
  - default is `22`
- `platform`
  - optional
  - intended for future platform-specific behavior
- `username`
  - optional
  - per-router override if different from CLI default

Potential future fields:

- `password-env`
  - environment variable containing router password
- `jump-host`
  - for future proxy/jump-host support
- `tags`
  - for selecting a subset of routers
- `aliases`
  - if event naming diverges from inventory naming

## Normalization Rules

Router name lookup should be case-insensitive.

Recommended normalization:

1. trim leading and trailing whitespace
2. convert to lowercase

Examples:

- `R1` -> `r1`
- ` r1 ` -> `r1`
- `N9KV_SWITCH1` -> `n9kv_switch1`

Inventory keys should be stored in lowercase in YAML for readability and consistency, but the loader should still normalize keys defensively.

## CLI Resolution Behavior

### 1. `--router-name` without inventory file

Treat `--router-name` as direct connection target.

Behavior:

- connect to `--router-name`
- use `--port` if provided, otherwise `22`
- use CLI username/password

This preserves today’s direct mode.

### 2. `--router-name` with inventory file

Treat `--router-name` as a logical router identity and look it up in inventory.

Behavior:

- normalize `--router-name`
- find matching entry under `routers`
- connect to the resolved `address`
- use inventory `port` if present, otherwise `22`
- use inventory `username` if present, otherwise CLI `--username`

If the router is not found:

- return a clear error
- do not silently fall back to direct connection

That avoids confusion during automated incident collection.

### 3. inventory file without `--router-name`

Use inventory as a batch target source.

Behavior:

- iterate all routers in inventory
- connect to each using resolved address and port

This replaces the current plain-text router list behavior with a richer source of truth.

## Recommended Flag Semantics

Keep existing flags but shift `--routers-file` to mean inventory YAML instead of plain-text list.

Recommended interpretation:

- `--routers-file`
  - YAML inventory file
- `--router-name`
  - direct target if no inventory file is provided
  - inventory lookup key if inventory file is provided

This keeps the CLI familiar while making it much more useful for automation.

## Resolution Priority

When inventory is present and a single router is selected:

1. normalize router name
2. resolve inventory entry
3. choose connection address from inventory
4. choose SSH port from inventory, else default to `22`
5. choose username from inventory if present, else CLI username
6. use CLI password unless future per-router secret handling is added

## Why This Fits The Incident Pipeline

This inventory design lets a future webhook bridge pass a simple incident payload such as:

```json
{
  "router-name": "R1",
  "incident-type": "route-loss",
  "command-profile": "post-route-loss"
}
```

Then `routercommander` can:

1. normalize `R1` to `r1`
2. resolve `r1 -> vxr-slurm-307:24965`
3. connect with the correct SSH settings
4. run the requested command profile

That keeps the event pipeline clean:

- events carry logical identity
- inventory provides access details
- `routercommander` performs the actual collection

## Implementation Notes

Suggested helper functions:

- `normalizeRouterName(name string) string`
- `loadRouterInventory(path string) (*RouterInventory, error)`
- `resolveRouterTarget(name string, inventory *RouterInventory, defaultPort int, defaultUser string) (*ResolvedTarget, error)`

Suggested data types:

```go
type RouterInventory struct {
    Routers map[string]*RouterTarget `yaml:"routers"`
}

type RouterTarget struct {
    Address  string `yaml:"address"`
    Port     int    `yaml:"port"`
    Platform string `yaml:"platform"`
    Username string `yaml:"username"`
}

type ResolvedTarget struct {
    Name     string
    Address  string
    Port     int
    Platform string
    Username string
}
```

## Recommendation

Implement this in two steps:

1. replace plain-text router list loading with YAML inventory loading
2. add single-router lookup logic using normalized names

That gives a practical path forward without over-designing the first version.
