package pomelo

import (
	"errors"
	"go.k6.io/k6/lib"
	"log"
	"math/rand"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Jourmey/xk6-pomelo/pomelosdk"
	"github.com/dop251/goja"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
)

func init() {
	modules.Register("k6/x/pomelo", new(RootModule))
}

type (
	RootModule struct{}

	Instance struct {
		vu  modules.VU
		obj *goja.Object
	}

	Client struct {
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

type ConnectArgs struct {
	uid         string
	channelId   int
	roomId      string
	chatAddress string
}

// Exports returns the exports of the ws module.
func (mi *Instance) Exports() modules.Exports {
	return modules.Exports{Default: mi.obj}
}

func (mi *Instance) Connect(args goja.Value) (client *goja.Object, err error) {
	ctx := mi.vu.Context()
	rt := mi.vu.Runtime()
	state := mi.vu.State()
	if state == nil {
		return nil, errors.New("invalid state")
	}

	log.Println("pomelo.connect starting, arg:", args)

	parsedArgs, err := parseConnectArgs(state, rt, args)
	if err != nil {
		return nil, err
	}

	log.Println("pomelo.connect.parseConnectArgs , parsedArgs:", parsedArgs)

	//parsedArgs.tagsAndMeta.SetSystemTagOrMetaIfEnabled(state.Options.SystemTags, metrics.TagURL, url)

	connector := pomelosdk.NewConnector()

	cl := &Client{
		vu:                     mi.vu,
		obj:                    rt.NewObject(),
		connectArgs:            parsedArgs,
		chatConnectorConnected: false,
		chatConnector:          connector,
		chatReqId:              new(uint64),
		chatAckReqId:           new(uint64),
	}

	err = runAndWaitConnect(ctx, connector, parsedArgs.chatAddress, 10*time.Second)
	if err != nil {
		return nil, err
	}

	uniqId := rand.Int()

	request := entryHandlerEnterRequest{
		Uid:      cl.connectArgs.uid,
		Username: cl.connectArgs.uid,
		Uname:    cl.connectArgs.uid,

		Rtype:        cl.connectArgs.channelId,
		Rid:          cl.connectArgs.roomId,
		Role:         1, //1:学生，2:辅导，4:授课，3:旁听用户，5:游客
		Ulevel:       1,
		Classid:      cl.connectArgs.roomId,
		Mtcv:         "0.0.1",
		Pv:           "1.1",
		UniqId:       strconv.Itoa(uniqId),
		InteractMode: 1,
		LiveType:     1,
		Route:        ROUTE_Connector_EntryHandler_Enter,
		ReqId:        int(atomic.LoadUint64(cl.chatReqId)),
	}

	err = syncRequest(ctx, connector, 30*time.Second, cl.chatReqId, ROUTE_Connector_EntryHandler_Enter, request, nil)
	if err != nil {
		return nil, err
	}

	log.Println("pomelo.connect success, arg:", parsedArgs)

	// 监听ack回复
	cl.onEvent()
	cl.chatConnectorConnected = true
	if err := cl.obj.Set("request", cl.Request); err != nil {
		common.Throw(rt, err)
	}
	if err := cl.obj.Set("close", cl.Close); err != nil {
		common.Throw(rt, err)
	}

	return cl.obj, nil
}

//nolint:gocognit
func parseConnectArgs(state *lib.State, rt *goja.Runtime, args goja.Value) (res ConnectArgs, err error) {

	params := args.ToObject(rt)
	for _, k := range params.Keys() {
		switch k {

		case "uid":
			res.uid = params.Get(k).ToString().String()
		case "channelId":
			res.channelId = int(params.Get(k).ToInteger())
		case "roomId":
			res.roomId = params.Get(k).ToString().String()
		case "chatAddress":
			res.chatAddress = params.Get(k).ToString().String()
		}
	}

	return
}
