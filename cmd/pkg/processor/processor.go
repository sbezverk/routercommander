package processor

import (
	"regexp"
	"sync"
)

const initialBufferSize = 1024 * 1024

type Feed interface {
	Bytes() []byte
	Write([]byte) (int, error)
}

var _ Feed = &cmdBuffer{}

type cmdBuffer struct {
	buffer      []byte
	newCmdFound bool
	cmd         []byte
	sync.Mutex
	currentPos int
	growFactor int
}

func NewFeed() Feed {
	return &cmdBuffer{
		currentPos:  0,
		growFactor:  1,
		newCmdFound: false,
		buffer:      make([]byte, initialBufferSize),
	}
}

// var exit = regexp.MustCompile(`.*[eE]{1}[xX]{1}[iI]{1}[tT]{1}`)
var prompt = regexp.MustCompile(`RP/[0-9]/RP[0-1]/.*#`)
var crlf = regexp.MustCompile(`(\r\n|\r|\n)`)

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
	p := prompt.FindIndex(b)
	if p != nil {
		//		glog.Infof("><SB> found prompt...")
		le := crlf.FindIndex(b[p[1]:])
		if le != nil {
			c.cmd = make([]byte, len(b[p[1]:p[1]+le[0]]))
			copy(c.cmd, b[p[1]:p[1]+le[0]])
			//			glog.Infof("><SB> found command: %s", string(c.cmd))
		}
	}
	copy(c.buffer[c.currentPos:], b)
	c.currentPos += l
	return l, nil

	// if c.newCmdFound {
	// 	// glog.Infof("><SB> exit was found previously")
	// 	return c.buffer.Write(b)
	// }
	// if exit.Match(b) {
	// 	glog.Infof("><SB> found exit")
	// 	c.newCmdFound = true
	// 	//return len(b), nil
	// }
	// // Ignoring bytes since they are part of ssh commands echo sequence
	// //	glog.Infof("><SB> match not found in %s", string(b))
	// return len(b), nil
}
func (c *cmdBuffer) Bytes() []byte {
	c.Lock()
	defer c.Unlock()
	return c.buffer
}
