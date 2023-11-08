package pomelo

import (
	"github.com/stretchr/testify/require"
	"go.k6.io/k6/js/modulestest"
	"go.k6.io/k6/lib"
	"go.k6.io/k6/lib/testutils/httpmultibin"
	"go.k6.io/k6/metrics"
	"gopkg.in/guregu/null.v3"
	"testing"
	"time"
)

type testState struct {
	*modulestest.Runtime
	tb      *httpmultibin.HTTPMultiBin
	samples chan metrics.SampleContainer
}

func newTestState(t testing.TB) testState {
	tb := httpmultibin.NewHTTPMultiBin(t)

	testRuntime := modulestest.NewRuntime(t)
	samples := make(chan metrics.SampleContainer, 1000)

	root, err := lib.NewGroup("", nil)
	require.NoError(t, err)
	registry := metrics.NewRegistry()
	state := &lib.State{
		Group:  root,
		Dialer: tb.Dialer,
		Options: lib.Options{
			SystemTags: metrics.NewSystemTagSet(
				metrics.TagURL,
				metrics.TagProto,
				metrics.TagStatus,
				metrics.TagSubproto,
			),
			UserAgent: null.StringFrom("TestUserAgent"),
			Throw:     null.BoolFrom(true),
		},
		Samples:        samples,
		TLSConfig:      tb.TLSClientConfig,
		BuiltinMetrics: metrics.RegisterBuiltinMetrics(registry),
		Tags:           lib.NewVUStateTags(registry.RootTagSet()),
	}

	m := new(RootModule).NewModuleInstance(testRuntime.VU)
	require.NoError(t, testRuntime.VU.RuntimeField.Set("pomelo", m.Exports().Default))
	testRuntime.MoveToVUContext(state)

	return testState{
		Runtime: testRuntime,
		tb:      tb,
		samples: samples,
	}
}

func TestSessionConnectWs(t *testing.T) {
	// TODO: split and paralelize tests
	t.Parallel()
	tb := httpmultibin.NewHTTPMultiBin(t)
	sr := tb.Replacer.Replace

	test := newTestState(t)
	res, err := test.VU.Runtime().RunString(sr(`
     let args = {
        "uid": "zhengjiaming1",
        "channelId": 2,
        "roomId": "RoomId",
        "chatAddress": "wss://dmsp-ch2-test.speiyou.com:443",
    }

    var client=pomelo.connect(args);


    let message = {
        "topic": "interaction_up",
        "tag": "ANSWER",
        "body": {
            "from": "stuUid",
            "to": "*",
            "channelId": "${tutorId}_${liveId}",
            "channelType": 2,
            "content": {
                "protocolVersion": "1.0",
                "parentType": "interaction",
                "type": "choose",
                "module": "ANSWER",
                "command": "ANSWER",
                "online_trace_id": "pcStudent-${__UUID()}",
                "liveId": "${liveId}",
                "classId": "${classId}",
                "lecturerId": "${lecturerId}",
                "tutorId": "${tutorId}",
                "stuId": "${stuUid}",
                "body": "{\"timestamp\":${__time()},\"actId\":\"${actId}\",\"ext\":{\"answerTime\":\"-1\"},\"actTempId\":\"5\",\"answerList\":[{\"queId\":\"6\",\"queType\":\"choose\",\"answer\":\"A\",\"score\":0,\"realScore\":0}]}"
            },
            "source": "pc"
        }
    }

     client.request("producer.msgHandler.publish", JSON.stringify(message), function(data) {
        console.log('callback , data:', data)
    });
		`))

	time.Sleep(6 * time.Second)

	require.NoError(t, err)
	t.Log(res)
}

func TestSessionConnect2(t *testing.T) {
	// TODO: split and paralelize tests
	t.Parallel()
	tb := httpmultibin.NewHTTPMultiBin(t)
	sr := tb.Replacer.Replace

	test := newTestState(t)
	res, err := test.VU.Runtime().RunString(sr(`
    let args = {
        "uid": "zhengjiaming1",
        "channelId": 1,
        "roomId": "RoomId",
        "chatAddress": "wss://dmsp-ch1-test.speiyou.com:443",
    }

   var client1= pomelo.connect(args);


    let args2 = {
        "uid": "zhengjiaming1",
        "channelId": 2,
        "roomId": "RoomId",
        "chatAddress": "wss://dmsp-ch2-test.speiyou.com:443",
    }

     var client2=  pomelo.connect(args2);
`))

	time.Sleep(6 * time.Second)

	require.NoError(t, err)
	t.Log(res)
}
