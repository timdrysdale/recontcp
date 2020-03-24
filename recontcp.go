/*
   recontcp is a tcp client that automatically reconnects
   Copyright (C) 2020 Timothy Drysdale <timothy.d.drysdale@gmail.com>

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as
   published by the Free Software Foundation, either version 3 of the
   License, or (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package recontcp

import (
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/jpillora/backoff"
	log "github.com/sirupsen/logrus"
	"github.com/timdrysdale/chanstats"
)

type TcpMessage struct {
	Data []byte
	N    int
}

// connects (retrying/reconnecting if necessary) to tcp socket at url

type ReconTcp struct {
	ForwardIncoming bool
	In              chan TcpMessage
	Out             chan TcpMessage
	Retry           RetryConfig
	Stats           *chanstats.ChanStats
	Url             string
}

type RetryConfig struct {
	Factor  float64
	Jitter  bool
	Min     time.Duration
	Max     time.Duration
	Timeout time.Duration
}

func New() *ReconTcp {
	r := &ReconTcp{
		In:              make(chan TcpMessage),
		Out:             make(chan TcpMessage),
		ForwardIncoming: true,
		Retry: RetryConfig{Factor: 2,
			Min:     1 * time.Second,
			Max:     10 * time.Second,
			Timeout: 1 * time.Second,
			Jitter:  false},
		Stats: chanstats.New(),
	}
	return r
}

// run this in a separate goroutine so that the connection can be
// ended from where it was initialised, by close((* ReconWs).Stop)
func (r *ReconTcp) Reconnect(closed chan struct{}, url string) {

	fmt.Printf("Reconnect url: %s\n", url)

	boff := &backoff.Backoff{
		Min:    r.Retry.Min,
		Max:    r.Retry.Max,
		Factor: r.Retry.Factor,
		Jitter: r.Retry.Jitter,
	}

	rand.Seed(time.Now().UTC().UnixNano())

	// try dialling ....

	for {

		select {
		case <-closed:
			return
		default:

			fmt.Printf("about to dial\n")
			err := r.Dial(closed, url)

			fmt.Printf("dial error: %v\n", err)

			log.WithField("error", err).Debug("Dial finished")
			if err == nil {
				boff.Reset()
			} else {
				time.Sleep(boff.Duration())
			}
			//TODO immediate return if cancelled....
		}
	}
}

// Dial the websocket server once.
// If dial fails then return immediately
// If dial succeeds then handle message traffic until
// the context is cancelled
func (r *ReconTcp) Dial(closed chan struct{}, hostport string) error {

	var err error

	fmt.Printf("in Dial: %s\n", hostport)

	log.WithField("To", hostport).Debug("Connecting")

	var d net.Dialer

	//assume our context has been given a deadline if needed

	fmt.Println("about to DialContext")
	c, err := d.Dial("tcp", hostport)
	fmt.Println("after DialContext")

	if err != nil {
		log.WithField("error", err).Error("Dialing")
		return err
	}

	// assume we are conntected?
	r.Stats.ConnectedAt = time.Now()

	log.WithField("To", hostport).Info("Connected")

	// handle our reading tasks

	client := &Client{
		conn:    c,
		send:    make(chan message, 256), //TODO is this too big?!
		receive: make(chan message, 256),
	}

	go client.writePump(closed)
	go client.readPump(closed)
	<-closed
	return nil
}
