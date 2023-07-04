package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/sbezverk/routercommander/pkg/types"
)

func matchPatterns(results []*types.CmdResult, patterns []*types.Pattern) (map[string][]string, error) {
	matches := make(map[string][]string)
	for _, re := range results {
		reader := bufio.NewReader(bytes.NewReader(re.Result))
		done := false
		for !done {
			b, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					return nil, fmt.Errorf("failed to read command: %q result buffer with error: %v", re.Cmd, err)
				}
				done = true
			}
			for _, p := range patterns {
				m, ok := matches[p.PatternString]
				if !ok {
					m = make([]string, 0)
				}
				if i := p.RegExp.FindIndex(b); i != nil {
					m = append(m, strings.Trim(string(b), "\n\r\t"))
				}
				matches[p.PatternString] = m
			}
		}
	}

	return matches, nil
}
