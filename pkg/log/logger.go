package log

import (
	"os"
	"strings"
	"time"
)

type Logger interface {
	Log([]byte) error
	Close()
}

var _ Logger = &logger{}

type logger struct {
	f     *os.File
	input chan *data
	stop  chan struct{}
}

type data struct {
	b   []byte
	err chan error
}

func (l logger) Log(b []byte) error {
	d := &data{
		b:   b,
		err: make(chan error),
	}
	l.input <- d

	return <-d.err
}

func (l logger) Close() {
	l.f.Close()
}

func (l logger) worker() {
	for {
		select {
		case d := <-l.input:
			if _, err := l.f.Write(d.b); err != nil {
				d.err <- err
				continue
			}
			d.err <- nil
		case <-l.stop:
			return
		}
	}
}

func NewLogger() (Logger, error) {
	ts := strings.Replace(time.Now().Format("2006-01-02_15:04:05"), " ", "_", -1)
	fileName := "./logs/pathchecker" + "_" + ts + ".log"
	f, err := os.Create(fileName)
	if err != nil {

		return nil, err
	}

	l := &logger{
		f:     f,
		input: make(chan *data),
		stop:  make(chan struct{}),
	}

	go l.worker()

	return l, nil
}
