package pipeline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sbezverk/routercommander/pkg/types"
)

func joinTestContext(maxRecords int) *types.RunContext {
	return &types.RunContext{
		Collections: map[string][]types.Record{},
		MaxRecords:  maxRecords,
	}
}

func TestExecuteJoinPreservesLeftAndRightSourceOrder(t *testing.T) {
	ctx := joinTestContext(20)
	ctx.Collections["left"] = []types.Record{
		{"vrf": "A", "id": "1", "left_value": "l1"},
		{"vrf": "B", "id": "2", "left_value": "l2"},
		{"vrf": "A", "id": "1", "left_value": "l3"},
	}
	ctx.Collections["right"] = []types.Record{
		{"vrf": "A", "id": "1", "right_value": "r1"},
		{"vrf": "A", "id": "1", "right_value": "r2"},
		{"vrf": "B", "id": "2", "right_value": "r3"},
	}
	leftBefore := append([]types.Record(nil), ctx.Collections["left"]...)
	rightBefore := append([]types.Record(nil), ctx.Collections["right"]...)

	err := executeJoin(ctx, &types.PipelineJoin{
		Left:   "left",
		Right:  "right",
		On:     []string{"vrf", "id"},
		Output: "joined",
	})
	if err != nil {
		t.Fatalf("executeJoin() error = %v", err)
	}

	want := []types.Record{
		{"vrf": "A", "id": "1", "left_value": "l1", "right_value": "r1"},
		{"vrf": "A", "id": "1", "left_value": "l1", "right_value": "r2"},
		{"vrf": "B", "id": "2", "left_value": "l2", "right_value": "r3"},
		{"vrf": "A", "id": "1", "left_value": "l3", "right_value": "r1"},
		{"vrf": "A", "id": "1", "left_value": "l3", "right_value": "r2"},
	}
	if got := ctx.Collections["joined"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("joined records = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(ctx.Collections["left"], leftBefore) {
		t.Fatal("left source collection was modified")
	}
	if !reflect.DeepEqual(ctx.Collections["right"], rightBefore) {
		t.Fatal("right source collection was modified")
	}
}

func TestExecuteJoinRejectsInvalidSourceCollections(t *testing.T) {
	tests := []struct {
		name        string
		collections map[string][]types.Record
		want        string
	}{
		{
			name: "missing left collection",
			collections: map[string][]types.Record{
				"right": {{"key": "1"}},
			},
			want: "left collection",
		},
		{
			name: "nil left collection",
			collections: map[string][]types.Record{
				"left":  nil,
				"right": {{"key": "1"}},
			},
			want: "left collection",
		},
		{
			name: "empty right collection",
			collections: map[string][]types.Record{
				"left":  {{"key": "1"}},
				"right": {},
			},
			want: "right collection",
		},
		{
			name: "missing left join field",
			collections: map[string][]types.Record{
				"left":  {{"other": "1"}},
				"right": {{"key": "1"}},
			},
			want: "field \"key\"",
		},
		{
			name: "missing right join field",
			collections: map[string][]types.Record{
				"left":  {{"key": "1"}},
				"right": {{"other": "1"}},
			},
			want: "join field \"key\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := joinTestContext(10)
			ctx.Collections = tt.collections
			err := executeJoin(ctx, &types.PipelineJoin{
				Left:   "left",
				Right:  "right",
				On:     []string{"key"},
				Output: "joined",
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("executeJoin() error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func TestExecuteJoinUnmatchedPoliciesAndCollisions(t *testing.T) {
	t.Run("ignore unmatched", func(t *testing.T) {
		ctx := joinTestContext(10)
		ctx.Collections["left"] = []types.Record{{"key": "1"}, {"key": "2"}}
		ctx.Collections["right"] = []types.Record{{"key": "1", "value": "one"}}

		err := executeJoin(ctx, &types.PipelineJoin{
			Left: "left", Right: "right", On: []string{"key"}, Output: "joined", Unmatched: "ignore",
		})
		if err != nil {
			t.Fatalf("executeJoin() error = %v", err)
		}
		want := []types.Record{{"key": "1", "value": "one"}}
		if got := ctx.Collections["joined"]; !reflect.DeepEqual(got, want) {
			t.Fatalf("joined records = %#v, want %#v", got, want)
		}
	})

	t.Run("error unmatched", func(t *testing.T) {
		ctx := joinTestContext(10)
		ctx.Collections["left"] = []types.Record{{"key": "2"}}
		ctx.Collections["right"] = []types.Record{{"key": "1", "value": "one"}}

		err := executeJoin(ctx, &types.PipelineJoin{
			Left: "left", Right: "right", On: []string{"key"}, Output: "joined", Unmatched: "error",
		})
		if err == nil || !strings.Contains(err.Error(), "no matching records") {
			t.Fatalf("executeJoin() error = %v, want unmatched-record error", err)
		}
		if _, exists := ctx.Collections["joined"]; exists {
			t.Fatal("join output was stored after failure")
		}
	})

	t.Run("non-key collision", func(t *testing.T) {
		ctx := joinTestContext(10)
		ctx.Collections["left"] = []types.Record{{"key": "1", "value": "left"}}
		ctx.Collections["right"] = []types.Record{{"key": "1", "value": "right"}}

		err := executeJoin(ctx, &types.PipelineJoin{
			Left: "left", Right: "right", On: []string{"key"}, Output: "joined",
		})
		if err == nil || !strings.Contains(err.Error(), "non-key field collision") {
			t.Fatalf("executeJoin() error = %v, want collision error", err)
		}
	})
}

func TestExecuteJoinEnforcesInclusiveRecordLimit(t *testing.T) {
	ctx := joinTestContext(2)
	ctx.Collections["left"] = []types.Record{{"key": "1"}, {"key": "2"}}
	ctx.Collections["right"] = []types.Record{{"key": "1"}, {"key": "2"}}

	err := executeJoin(ctx, &types.PipelineJoin{
		Left: "left", Right: "right", On: []string{"key"}, Output: "joined",
	})
	if err != nil {
		t.Fatalf("exactly-limit-sized join failed: %v", err)
	}
	if got := len(ctx.Collections["joined"]); got != 2 {
		t.Fatalf("joined record count = %d, want 2", got)
	}

	ctx = joinTestContext(1)
	ctx.Collections["left"] = []types.Record{{"key": "1"}, {"key": "2"}}
	ctx.Collections["right"] = []types.Record{{"key": "1"}, {"key": "2"}}
	err = executeJoin(ctx, &types.PipelineJoin{
		Left: "left", Right: "right", On: []string{"key"}, Output: "joined",
	})
	if err == nil || !strings.Contains(err.Error(), "maximum allowed records") {
		t.Fatalf("executeJoin() error = %v, want record-limit error", err)
	}
}
