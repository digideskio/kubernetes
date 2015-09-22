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
	"testing"
	"time"

	mesos "github.com/mesos/mesos-go/mesosproto"
	util "github.com/mesos/mesos-go/mesosutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	assertext "k8s.io/kubernetes/contrib/mesos/pkg/assert"
	"k8s.io/kubernetes/contrib/mesos/pkg/executor/messages"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/meta"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/podtask"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/testapi"
	"k8s.io/kubernetes/pkg/api/unversioned"
)

// Create a pod with a given index, requiring one port
var currentPodNum int = 0

func NewTestPod() (*api.Pod, int) {
	currentPodNum = currentPodNum + 1
	name := fmt.Sprintf("pod%d", currentPodNum)
	return &api.Pod{
		TypeMeta: unversioned.TypeMeta{APIVersion: testapi.Default.Version()},
		ObjectMeta: api.ObjectMeta{
			Name:      name,
			Namespace: api.NamespaceDefault,
			SelfLink:  fmt.Sprintf("http://1.2.3.4/api/v1beta1/pods/%s", name),
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Ports: []api.ContainerPort{
						{
							ContainerPort: 8000 + currentPodNum,
							Protocol:      api.ProtocolTCP,
						},
					},
				},
			},
		},
		Status: api.PodStatus{
			PodIP: fmt.Sprintf("1.2.3.%d", 4+currentPodNum),
			Conditions: []api.PodCondition{
				{
					Type:   api.PodReady,
					Status: api.ConditionTrue,
				},
			},
		},
	}, currentPodNum
}

// Test to create the scheduler plugin with the config returned by the scheduler,
// and play through the whole life cycle of the plugin while creating pods, deleting
// and failing them.
func TestPlugin_LifeCycle(t *testing.T) {
	assert := &eventAssertions{*assert.New(t)}

	lt := newLifecycleTest(t)
	defer lt.Close()

	// run plugin
	launchedTasks := lt.Start()
	defer lt.End()

	// fake new, unscheduled pod
	pod, i := NewTestPod()
	lt.podsListWatch.Add(pod, true) // notify watchers

	// wait for failedScheduling event because there is no offer
	assert.EventWithReason(lt.eventObs, scheduler.FailedScheduling, "failedScheduling event not received")

	// add some matching offer
	offers := []*mesos.Offer{newTestOffer(fmt.Sprintf("offer%d", i))}
	lt.scheduler.ResourceOffers(nil, offers)

	// and wait for scheduled pod
	assert.EventWithReason(lt.eventObs, scheduler.Scheduled)
	select {
	case launchedTask := <-launchedTasks:
		// report back that the task has been staged, and then started by mesos
		lt.scheduler.StatusUpdate(
			lt.driver,
			newTaskStatusForTask(launchedTask.taskInfo, mesos.TaskState_TASK_STAGING),
		)

		lt.scheduler.StatusUpdate(
			lt.driver,
			newTaskStatusForTask(launchedTask.taskInfo, mesos.TaskState_TASK_RUNNING),
		)

		// check that ExecutorInfo.data has the static pod data
		assert.Len(launchedTask.taskInfo.Executor.Data, 3)

		// report back that the task has been lost
		lt.driver.AssertNumberOfCalls(t, "SendFrameworkMessage", 0)

		lt.scheduler.StatusUpdate(
			lt.driver,
			newTaskStatusForTask(launchedTask.taskInfo, mesos.TaskState_TASK_LOST),
		)

		// and wait that framework message is sent to executor
		lt.driver.AssertNumberOfCalls(t, "SendFrameworkMessage", 1)

	case <-time.After(time.Minute):
		t.Fatalf("timed out waiting for launchTasks call")
	}

	// Launch a pod and wait until the scheduler driver is called
	schedulePodWithOffers := func(pod *api.Pod, offers []*mesos.Offer) (*api.Pod, *launchedTask, *mesos.Offer) {
		// wait for failedScheduling event because there is no offer
		assert.EventWithReason(lt.eventObs, scheduler.FailedScheduling, "failedScheduling event not received")

		// supply a matching offer
		lt.scheduler.ResourceOffers(lt.driver, offers)

		// and wait to get scheduled
		assert.EventWithReason(lt.eventObs, scheduler.Scheduled)

		// wait for driver.launchTasks call
		select {
		case launchedTask := <-launchedTasks:
			for _, offer := range offers {
				if offer.Id.GetValue() == launchedTask.offerId.GetValue() {
					return pod, &launchedTask, offer
				}
			}
			t.Fatalf("unknown offer used to start a pod")
			return nil, nil, nil
		case <-time.After(time.Minute):
			t.Fatal("timed out waiting for launchTasks")
			return nil, nil, nil
		}
	}

	// Launch a pod and wait until the scheduler driver is called
	launchPodWithOffers := func(pod *api.Pod, offers []*mesos.Offer) (*api.Pod, *launchedTask, *mesos.Offer) {
		lt.podsListWatch.Add(pod, true)
		return schedulePodWithOffers(pod, offers)
	}

	// Launch a pod, wait until the scheduler driver is called and report back that it is running
	startPodWithOffers := func(pod *api.Pod, offers []*mesos.Offer) (*api.Pod, *launchedTask, *mesos.Offer) {
		// notify about pod, offer resources and wait for scheduling
		pod, launchedTask, offer := launchPodWithOffers(pod, offers)
		if pod != nil {
			// report back status
			lt.scheduler.StatusUpdate(
				lt.driver,
				newTaskStatusForTask(launchedTask.taskInfo, mesos.TaskState_TASK_STAGING),
			)
			lt.scheduler.StatusUpdate(
				lt.driver,
				newTaskStatusForTask(launchedTask.taskInfo, mesos.TaskState_TASK_RUNNING),
			)

			return pod, launchedTask, offer
		}

		return nil, nil, nil
	}

	startTestPod := func() (*api.Pod, *launchedTask, *mesos.Offer) {
		pod, i := NewTestPod()
		offers := []*mesos.Offer{newTestOffer(fmt.Sprintf("offer%d", i))}
		return startPodWithOffers(pod, offers)
	}

	// start another pod
	pod, launched, _ := startTestPod()

	// mock driver.KillTask, should be invoked when a pod is deleted
	lt.driver.On("KillTask",
		mock.AnythingOfType("*mesosproto.TaskID"),
	).Return(mesos.Status_DRIVER_RUNNING, nil).Run(func(args mock.Arguments) {
		killedTaskId := *(args.Get(0).(*mesos.TaskID))
		assert.Equal(*launched.taskInfo.TaskId, killedTaskId, "expected same TaskID as during launch")
	})
	killTaskCalled := lt.driver.Upon()

	// stop it again via the apiserver mock
	lt.podsListWatch.Delete(pod, true) // notify watchers

	// and wait for the driver killTask call with the correct TaskId
	select {
	case <-killTaskCalled:
		// report back that the task is finished
		lt.scheduler.StatusUpdate(
			lt.driver,
			newTaskStatusForTask(launched.taskInfo, mesos.TaskState_TASK_FINISHED),
		)

	case <-time.After(time.Minute):
		t.Fatal("timed out waiting for KillTask")
	}

	// start a pod with on a given NodeName and check that it is scheduled to the right host
	pod, i = NewTestPod()
	pod.Spec.NodeName = "hostname1"
	offers = []*mesos.Offer{}
	for j := 0; j < 3; j++ {
		offer := newTestOffer(fmt.Sprintf("offer%d_%d", i, j))
		hostname := fmt.Sprintf("hostname%d", j)
		offer.Hostname = &hostname
		offers = append(offers, offer)
	}

	_, _, usedOffer := startPodWithOffers(pod, offers)

	assert.Equal(offers[1].Id.GetValue(), usedOffer.Id.GetValue())
	assert.Equal(pod.Spec.NodeName, *usedOffer.Hostname)

	lt.scheduler.OfferRescinded(lt.driver, offers[0].Id)
	lt.scheduler.OfferRescinded(lt.driver, offers[2].Id)

	// start pods:
	// - which are failing while binding,
	// - leading to reconciliation
	// - with different states on the apiserver

	failPodFromExecutor := func(task *mesos.TaskInfo) {
		beforePodLookups := lt.apiServer.Stats(pod.Name)
		status := newTaskStatusForTask(task, mesos.TaskState_TASK_FAILED)
		message := messages.CreateBindingFailure
		status.Message = &message
		lt.scheduler.StatusUpdate(lt.driver, status)

		// wait until pod is looked up at the apiserver
		assertext.EventuallyTrue(t, time.Second, func() bool {
			return lt.apiServer.Stats(pod.Name) == beforePodLookups+1
		}, "expect that reconcileTask will access apiserver for pod %v", pod.Name)
	}

	launchTestPod := func() (*api.Pod, *launchedTask, *mesos.Offer) {
		pod, i := NewTestPod()
		offers := []*mesos.Offer{newTestOffer(fmt.Sprintf("offer%d", i))}
		return launchPodWithOffers(pod, offers)
	}

	// 1. with pod deleted from the apiserver
	//    expected: pod is removed from internal task registry
	pod, launched, _ = launchTestPod()
	lt.podsListWatch.Delete(pod, false) // not notifying the watchers
	failPodFromExecutor(launched.taskInfo)

	podKey, _ := podtask.MakePodKey(api.NewDefaultContext(), pod.Name)
	assertext.EventuallyTrue(t, time.Second, func() bool {
		t, _ := lt.Plugin.api.tasks().ForPod(podKey)
		return t == nil
	})

	// 2. with pod still on the apiserver, not bound
	//    expected: pod is rescheduled
	pod, launched, _ = launchTestPod()
	failPodFromExecutor(launched.taskInfo)

	retryOffers := []*mesos.Offer{newTestOffer("retry-offer")}
	schedulePodWithOffers(pod, retryOffers)

	// 3. with pod still on the apiserver, bound, notified via ListWatch
	// expected: nothing, pod updates not supported, compare ReconcileTask function
	pod, launched, usedOffer = startTestPod()
	pod.Annotations = map[string]string{
		meta.BindingHostKey: *usedOffer.Hostname,
	}
	pod.Spec.NodeName = *usedOffer.Hostname
	lt.podsListWatch.Modify(pod, true) // notifying the watchers
	time.Sleep(time.Second / 2)
	failPodFromExecutor(launched.taskInfo)
}

// Create mesos.TaskStatus for a given task
func newTaskStatusForTask(task *mesos.TaskInfo, state mesos.TaskState) *mesos.TaskStatus {
	healthy := state == mesos.TaskState_TASK_RUNNING
	ts := float64(time.Now().Nanosecond()) / 1000000000.0
	source := mesos.TaskStatus_SOURCE_EXECUTOR
	return &mesos.TaskStatus{
		TaskId:     task.TaskId,
		State:      &state,
		SlaveId:    task.SlaveId,
		ExecutorId: task.Executor.ExecutorId,
		Timestamp:  &ts,
		Healthy:    &healthy,
		Source:     &source,
		Data:       task.Data,
	}
}

// Offering some cpus and memory and the 8000-9000 port range
func newTestOffer(id string) *mesos.Offer {
	hostname := "some_hostname"
	cpus := util.NewScalarResource("cpus", 3.75)
	mem := util.NewScalarResource("mem", 940)
	var port8000 uint64 = 8000
	var port9000 uint64 = 9000
	ports8000to9000 := mesos.Value_Range{Begin: &port8000, End: &port9000}
	ports := util.NewRangesResource("ports", []*mesos.Value_Range{&ports8000to9000})
	return &mesos.Offer{
		Id:        util.NewOfferID(id),
		Hostname:  &hostname,
		SlaveId:   util.NewSlaveID(hostname),
		Resources: []*mesos.Resource{cpus, mem, ports},
	}
}
