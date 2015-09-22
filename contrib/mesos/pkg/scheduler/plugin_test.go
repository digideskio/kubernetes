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

package scheduler

import (
	"sync"
	"testing"

	"k8s.io/kubernetes/pkg/api"

	mesos "github.com/mesos/mesos-go/mesosproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/kubernetes/contrib/mesos/pkg/offers"
	"k8s.io/kubernetes/contrib/mesos/pkg/queue"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/podtask"
)

// implements SchedulerInterface
type MockScheduler struct {
	sync.RWMutex
	mock.Mock
}

func (m *MockScheduler) slaveHostNameFor(id string) (hostName string) {
	args := m.Called(id)
	x := args.Get(0)
	if x != nil {
		hostName = x.(string)
	}
	return
}

func (m *MockScheduler) algorithm() (f PodScheduler) {
	args := m.Called()
	x := args.Get(0)
	if x != nil {
		f = x.(PodScheduler)
	}
	return
}

func (m *MockScheduler) createPodTask(ctx api.Context, pod *api.Pod) (task *podtask.T, err error) {
	args := m.Called(ctx, pod)
	x := args.Get(0)
	if x != nil {
		task = x.(*podtask.T)
	}
	err = args.Error(1)
	return
}

func (m *MockScheduler) offers() (f offers.Registry) {
	args := m.Called()
	x := args.Get(0)
	if x != nil {
		f = x.(offers.Registry)
	}
	return
}

func (m *MockScheduler) tasks() (f podtask.Registry) {
	args := m.Called()
	x := args.Get(0)
	if x != nil {
		f = x.(podtask.Registry)
	}
	return
}

func (m *MockScheduler) killTask(taskId string) error {
	args := m.Called(taskId)
	return args.Error(0)
}

func (m *MockScheduler) launchTask(task *podtask.T) error {
	args := m.Called(task)
	return args.Error(0)
}

// @deprecated this is a placeholder for me to test the mock package
func TestNoSlavesYet(t *testing.T) {
	obj := &MockScheduler{}
	obj.On("slaveHostNameFor", "foo").Return(nil)
	obj.slaveHostNameFor("foo")
	obj.AssertExpectations(t)
}

/*-----------------------------------------------------------------------------
 |
 |   this really belongs in the mesos-go package, but that's being updated soon
 |   any way so just keep it here for now unless we *really* need it there.
 |
 \-----------------------------------------------------------------------------

// Scheduler defines the interfaces that needed to be implemented.
type Scheduler interface {
        Registered(SchedulerDriver, *FrameworkID, *MasterInfo)
        Reregistered(SchedulerDriver, *MasterInfo)
        Disconnected(SchedulerDriver)
        ResourceOffers(SchedulerDriver, []*Offer)
        OfferRescinded(SchedulerDriver, *OfferID)
        StatusUpdate(SchedulerDriver, *TaskStatus)
        FrameworkMessage(SchedulerDriver, *ExecutorID, *SlaveID, string)
        SlaveLost(SchedulerDriver, *SlaveID)
        ExecutorLost(SchedulerDriver, *ExecutorID, *SlaveID, int)
        Error(SchedulerDriver, string)
}
*/

// Test to create the scheduler plugin with an empty plugin config
func TestPlugin_New(t *testing.T) {
	assert := assert.New(t)

	c := PluginConfig{}
	p := NewPlugin(&c)
	assert.NotNil(p)
}

func TestDeleteOne_NonexistentPod(t *testing.T) {
	assert := assert.New(t)
	obj := &MockScheduler{}
	reg := podtask.NewInMemoryRegistry()
	obj.On("tasks").Return(reg)

	qr := newQueuer(nil)
	assert.Equal(0, len(qr.podQueue.List()))
	d := &deleter{
		api: obj,
		qr:  qr,
	}
	pod := &Pod{Pod: &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name:      "foo",
			Namespace: api.NamespaceDefault,
		}}}
	err := d.deleteOne(pod)
	assert.Equal(err, noSuchPodErr)
	obj.AssertExpectations(t)
}

func TestDeleteOne_PendingPod(t *testing.T) {
	assert := assert.New(t)
	obj := &MockScheduler{}
	reg := podtask.NewInMemoryRegistry()
	obj.On("tasks").Return(reg)

	pod := &Pod{Pod: &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name:      "foo",
			UID:       "foo0",
			Namespace: api.NamespaceDefault,
		}}}
	_, err := reg.Register(podtask.New(api.NewDefaultContext(), "bar", *pod.Pod, &mesos.ExecutorInfo{}))
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// preconditions
	qr := newQueuer(nil)
	qr.podQueue.Add(pod, queue.ReplaceExisting)
	assert.Equal(1, len(qr.podQueue.List()))
	_, found := qr.podQueue.Get("default/foo")
	assert.True(found)

	// exec & post conditions
	d := &deleter{
		api: obj,
		qr:  qr,
	}
	err = d.deleteOne(pod)
	assert.Nil(err)
	_, found = qr.podQueue.Get("foo0")
	assert.False(found)
	assert.Equal(0, len(qr.podQueue.List()))
	obj.AssertExpectations(t)
}

func TestDeleteOne_Running(t *testing.T) {
	assert := assert.New(t)
	obj := &MockScheduler{}
	reg := podtask.NewInMemoryRegistry()
	obj.On("tasks").Return(reg)

	pod := &Pod{Pod: &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name:      "foo",
			UID:       "foo0",
			Namespace: api.NamespaceDefault,
		}}}
	task, err := reg.Register(podtask.New(api.NewDefaultContext(), "bar", *pod.Pod, &mesos.ExecutorInfo{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task.Set(podtask.Launched)
	err = reg.Update(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// preconditions
	qr := newQueuer(nil)
	qr.podQueue.Add(pod, queue.ReplaceExisting)
	assert.Equal(1, len(qr.podQueue.List()))
	_, found := qr.podQueue.Get("default/foo")
	assert.True(found)

	obj.On("killTask", task.ID).Return(nil)

	// exec & post conditions
	d := &deleter{
		api: obj,
		qr:  qr,
	}
	err = d.deleteOne(pod)
	assert.Nil(err)
	_, found = qr.podQueue.Get("foo0")
	assert.False(found)
	assert.Equal(0, len(qr.podQueue.List()))
	obj.AssertExpectations(t)
}

func TestDeleteOne_badPodNaming(t *testing.T) {
	assert := assert.New(t)
	obj := &MockScheduler{}
	pod := &Pod{Pod: &api.Pod{}}
	d := &deleter{
		api: obj,
		qr:  newQueuer(nil),
	}

	err := d.deleteOne(pod)
	assert.NotNil(err)

	pod.Pod.ObjectMeta.Name = "foo"
	err = d.deleteOne(pod)
	assert.NotNil(err)

	pod.Pod.ObjectMeta.Name = ""
	pod.Pod.ObjectMeta.Namespace = "bar"
	err = d.deleteOne(pod)
	assert.NotNil(err)

	obj.AssertExpectations(t)
}
