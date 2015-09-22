/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package schedulertest

import (
	"fmt"
	"time"

	log "github.com/golang/glog"

	"github.com/stretchr/testify/assert"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/runtime"
)

// Add assertions to reason about event streams
type event struct {
	object  runtime.Object
	reason  string
	message string
}

type eventPredicate func(e event) bool

type eventAssertions struct {
	assert.Assertions
}

// EventObserver implements record.EventRecorder for the purposes of validation via EventAssertions.
type eventObserver struct {
	fifo chan event
}

func newEventObserver() *eventObserver {
	return &eventObserver{
		fifo: make(chan event, 1000),
	}
}

func (o *eventObserver) Event(object runtime.Object, reason, message string) {
	o.fifo <- event{object: object, reason: reason, message: message}
}

func (o *eventObserver) Eventf(object runtime.Object, reason, messageFmt string, args ...interface{}) {
	o.fifo <- event{object: object, reason: reason, message: fmt.Sprintf(messageFmt, args...)}
}

func (o *eventObserver) PastEventf(object runtime.Object, timestamp unversioned.Time, reason, messageFmt string, args ...interface{}) {
	o.fifo <- event{object: object, reason: reason, message: fmt.Sprintf(messageFmt, args...)}
}

func (a *eventAssertions) Event(observer *eventObserver, pred eventPredicate, msgAndArgs ...interface{}) bool {
	// parse msgAndArgs: first possibly a duration, otherwise a format string with further args
	timeout := time.Minute
	msg := "event not received"
	msgArgStart := 0
	if len(msgAndArgs) > 0 {
		switch msgAndArgs[0].(type) {
		case time.Duration:
			timeout = msgAndArgs[0].(time.Duration)
			msgArgStart += 1
		}
	}
	if len(msgAndArgs) > msgArgStart {
		msg = fmt.Sprintf(msgAndArgs[msgArgStart].(string), msgAndArgs[msgArgStart+1:]...)
	}

	// watch events
	result := make(chan bool)
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case e, ok := <-observer.fifo:
				if !ok {
					result <- false
					return
				} else if pred(e) {
					log.V(3).Infof("found asserted event for reason '%v': %v", e.reason, e.message)
					result <- true
					return
				} else {
					log.V(5).Infof("ignoring not-asserted event for reason '%v': %v", e.reason, e.message)
				}
			case _, ok := <-stop:
				if !ok {
					return
				}
			}
		}
	}()
	defer close(stop)

	// wait for watch to match or timeout
	select {
	case matched := <-result:
		return matched
	case <-time.After(timeout):
		return a.Fail(msg)
	}
}

func (a *eventAssertions) EventWithReason(observer *eventObserver, reason string, msgAndArgs ...interface{}) bool {
	return a.Event(observer, func(e event) bool {
		return e.reason == reason
	}, msgAndArgs...)
}
