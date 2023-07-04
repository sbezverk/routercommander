package log

import (
	"os"
	"path"
	"strings"
	"time"
)

type Logger interface {
	GetLog() []byte
	GetLogFileName() string
	Log([]byte) error
	Close()
}

type Queue interface {
	Push([]byte)
	Pop() []byte
	GetAll() []byte
}
type node struct {
	item []byte
	next *node
}

var _ Queue = &queue{}

type queue struct {
	first *node
	last  *node
	size  int
}

func (q *queue) Push(b []byte) {
	if q.first == nil {
		q.first = &node{
			item: make([]byte, len(b)),
			next: nil,
		}
		copy(q.first.item, b)
		q.last = q.first
	} else {
		l := &node{
			item: make([]byte, len(b)),
			next: nil,
		}
		copy(l.item, b)
		q.last.next = l
		q.last = l
	}
	q.size += len(b)
}

func (q *queue) Pop() []byte {
	b := make([]byte, len(q.first.item))
	f := q.first
	q.first = f.next
	q.size -= len(q.first.item)

	return b
}

func (q *queue) GetAll() []byte {
	b := make([]byte, q.size)
	cp := 0
	for n := q.first; n != nil; {
		copy(b[cp:], n.item)
		cp += len(n.item)
		n = n.next
	}

	return b
}

func newQueue() Queue {
	return &queue{
		first: nil,
		size:  0,
	}
}

var _ Logger = &logger{}

type logger struct {
	f     *os.File
	input chan *data
	out   chan chan []byte
	stop  chan struct{}
}

type data struct {
	b   []byte
	err chan error
}

func (l logger) GetLog() []byte {
	lc := make(chan []byte)
	l.out <- lc
	return <-lc
}

func (l logger) GetLogFileName() string {
	_, fn := path.Split(l.f.Name())
	return fn
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
	q := newQueue()
	for {
		select {
		case d := <-l.input:
			if _, err := l.f.Write(d.b); err != nil {
				d.err <- err
				continue
			}
			q.Push(d.b)
			d.err <- nil
		case <-l.stop:
			return
		case lc := <-l.out:
			lc <- q.GetAll()
		}
	}
}

func NewLogger(prefix string, logLoc string) (Logger, error) {
	ts := strings.Replace(time.Now().Format("2006-01-02_15:04:05"), " ", "_", -1)
	fileName := logLoc + prefix + "_" + ts + ".log"
	f, err := os.Create(fileName)
	if err != nil {

		return nil, err
	}

	l := &logger{
		f:     f,
		input: make(chan *data),
		stop:  make(chan struct{}),
		out:   make(chan chan []byte),
	}

	go l.worker()

	return l, nil
}
