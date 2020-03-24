package recontcp

import (
	"fmt"
	"io"
	"time"

	log "github.com/sirupsen/logrus"
)

// ***************************************************************************

// readPump pumps messages from the request connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump(closed chan struct{}) {
	defer func() {
		// Ensure that we close the connection:
		defer c.conn.Close()
	}()

	maxFrameBytes := 1024

	var frameBuffer mutexBuffer

	rawFrame := make([]byte, maxFrameBytes)

	glob := make([]byte, maxFrameBytes)

	frameBuffer.b.Reset() //else we send whole buffer on first flush

	tCh := make(chan int)

	// Read from the buffer, blocking if empty
	go func() {

		for {

			select {
			case <-closed:
				fmt.Printf("readPump closed")
			default:
			}

			tCh <- 0 //tell the monitoring routine we're alive

			n, err := io.ReadAtLeast(c.conn, glob, 1)

			if err == nil {

				frameBuffer.mux.Lock()

				_, err = frameBuffer.b.Write(glob[:n])

				frameBuffer.mux.Unlock()

				if err != nil {
					log.Errorf("%v", err) //was Fatal?
					return
				}

			} else {

				return // avoid spinning our wheels

			}
		}
	}()

	for {

		select {

		case <-tCh:

			// do nothing, just received data from buffer

		case <-time.After(1 * time.Millisecond):
			// no new data for >= 1mS weakly implies frame has been fully sent to us
			// this is two orders of magnitude more delay than when reading from
			// non-empty buffer so _should_ be ok, but recheck if errors crop up on
			// lower powered system. Assume am on same computer as capture routine

			//flush buffer to internal send channel
			frameBuffer.mux.Lock()

			n, err := frameBuffer.b.Read(rawFrame)

			frame := rawFrame[:n]

			frameBuffer.b.Reset()

			frameBuffer.mux.Unlock()

			if err == nil && n > 0 {
				c.receive <- message{data: frame}
			}

		}
	}
}

// writePump pumps messages from the hub to the tcp connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump(closed <-chan struct{}) {

	for {
		select {
		case message, ok := <-c.send:

			if !ok {
				return
			}

			_, err := c.conn.Write(message.data) //size was n
			c.bufrw.Flush()
			if err != nil {
				return
			}

		case <-closed:
			return
		}
	}
}
