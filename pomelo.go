package pomelo

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/zeromicro/go-zero/core/jsonx"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"xk6-pomelo/pomelosdk"

	"github.com/dop251/goja"
	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/syncx"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/metrics"
)

// pomelo 路由
const (
	ROUTE_GATE                         = "gate.gateHandler.queryEntry"
	ROUTE_Connector_EntryHandler_Enter = "connector.entryHandler.enter"
	ROUTE_Chat_ChatHandler_Send        = "chat.chatHandler.send"

	ROUTE_ACK = "ack"
)

type (
	RootModule struct{}

	Instance struct {
		vu  modules.VU
		obj *goja.Object

		connectArgs ConnectArgs

		chatConnectorConnected bool
		chatConnector          *pomelosdk.Connector
		chatReqId              *uint64
		chatAckReqId           *uint64
	}
)

var (
	_ modules.Module   = &RootModule{}
	_ modules.Instance = &Instance{}
)

// New returns a pointer to a new RootModule instance.
func New() *RootModule {
	return &RootModule{}
}

// NewModuleInstance implements the modules.Module interface to return
// a new instance for each VU.
func (*RootModule) NewModuleInstance(m modules.VU) modules.Instance {
	rt := m.Runtime()
	mi := &Instance{
		vu: m,
	}
	obj := rt.NewObject()
	if err := obj.Set("connect", mi.Connect); err != nil {
		common.Throw(rt, err)
	}

	mi.obj = obj
	return mi
}

// Socket is the representation of the websocket returned to the js.
type Socket struct {
	rt            *goja.Runtime
	ctx           context.Context //nolint:containedctx
	conn          *websocket.Conn
	eventHandlers map[string][]goja.Callable
	scheduled     chan goja.Callable
	done          chan struct{}
	shutdownOnce  sync.Once

	pingSendTimestamps map[string]time.Time
	pingSendCounter    int

	tagsAndMeta    *metrics.TagsAndMeta
	samplesOutput  chan<- metrics.SampleContainer
	builtinMetrics *metrics.BuiltinMetrics
}

type ConnectArgs struct {
	Uid         int
	ChannelId   int
	RoomId      string
	ChatAddress string
}

const writeWait = 10 * time.Second

// Exports returns the exports of the ws module.
func (mi *Instance) Exports() modules.Exports {
	return modules.Exports{Default: mi.obj}
}

func (mi *Instance) Connect(args ConnectArgs) error {
	ctx := mi.vu.Context()
	//rt := mi.vu.Runtime()
	state := mi.vu.State()
	if state == nil {
		return errors.New("invalid state")
	}

	//parsedArgs, err := parseConnectArgs(state, rt, args...)
	//if err != nil {
	//	return nil, err
	//}

	//parsedArgs.tagsAndMeta.SetSystemTagOrMetaIfEnabled(state.Options.SystemTags, metrics.TagURL, url)

	connector := pomelosdk.NewConnector()

	err := runAndWaitConnect(ctx, connector, "", 10*time.Second)
	if err != nil {
		return err
	}

	uniqId := rand.Int()

	request := EntryHandlerEnterRequest{
		Uid:      mi.connectArgs.Uid,
		Username: mi.connectArgs.Uid,
		Uname:    mi.connectArgs.Uid,

		Rtype:        mi.connectArgs.ChannelId,
		Rid:          mi.connectArgs.RoomId,
		Role:         1, //1:学生，2:辅导，4:授课，3:旁听用户，5:游客
		Ulevel:       1,
		Classid:      mi.connectArgs.RoomId,
		Mtcv:         "0.0.1",
		Pv:           "1.1",
		UniqId:       strconv.Itoa(uniqId),
		InteractMode: 1,
		LiveType:     1,
		Route:        ROUTE_Connector_EntryHandler_Enter,
		ReqId:        int(atomic.LoadUint64(mi.chatReqId)),
	}

	err = syncRequest(ctx, connector, 30*time.Second, mi.chatReqId, ROUTE_Connector_EntryHandler_Enter, request, nil)
	if err != nil {
		return err
	}

	return nil
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

type EntryHandlerEnterRequest struct {
	Uid          int    `json:"uid"`
	Username     int    `json:"username"`
	Rtype        int    `json:"rtype"`
	Rid          string `json:"rid"`
	Role         int    `json:"role"` //1:学生，2:辅导，4:授课，3:旁听用户，5:游客
	Ulevel       int    `json:"ulevel"`
	Uname        int    `json:"uname"`
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

	err = connector.Request(route, requestBytes, func(data []byte) {

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
