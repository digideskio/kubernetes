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

package node

import (
	"sync"

	"k8s.io/kubernetes/pkg/client/cache"
	client "k8s.io/kubernetes/pkg/client/unversioned"
)

type Registrator struct {
	lock sync.Mutex
	store cache.Store
	client *client.Client

	inProgress map[string]struct{}
}

func NewRegistator(client *client.Client, store cache.Store) *Registrator {
	return &Registrator{
		store: store,
		client: client,
	}
}

func (r *Registrator) Blocked(hostName string) bool {
	r.lock.Lock()
	defer r.lock.Unlock()

	_, blocked := r.inProgress[hostName]
	return blocked
}

func (r *Registrator) block(hostName string) bool {
	r.lock.Lock()
	defer r.lock.Unlock()

	_, blocked := r.inProgress[hostName]
	if blocked {
		return false
	}

	r.inProgress[hostName] = struct{}{}
	return true
}

func (r *Registrator) unblock(hostName string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.inProgress, hostName)
}

func (r *Registrator) Register(hostName string, labels map[string]string) (bool, error) {
	if gotBlocked := r.block(hostName); !gotBlocked {
		return false, nil
	}
	defer r.unblock(hostName)

	_, err := CreateOrUpdate(r.client, hostName, labels)
	return true, err
}
