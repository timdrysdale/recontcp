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
func (r *ReconTcp) readPump(closed chan struct{}) {
	defer func() {
		// Ensure that we close the connection:
		defer r.Conn.Close() //TODO do we need this?
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
			fmt.Printf("readPump waiting to readAtLeast\n")
			n, err := io.ReadAtLeast(r.Conn, glob, 1)
			fmt.Printf("readPump got error %v\n", err)
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
				r.In <- TcpMessage{Data: frame}
			}

		}
	}
}

// writePump pumps messages from the hub to the tcp connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (r *ReconTcp) writePump(closed <-chan struct{}) {
	fmt.Printf("in writePump\n")
	for {
		select {
		case message, ok := <-r.Out:
			fmt.Printf("writePump got message to send: %v\n", message)
			if !ok {
				return
			}

			n, err := r.Conn.Write(message.Data) //size was n
			fmt.Printf("writepump wrote message %v of size %v\n", message.Data, n)
			if err != nil {
				fmt.Printf("writepump error: %v", err)
				return
			}

		case <-closed:
			return
		}
	}
}
