import {check} from 'k6';

const mqtt = require('k6/x/mqtt');

export default function () {
    // Message content one per ITER
    const k6Message = `k6-message-content-${rnd} ${__VU}:${__ITER}`;
    check(publisher, {
        "is publisher connected": publisher => publisher.isConnected()
    });
    check(subscriber, {
        "is subcriber connected": subscriber => subscriber.isConnected()
    });

    // subscribe first
    try {
        subscriber.subscribe(
            // topic to be used
            k6Topic,
            // The QoS of messages
            1,
            // timeout in ms
            subscribeTimeout,
        )
    } catch (error) {
        err = error
    }

    if (err != undefined) {
        console.error("subscribe error:", err)
        // you may want to use fail here if you want only to test successfull connection only
        // fail("fatal could not connect to broker for subscribe")
    }
    let count = messageCount;
    subscriber.addEventListener("message", (obj) => {
        // closing as we received one message
        let message = obj.message
        check(message, {
            "message received": msg => msg != undefined
        });
        check(message, {
            "is content correct": msg => msg == k6Message
        });

        if (--count > 0) {
            // tell the subscriber that you want to wait for more than one message
            // if you don't call subContinue you'll receive only one message per subscribe
            subscriber.subContinue();
        }
    })
    subscriber.addEventListener("error", (err) => {
        check(null, {
            "message received": false
        });
    })
    for (let i = 0; i < messageCount; i++) {
        // publish count messages
        let err_publish;
        try {
            publisher.publish(
                // topic to be used
                k6Topic,
                // The QoS of messages
                1,
                // Message to be sent
                k6Message,
                // retain policy on message
                false,
                // timeout in ms
                publishTimeout,
                // async publish handlers if needed
                // (obj) => { // success
                //     console.log(obj.type) // publish
                //     console.log(obj.topic) // published topic
                // },
                // (err) => { // failure
                //     console.log(err.type)  // error
                //     console.log(err.message)
                // }
            );
        } catch (error) {
            err_publish = error
        }
        check(err_publish, {
            "is sent": err => err == undefined
        });
    }
}

export function teardown() {
    // closing both connections at VU close
    publisher.close(closeTimeout);
    subscriber.close(closeTimeout);
}
