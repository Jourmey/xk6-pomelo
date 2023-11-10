package pomelo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Jourmey/xk6-pomelo/pomelosdk"
	"github.com/dop251/goja"
	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/syncx"
	"go.k6.io/k6/js/modules"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type Client struct {
	vu  modules.VU
	obj *goja.Object

	// events handler
	sync.RWMutex
	events map[string]pomelosdk.Callback

	connectArgs ConnectArgs

	chatConnectorConnected bool
	chatConnector          *pomelosdk.Connector
	chatReqId              *uint64
	chatAckReqId           *uint64
}

func (c *Client) Request(route string, data []byte, callback pomelosdk.Callback) error {
	if c.chatConnectorConnected == false {
		return errors.New("invalid connector")
	}

	return c.asyncRequest(c.chatConnector, c.chatReqId, route, data, callback)
}

// asyncRequest 异步简单发送消息
func (c *Client) asyncRequest(connector *pomelosdk.Connector, reqId *uint64, route string, sendData []byte, cb pomelosdk.Callback) error {

	//id := atomic.LoadUint64(reqId)

	err := connector.Request(route, sendData, func(data string) {

		//log.Println(fmt.Sprintf("[%s][%d] %d - %s -- callback success, response.data: %s ", c.connectArgs.uid, c.connectArgs.channelId, id, route, string(data)))

		if cb != nil {
			cb(data)
		}

	})

	// 增加发送序号
	atomic.AddUint64(reqId, 1)

	//log.Println(fmt.Sprintf("[%s][%d] %d - %s -- request success, request: %s ", c.connectArgs.uid, c.connectArgs.channelId, id, route, string(sendData)))

	return err
}

func (c *Client) Close() {

	if c.chatConnectorConnected {
		c.chatConnector.Close()
	}
}

// On add the callback for the event
func (c *Client) On(event string, callback pomelosdk.Callback) {
	c.Lock()
	defer c.Unlock()

	c.events[event] = callback
}

// 事件监听
func (c *Client) onEvent() {

	type callbackMessage struct {
		MsgId     int    `json:"msgId"`
		OnlineNum uint64 `json:"onlineNum"`
	}

	type eventAck struct {
		Ack   int `json:"ack"`
		MsgId int `json:"msgId"`
	}

	ack := func(data string) {
		var cme callbackMessage
		if err := json.Unmarshal([]byte(data), &cme); err != nil {
			log.Println(fmt.Sprintf("[%s][%d] OnServer,data: %s", c.connectArgs.uid, c.connectArgs.channelId, string(data)))
		}

		ack := eventAck{
			Ack:   1,
			MsgId: cme.MsgId,
		}

		requestBytes, err := json.Marshal(ack)
		if err != nil {
			return
		}

		err = c.asyncRequest(c.chatConnector, c.chatAckReqId, ROUTE_ACK, requestBytes, nil)
		if err != nil {
			log.Println(fmt.Sprintf("[%s][%d] asyncRequest failed: %s", c.connectArgs.uid, c.connectArgs.channelId, err))
		}
	}

	c.chatConnector.On(Event_OnServer, func(data string) {
		//log.Println(fmt.Sprintf("[%s][%d] onServer,data: %s", c.connectArgs.uid, c.connectArgs.channelId, string(data)))

		ack(data)

		cb, ok := c.eventHandler(Event_OnServer)
		if ok && cb != nil {
			go cb(data)
		}
	})

	c.chatConnector.On(Event_OnAdd, func(data string) {
		//log.Println(fmt.Sprintf("[%s][%d] onAdd,data: %s", c.connectArgs.uid, c.connectArgs.channelId, string(data)))

		ack(data)

		cb, ok := c.eventHandler(Event_OnAdd)
		if ok && cb != nil {
			go cb(data)
		}
	})

	c.chatConnector.On(Event_OnLeave, func(data string) {
		//log.Println(fmt.Sprintf("[%s][%d] onLeave,data: %s", c.connectArgs.uid, c.connectArgs.channelId, string(data)))

		ack(data)

		cb, ok := c.eventHandler(Event_OnLeave)
		if ok && cb != nil {
			go cb(data)
		}
	})

	c.chatConnector.On(Event_OnChat, func(data string) {
		//log.Println(fmt.Sprintf("[%s][%d] onChat,duration: null ,data: %s", c.connectArgs.uid, c.connectArgs.channelId, string(data)))

		ack(data)

		cb, ok := c.eventHandler(Event_OnChat)
		if ok && cb != nil {
			go cb(data)
		}
	})

}

func (c *Client) eventHandler(event string) (pomelosdk.Callback, bool) {
	c.RLock()
	defer c.RUnlock()

	cb, ok := c.events[event]
	return cb, ok
}

// runAndWaitConnect Connector 初始化握手信息和保持连接
func runAndWaitConnect(ctx context.Context, connector *pomelosdk.Connector, address string, timeout time.Duration) error {
	err := connector.InitReqHandshake("0.6.0", "golang-websocket", nil, map[string]interface{}{"uid": "dude"})
	if err != nil {
		return err
	}
	err = connector.InitHandshakeACK(13)
	if err != nil {
		return err
	}

	// 确保连接成功再返回
	var (
		cond              = syncx.NewCond()
		connectorRunError error
	)

	connector.Connected(func() {

		cond.Signal()
	})

	go func() {

		// 增加超时时间
		ctx2, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		err = connector.Run(ctx2, address, 10)
		if err != nil {
			connectorRunError = err
		}
	}()

	_, ok := cond.WaitWithTimeout(timeout + 5*time.Second)
	if !ok {
		return errors.New("run timeout")
	}

	if connectorRunError != nil {
		return connectorRunError
	}

	return nil
}

type entryHandlerEnterRequest struct {
	Uid          string `json:"uid"`
	Username     string `json:"username"`
	Rtype        int    `json:"rtype"`
	Rid          string `json:"rid"`
	Role         int    `json:"role"`
	Ulevel       int    `json:"ulevel"`
	Uname        string `json:"uname"`
	Classid      string `json:"classid"`
	Mtcv         string `json:"mainTeacherClientVer"`
	Pv           string `json:"protocolVersion"`
	UniqId       string `json:"uniqId"`
	InteractMode int    `json:"interactMode"`
	LiveType     int    `json:"liveType"`
	Route        string `json:"route"`
	ReqId        int    `json:"reqId"`
}

// syncRequest 同步发送消息
func syncRequest(ctx context.Context, connector *pomelosdk.Connector, timeout time.Duration, reqId *uint64, route string, request interface{}, response interface{}) error {

	cond := syncx.NewCond()

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	var (
		responseStr string
	)

	err = connector.Request(route, requestBytes, func(data string) {

		responseStr = string(data)

		cond.Signal()
	})

	// 增加发送序号
	atomic.AddUint64(reqId, 1)

	_, ok := cond.WaitWithTimeout(timeout)
	if !ok {
		return errors.New("请求消息超时")
	}

	if response != nil {
		err = jsonx.UnmarshalFromString(responseStr, response)
		if err != nil {
			return err
		}
	}

	return nil
}
