package pomelosdk

import (
	"github.com/Jourmey/xk6-pomelo/pomelosdk/codec"
)

// Callback represents the callback type which will be called
// when the correspond events is occurred.
type Callback func(data string)

// NewConnector create a new Connector
func NewConnector(uid string) *Connector {
	return &Connector{
		uid:    uid,
		die:    make(chan byte),
		codec:  codec.NewDecoder(),
		chSend: make(chan []byte, 64),
		//chSend:    make(chan []byte),
		mid:       1,
		events:    map[string]Callback{},
		responses: map[uint]Callback{},
	}
}
