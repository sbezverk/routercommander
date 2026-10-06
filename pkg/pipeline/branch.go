package pipeline

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/types"
)

type branchChildError struct {
	err error
}

func (e *branchChildError) Error() string {
	return fmt.Sprintf("branch child execution failed: %v", e.err)
}

func (e *branchChildError) Unwrap() error {
	return e.err
}

func executeBranch(
	r types.Router,
	stepID string,
	branch *types.PipelineBranch,
	ctx *types.RunContext) error {

	if ctx == nil {
		return fmt.Errorf("RunContext is nil")
	}
	if branch == nil {
		return fmt.Errorf("Branch is nil")
	}
	if len(branch.Cases) == 0 && len(branch.Default) == 0 {
		return fmt.Errorf("Both Cases and Default are nil")
	}

	var matchedSteps []*types.PipelineStep
	selected := ""
	for caseIndex, caseItem := range branch.Cases {
		if caseItem == nil {
			return fmt.Errorf("Case item is nil")
		}
		if caseItem.When == nil {
			return fmt.Errorf("Case item 'When' is nil")
		}
		if len(caseItem.Steps) == 0 {
			return fmt.Errorf("Case item 'Steps' is empty")
		}
		scope := predicateScope(ctx)
		matched, err := evaluatePredicate(scope, caseItem.When)
		if err != nil {
			return fmt.Errorf("branch %q case %d predicate evaluation failed: %w", stepID, caseIndex, err)
		}
		if glog.V(4) {
			glog.Infof("router %q: pipeline branch %q path %q evaluated case %d: matched=%t%s", ctx.RouterName, stepID, strings.Join(ctx.StepPath, " -> "), caseIndex, matched, branchRecordLogSuffix(ctx))
		}
		if matched {
			matchedSteps = caseItem.Steps
			selected = fmt.Sprintf("case %d", caseIndex)
			break
		}
	}
	if matchedSteps == nil {
		if len(branch.Default) > 0 {
			matchedSteps = branch.Default
			selected = "default"
		} else {
			glog.Infof("router %q: pipeline branch %q path %q matched no case and has no default%s", ctx.RouterName, stepID, strings.Join(ctx.StepPath, " -> "), branchRecordLogSuffix(ctx))
			if ctx.InRecordScope {
				return types.ErrPipelineSkipRecord
			}
			return nil
		}
	}

	childIDs := make([]string, 0, len(matchedSteps))
	for _, child := range matchedSteps {
		if child != nil {
			childIDs = append(childIDs, child.ID)
		}
	}
	glog.Infof("router %q: pipeline branch %q path %q selected %s; child steps=%v%s", ctx.RouterName, stepID, strings.Join(ctx.StepPath, " -> "), selected, childIDs, branchRecordLogSuffix(ctx))

	if err := executePipeline(r, matchedSteps, ctx); err != nil {
		return &branchChildError{err: err}
	}
	return nil
}

func predicateScope(ctx *types.RunContext) map[string]string {
	scope := map[string]string{}
	maps.Copy(scope, ctx.Variables)
	maps.Copy(scope, ctx.Current)
	return scope
}

func branchRecordLogSuffix(ctx *types.RunContext) string {
	if !ctx.InRecordScope {
		return ""
	}
	return fmt.Sprintf(", current record=%v", ctx.Current)
}

func unwrapBranchChildError(err error) (error, bool) {
	var childErr *branchChildError
	if !errors.As(err, &childErr) {
		return nil, false
	}
	return childErr.err, true
}
