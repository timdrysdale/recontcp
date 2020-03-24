package recontcp

import (
	"bytes"
	"sync"
)

// messages will be wrapped in this struct for muxing
type message struct {
	data []byte //text data are converted to/from bytes as needed
}

type mutexBuffer struct {
	mux sync.Mutex
	b   bytes.Buffer
}
