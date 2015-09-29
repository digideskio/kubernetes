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

package service

import (
	log "github.com/golang/glog"
	"k8s.io/kubernetes/pkg/kubelet"
	"k8s.io/kubernetes/pkg/util"
)

// executorKubelet decorates the kubelet with a Run function that terminates when
// executorDone is closed. Moreover, it closes kubeletDone to notify the executor.
type executorKubelet struct {
	*kubelet.Kubelet
	kubeletDone  chan<- struct{} // closed once kubelet.Run() returns
	executorDone <-chan struct{} // closed when executor terminates
}

// runs the main kubelet loop, closing the kubeletFinished chan when the loop exits.
// Returns when executorDone is closed.
func (kl *executorKubelet) Run(mergedUpdates <-chan kubelet.PodUpdate) {
	defer func() {
		close(kl.kubeletDone)
		util.HandleCrash()
		log.Infoln("kubelet run terminated") //TODO(jdef) turn down verbosity
		// important: never return! this is in our contract
		select {}
	}()

	// push merged updates into another, closable update channel which is closed
	// when the executor shuts down.
	closableUpdates := make(chan kubelet.PodUpdate)
	go func() {
		// closing closableUpdates will cause our patched kubelet's syncLoop() to exit
		defer close(closableUpdates)
	pipeLoop:
		for {
			select {
			case <-kl.executorDone:
				break pipeLoop
			default:
				select {
				case u := <-mergedUpdates:
					select {
					case closableUpdates <- u: // noop
					case <-kl.executorDone:
						break pipeLoop
					}
				case <-kl.executorDone:
					break pipeLoop
				}
			}
		}
	}()

	// we expect that Run() will complete after closableUpdates is closed and the
	// kubelet's syncLoop() has finished processing its backlog, which hopefully
	// will not take very long. Peeking into the future (current k8s master) it
	// seems that the backlog has grown from 1 to 50 -- this may negatively impact
	// us going forward, time will tell.
	util.Until(func() { kl.Kubelet.Run(closableUpdates) }, 0, kl.executorDone)

	//TODO(jdef) revisit this if/when executor failover lands
	// Force kubelet to delete all pods.
	kl.HandlePodDeletions(kl.GetPods())
}