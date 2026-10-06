package types

import (
	"reflect"
	"sort"
	"testing"
)

func TestLegacyCommandProfileRegression(t *testing.T) {
	profile := []byte(`collect:
  process_result: true
commands:
  - command: show platform
    command_timeout: 30
    location:
      - 0/0/CPU0
    process_result: true
    patterns:
      - pattern_string: '^Node'
`)

	commander, err := parseCommandFile(profile)
	if err != nil {
		t.Fatalf("legacy command profile failed to parse: %v", err)
	}
	if len(commander.MainCommandGroup) != 1 {
		t.Fatalf("expected one legacy command, got %d", len(commander.MainCommandGroup))
	}

	command := commander.MainCommandGroup[0]
	if command.Cmd != "show platform" {
		t.Fatalf("unexpected legacy command: %q", command.Cmd)
	}
	if command.CmdTimeout != 30 {
		t.Fatalf("command timeout did not bind: %d", command.CmdTimeout)
	}
	if !reflect.DeepEqual(command.Location, []string{"0/0/CPU0"}) {
		t.Fatalf("command location did not bind: %v", command.Location)
	}
	if len(command.Patterns) != 1 || command.Patterns[0].RegExp == nil {
		t.Fatalf("legacy command pattern was not compiled: %+v", command.Patterns)
	}
	if command.CommandResult == nil {
		t.Fatal("legacy command result state was not initialized")
	}
}

func TestPipelineModelParses(t *testing.T) {
	commander, err := GetCommands("pipeline_model.yaml")
	if err != nil {
		t.Fatalf("pipeline model failed to parse: %v", err)
	}
	if len(commander.Pipeline) == 0 {
		t.Fatal("expected pipeline model to contain steps")
	}
}

func TestPredicateValidation(t *testing.T) {
	stringValue := func(value string) *string { return &value }
	boolValue := func(value bool) *bool { return &value }

	tests := []struct {
		name      string
		predicate *Predicate
		wantErr   bool
	}{
		{name: "nil predicate", predicate: nil, wantErr: true},
		{name: "equals", predicate: &Predicate{Field: "status", Equals: stringValue("ok")}},
		{name: "exists false", predicate: &Predicate{Field: "status", Exists: boolValue(false)}},
		{name: "in", predicate: &Predicate{Field: "status", In: []string{"ok", "ready"}}},
		{name: "matches", predicate: &Predicate{Field: "prefix", Matches: stringValue(`^10\.`)}},
		{
			name: "all",
			predicate: &Predicate{All: []*Predicate{
				{Field: "status", Equals: stringValue("ok")},
				{Field: "prefix", Matches: stringValue(`^10\.`)},
			}},
		},
		{
			name: "any",
			predicate: &Predicate{Any: []*Predicate{
				{Field: "status", Equals: stringValue("ok")},
				{Field: "status", Equals: stringValue("ready")},
			}},
		},
		{name: "missing field", predicate: &Predicate{Equals: stringValue("ok")}, wantErr: true},
		{name: "missing operator", predicate: &Predicate{Field: "status"}, wantErr: true},
		{name: "conflicting leaf operators", predicate: &Predicate{Field: "status", Equals: stringValue("ok"), NotEquals: stringValue("failed")}, wantErr: true},
		{name: "empty in", predicate: &Predicate{Field: "status", In: []string{}}, wantErr: true},
		{name: "invalid matches", predicate: &Predicate{Field: "prefix", Matches: stringValue("[")}, wantErr: true},
		{name: "all and any", predicate: &Predicate{All: []*Predicate{{Field: "status", Exists: boolValue(true)}}, Any: []*Predicate{{Field: "status", Exists: boolValue(false)}}}, wantErr: true},
		{name: "compound with field", predicate: &Predicate{Field: "status", All: []*Predicate{{Field: "status", Exists: boolValue(true)}}}, wantErr: true},
		{name: "empty all", predicate: &Predicate{All: []*Predicate{}}, wantErr: true},
		{name: "nil child", predicate: &Predicate{Any: []*Predicate{nil}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.predicate.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Predicate.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPrepareLocationsExpandsSelectors(t *testing.T) {
	r := &router{
		platform: &platform{
			rps: &rps{rps: map[string]*rp{
				"0/RP0/CPU0": {location: "0/RP0/CPU0"},
				"0/RP1/CPU0": {location: "0/RP1/CPU0"},
			}},
			lcs: &lcs{lcs: map[string]*lc{
				"0/0/CPU0": {location: "0/0/CPU0"},
				"0/1/CPU0": {location: "0/1/CPU0"},
			}},
		},
	}

	locations, err := prepareLocations(r, &Command{
		Location: []string{"all-rp", "all-lc", "0/9/CPU0"},
	})
	if err != nil {
		t.Fatalf("location expansion failed: %v", err)
	}

	sort.Strings(locations)
	want := []string{
		"0/0/CPU0",
		"0/1/CPU0",
		"0/9/CPU0",
		"0/RP0/CPU0",
		"0/RP1/CPU0",
	}
	if !reflect.DeepEqual(locations, want) {
		t.Fatalf("unexpected expanded locations: got %v, want %v", locations, want)
	}
}

func TestGetAllLocationsDoesNotDuplicateRPsWhenNoLCs(t *testing.T) {
	r := &router{
		platform: &platform{
			rps: &rps{rps: map[string]*rp{
				"0/RP0/CPU0": {location: "0/RP0/CPU0"},
			}},
			lcs: &lcs{lcs: map[string]*lc{}},
		},
	}

	got := r.GetAllLocations()
	want := []string{"0/RP0/CPU0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected locations: got %v, want %v", got, want)
	}
}

func TestCloneForRunCreatesIndependentRouterCommandState(t *testing.T) {
	commands := &Commander{
		MainCommandGroup: []*Command{
			{
				Cmd:      "show version",
				Location: []string{"0/0/CPU0"},
			},
		},
	}

	routerA := commands.CloneForRun()
	routerB := commands.CloneForRun()
	routerA.MainCommandGroup[0].Location[0] = "0/1/CPU0"
	routerA.MainCommandGroup[0].CommandResult.PatternMatch = append(
		routerA.MainCommandGroup[0].CommandResult.PatternMatch,
		"router-a",
	)

	if routerB.MainCommandGroup[0].Location[0] != "0/0/CPU0" {
		t.Fatalf("router command location state was shared")
	}
	if len(routerB.MainCommandGroup[0].CommandResult.PatternMatch) != 0 {
		t.Fatalf("router command result state was shared")
	}
	if commands.MainCommandGroup[0].Location[0] != "0/0/CPU0" {
		t.Fatalf("source command location state was mutated")
	}
}
