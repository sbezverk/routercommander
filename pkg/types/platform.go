package types

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/sbezverk/routercommander/pkg/patterns"
)

type rp struct {
	isActive bool
	location string
}

func (r *rp) IsActive() bool {
	return r.isActive
}

type rps struct {
	rps map[string]*rp
}

func getRPs(b []byte) (*rps, error) {
	r := &rps{
		make(map[string]*rp),
	}
	rd := bufio.NewReader(bytes.NewReader(b))
	done := false
	for !done {
		l, err := rd.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			done = true
		}
		if patterns.RP.Find(l) != nil {
			parts := patterns.SubStringSeparator.Split(string(l), -1)
			if len(parts) < 3 {
				return nil, fmt.Errorf("invalid line %s in show platform", string(l))
			}
			isActive := patterns.ActiveRP.Find([]byte(parts[1])) != nil
			r.rps[strings.Trim(parts[0], " \t\n,")] = &rp{
				location: strings.Trim(parts[0], " \t\n,"),
				isActive: isActive,
			}
		}
	}
	if len(r.rps) == 0 {
		return nil, fmt.Errorf("no RP found")
	}

	return r, nil
}

type lc struct {
	location string
}

type lcs struct {
	lcs map[string]*lc
}

func getLCs(b []byte) (*lcs, error) {
	l := &lcs{
		lcs: make(map[string]*lc),
	}

	rd := bufio.NewReader(bytes.NewReader(b))
	done := false
	for !done {
		ln, err := rd.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			done = true
		}
		if patterns.LC.Find(ln) != nil {
			parts := patterns.SubStringSeparator.Split(string(ln), -1)
			if len(parts) < 3 {
				return nil, fmt.Errorf("invalid line %s in show platform", string(ln))
			}
			l.lcs[strings.Trim(parts[0], " \t\n,")] = &lc{
				location: strings.Trim(parts[0], " \t\n,"),
			}
		}
	}

	if len(l.lcs) == 0 {
		return nil, nil
	}

	return l, nil
}

type platform struct {
	rps *rps
	lcs *lcs
}

func populatePlatformInfo(b []byte) (*platform, error) {
	p := &platform{}
	var err error
	p.rps, err = getRPs(b)
	if err != nil {
		return nil, err
	}
	p.lcs, err = getLCs(b)
	if err != nil {
		return nil, err
	}

	return p, nil
}
