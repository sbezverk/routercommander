package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/types"
)

func check(op string, iteration int, field *types.Field, store map[int]map[int]interface{}) (bool, error) {
	switch op {
	case "compare_with_previous_neq":
		if iteration == 0 {
			return false, nil
		}
		glog.Infof("Previous value: %s current value: %s", store[iteration-1][field.FieldNumber], store[iteration][field.FieldNumber])
		if store[iteration][field.FieldNumber] != store[iteration-1][field.FieldNumber] {
			return true, nil
		}
	case "compare_with_previous_eq":
		if iteration == 0 {
			return false, nil
		}
		glog.Infof("Previous value: %s current value: %s", store[iteration-1][field.FieldNumber], store[iteration][field.FieldNumber])
		if store[iteration][field.FieldNumber] != store[iteration-1][field.FieldNumber] {
			return true, nil
		}
	case "compare_with_value_neq":
		glog.Infof("Expected value: %s current value: %s", field.Value, store[iteration][field.FieldNumber])
		if store[iteration][field.FieldNumber] != field.Value {
			return true, nil
		}
	case "compare_with_value_eq":
		glog.Infof("Expected value: %s current value: %s", field.Value, store[iteration][field.FieldNumber])
		if store[iteration][field.FieldNumber] == field.Value {
			return true, nil
		}
	case "contain_substring":
		glog.Infof("substring value: %s current value: %s", field.Value, store[iteration][field.FieldNumber])
		if !strings.Contains(store[iteration][field.FieldNumber].(string), field.Value) {
			return true, nil
		}
	case "not_contain_substring":
		glog.Infof("substring value: %s current value: %s", field.Value, store[iteration][field.FieldNumber])
		if strings.Contains(store[iteration][field.FieldNumber].(string), field.Value) {
			return true, nil
		}
	default:
		return false, fmt.Errorf("unknown operation: %s for field number: %d",
			field.Operation, field.FieldNumber)
	}

	return false, nil
}

func getValue(b []byte, index []int, field *types.Field, separator string) (string, error) {
	if separator == "" {
		separator = " "
	}
	endLine, err := regexp.Compile(`(?m)$`)
	if err != nil {
		return "", err
	}
	startLine, err := regexp.Compile(`(?m)^`)
	if err != nil {
		return "", err
	}
	// First, find the end of the line with matching pattern
	eIndex := endLine.FindIndex(b[index[0]:])
	if eIndex == nil {
		return "", fmt.Errorf("failed to find the end of line in data: %s", string(b[index[0]:]))
	}
	en := index[0] + eIndex[0]
	// Second, find the start of the string with matching pattern
	sIndex := startLine.FindAllIndex(b[:en-1], -1)
	if sIndex == nil {
		return "", fmt.Errorf("failed to find the start of line in data: %s", string(b[:index[0]]))
	}
	st := sIndex[len(sIndex)-1][0]
	s := string(b[st:en])
	// Splitting the resulting string using provided separator
	sepreg, err := regexp.Compile("[" + separator + "]+")
	if err != nil {
		return "", err
	}
	parts := sepreg.Split(s, -1)
	if len(parts) < field.FieldNumber-1 {
		return "", fmt.Errorf("failed to split string %s with separator %q to have field number %d", s, separator, field.FieldNumber)
	}

	return strings.Trim(parts[field.FieldNumber-1], " \n\t,"), nil
}
