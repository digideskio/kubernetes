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
	"log"
	"sync"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/watch"
)

// Create mock of pods ListWatch, usually listening on the apiserver pods watch endpoint
type mockPodsListWatch struct {
	listWatch   cache.ListWatch
	fakeWatcher *watch.FakeWatcher
	list        api.PodList
	lock        sync.Mutex
}

func newMockPodsListWatch(initialPodList api.PodList) *mockPodsListWatch {
	lw := mockPodsListWatch{
		fakeWatcher: watch.NewFake(),
		list:        initialPodList,
	}
	lw.listWatch = cache.ListWatch{
		WatchFunc: func(resourceVersion string) (watch.Interface, error) {
			return lw.fakeWatcher, nil
		},
		ListFunc: func() (runtime.Object, error) {
			lw.lock.Lock()
			defer lw.lock.Unlock()

			listCopy, err := api.Scheme.DeepCopy(&lw.list)
			return listCopy.(*api.PodList), err
		},
	}
	return &lw
}

func (lw *mockPodsListWatch) Pods() api.PodList {
	lw.lock.Lock()
	defer lw.lock.Unlock()

	return lw.list
}

func (lw *mockPodsListWatch) Pod(name string) *api.Pod {
	lw.lock.Lock()
	defer lw.lock.Unlock()

	for _, p := range lw.list.Items {
		if p.Name == name {
			return &p
		}
	}

	return nil
}

func (lw *mockPodsListWatch) Add(pod *api.Pod, notify bool) {
	lw.lock.Lock()
	defer lw.lock.Unlock()

	lw.list.Items = append(lw.list.Items, *pod)
	if notify {
		lw.fakeWatcher.Add(pod)
	}
}

func (lw *mockPodsListWatch) Modify(pod *api.Pod, notify bool) {
	lw.lock.Lock()
	defer lw.lock.Unlock()

	for i, otherPod := range lw.list.Items {
		if otherPod.Name == pod.Name {
			lw.list.Items[i] = *pod
			if notify {
				lw.fakeWatcher.Modify(pod)
			}
			return
		}
	}
	log.Fatalf("Cannot find pod %v to modify in MockPodsListWatch", pod.Name)
}

func (lw *mockPodsListWatch) Delete(pod *api.Pod, notify bool) {
	lw.lock.Lock()
	defer lw.lock.Unlock()

	for i, otherPod := range lw.list.Items {
		if otherPod.Name == pod.Name {
			lw.list.Items = append(lw.list.Items[:i], lw.list.Items[i+1:]...)
			if notify {
				lw.fakeWatcher.Delete(&otherPod)
			}
			return
		}
	}
	log.Fatalf("Cannot find pod %v to delete in MockPodsListWatch", pod.Name)
}
