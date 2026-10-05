package types

import "errors"

var (
	ErrPipelineSkipRecord = errors.New("pipeline skip record")
	ErrPipelineNoData     = errors.New("pipeline stopped: no data collected according to provided criteria")
)
