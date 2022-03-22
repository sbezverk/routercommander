package processor

import (
	"regexp"
	"sync"
)

const initialBufferSize = 1024 * 1024

type Feed interface {
	Bytes() []byte
	Write([]byte) (int, error)
	FinalizedBytes() []byte
}

var _ Feed = &cmdBuffer{}

type cmdBuffer struct {
	buffer      []byte
	newCmdFound bool
	cmd         []byte
	exitFound   chan struct{}
	sync.Mutex
	currentPos int
	growFactor int
}

func NewFeed() Feed {
	return &cmdBuffer{
		currentPos:  0,
		growFactor:  1,
		newCmdFound: false,
		exitFound:   make(chan struct{}),
		buffer:      make([]byte, initialBufferSize),
	}
}

var exit = regexp.MustCompile(`RP\/[0-9]\/RP[0-1]\/CPU0:([a-zA-Z_\-0-9]+){1}#\s*[eE]{1}[xX]{1}[iI]{1}[tT]{1}`)

func (c *cmdBuffer) Write(b []byte) (n int, err error) {
	c.Lock()
	defer c.Unlock()
	l := len(b)
	if l+c.currentPos >= initialBufferSize*c.growFactor {
		c.growFactor++
		t := make([]byte, initialBufferSize*c.growFactor)
		copy(t, c.buffer)
		c.buffer = t
	}
	copy(c.buffer[c.currentPos:], b)
	c.currentPos += l
	if exit.Match(b) {
		// Sending the signal that collection has been completed, since exit command
		// appeared in stdout.
		go func() {
			c.exitFound <- struct{}{}
		}()
	}

	return l, nil
}

func (c *cmdBuffer) Bytes() []byte {
	c.Lock()
	defer c.Unlock()
	return c.buffer[:c.currentPos]
}

//  FinalizedBytes function blocks until the exit command is detected in the stdout, once exit
// is detected, FinalizedBytes will be unblocked and return the content of the buffer.
func (c *cmdBuffer) FinalizedBytes() []byte {
	<-c.exitFound
	return c.buffer[:c.currentPos]
}
