package pipeline

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/sbezverk/routercommander/pkg/types"
)

func extractRecords(
	ctx *types.RunContext,
	extraction *types.PipelineExtract,
) ([]types.Record, error) {
	if ctx == nil {
		return nil, fmt.Errorf("runtime context is nil")
	}
	if extraction == nil || extraction.RecordSpec == nil {
		return nil, fmt.Errorf("extract step is nil")
	}
	if extraction.FromStepID == "" {
		return nil, fmt.Errorf("extract step missing FromStepID")
	}
	stepResults, ok := ctx.StepResults[extraction.FromStepID]
	if !ok {
		return nil, fmt.Errorf("step result for %q not found", extraction.FromStepID)
	}
	if extraction.RecordSpec.Pattern == nil || extraction.RecordSpec.Pattern.RegExp == nil {
		return nil, fmt.Errorf("invalid record spec or its pattern")
	}
	for i, context := range extraction.Context {
		if context == nil || context.Pattern == nil || context.Pattern.RegExp == nil {
			return nil, fmt.Errorf("invalid context or its pattern at index %d", i)
		}
	}

	maxRecords := extraction.RecordSpec.MaxRecords
	if maxRecords == 0 {
		maxRecords = types.DefaultMaxRecords
	}
	if maxRecords < 0 {
		return nil, fmt.Errorf("step id %s has a negative maximum number of records: %d", extraction.FromStepID, maxRecords)
	}

	records := make([]types.Record, 0)
	for _, stepResult := range stepResults {
		// Context belongs to one command result. A header from one location or
		// command iteration must not leak into the next result.
		currentContext := make(map[string]string)
		lr := bufio.NewReader(bytes.NewReader(stepResult.Output))
		lineNumber := 0

		for {
			line, readErr := lr.ReadString('\n')
			if len(line) > 0 {
				lineNumber++
				// Keep leading and other significant whitespace, but remove the
				// line terminator so patterns using `$` match as expected.
				line = strings.TrimSuffix(line, "\n")
				line = strings.TrimSuffix(line, "\r")

				for _, context := range extraction.Context {
					captured, matched := extractNamedRecord(line, context.Pattern.RegExp)
					if matched {
						maps.Copy(currentContext, captured)
					}
				}

				captured, matched := extractNamedRecord(line, extraction.RecordSpec.Pattern.RegExp)
				if matched {
					record := types.Record(maps.Clone(captured))
					for _, inherited := range extraction.RecordSpec.Inherit {
						value, found := currentContext[inherited]
						if !found {
							return nil, fmt.Errorf(
								"step id: %s, location %q, line %d: context value for %q not found",
								extraction.FromStepID, stepResult.Location, lineNumber, inherited,
							)
						}
						record[inherited] = value
					}

					matches, err := recordMatchesPredicates(record, extraction.RecordSpec.Where)
					if err != nil {
						return nil, fmt.Errorf(
							"step id: %s, location %q, line %d: record predicate evaluation failed: %w",
							extraction.FromStepID, stepResult.Location, lineNumber, err,
						)
					}
					if !matches {
						goto nextLine
					}

					for _, key := range extraction.RecordSpec.DeduplicateBy {
						if _, found := record[key]; !found {
							return nil, fmt.Errorf(
								"step id: %s, collection %q: deduplication field %q is missing",
								extraction.FromStepID, extraction.RecordSpec.Name, key,
							)
						}
					}
					if len(records) > 0 &&
						len(extraction.RecordSpec.DeduplicateBy) > 0 &&
						IsDuplicate(record, records[len(records)-1], extraction.RecordSpec.DeduplicateBy) {
						goto nextLine
					}

					records = append(records, record)
					if len(records) >= maxRecords {
						if ctx.Collections == nil {
							ctx.Collections = make(map[string][]types.Record)
						}
						ctx.Collections[extraction.RecordSpec.Name] = records
						return records, nil
					}
				}

			nextLine:
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return nil, fmt.Errorf(
					"step id: %s, location %q, line %d: failed reading output: %w",
					extraction.FromStepID, stepResult.Location, lineNumber, readErr,
				)
			}
		}
	}

	if ctx.Collections == nil {
		ctx.Collections = make(map[string][]types.Record)
	}
	ctx.Collections[extraction.RecordSpec.Name] = records
	return records, nil
}

func extractNamedRecord(line string, pattern *regexp.Regexp) (map[string]string, bool) {
	matches := pattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}

	record := make(map[string]string)
	for index, name := range pattern.SubexpNames() {
		if index == 0 || name == "" || index >= len(matches) {
			continue
		}
		record[name] = strings.TrimSpace(matches[index])
	}

	return record, true
}

func IsDuplicate(r1, r2 types.Record, dedupKeys []string) bool {
	for _, key := range dedupKeys {
		if r1[key] != r2[key] {
			return false
		}
	}
	return true
}

func recordMatchesPredicates(record types.Record, predicates []*types.Predicate) (bool, error) {
	for i, predicate := range predicates {
		matched, err := evaluatePredicate(record, predicate)
		if err != nil {
			return false, fmt.Errorf("predicate %d: %w", i, err)
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func evaluatePredicate(record types.Record, predicate *types.Predicate) (bool, error) {
	if predicate == nil {
		return false, fmt.Errorf("predicate is nil")
	}
	if predicate.All != nil {
		if len(predicate.All) == 0 {
			return false, fmt.Errorf("all predicate has no children")
		}
		for i, child := range predicate.All {
			matched, err := evaluatePredicate(record, child)
			if err != nil {
				return false, fmt.Errorf("all predicate child %d: %w", i, err)
			}
			if !matched {
				return false, nil
			}
		}
		return true, nil
	}
	if predicate.Any != nil {
		if len(predicate.Any) == 0 {
			return false, fmt.Errorf("any predicate has no children")
		}
		for i, child := range predicate.Any {
			matched, err := evaluatePredicate(record, child)
			if err != nil {
				return false, fmt.Errorf("any predicate child %d: %w", i, err)
			}
			if matched {
				return true, nil
			}
		}
		return false, nil
	}

	value, exists := record[predicate.Field]
	switch {
	case predicate.Exists != nil:
		if *predicate.Exists {
			return exists, nil
		}
		return !exists, nil
	case !exists:
		// Missing fields do not satisfy value comparisons. Use exists:false
		// when absence itself is the condition.
		return false, nil
	case predicate.Equals != nil:
		return value == *predicate.Equals, nil
	case predicate.NotEquals != nil:
		return value != *predicate.NotEquals, nil
	case predicate.Contains != nil:
		return strings.Contains(value, *predicate.Contains), nil
	case predicate.Matches != nil:
		matched, err := regexp.MatchString(*predicate.Matches, value)
		if err != nil {
			return false, fmt.Errorf("invalid matches expression: %w", err)
		}
		return matched, nil
	case predicate.In != nil:
		return slices.Contains(predicate.In, value), nil
	default:
		return false, fmt.Errorf("predicate has no supported operator")
	}
}
