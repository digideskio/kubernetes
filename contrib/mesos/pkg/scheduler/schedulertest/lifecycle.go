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
	"net/http"
	"testing"

	mesos "github.com/mesos/mesos-go/mesosproto"
	util "github.com/mesos/mesos-go/mesosutil"
	bindings "github.com/mesos/mesos-go/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler"
	schedcfg "k8s.io/kubernetes/contrib/mesos/pkg/scheduler/config"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/ha"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/podtask"
	mresource "k8s.io/kubernetes/contrib/mesos/pkg/scheduler/resource"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/schedulertest/driver"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/testapi"
	client "k8s.io/kubernetes/pkg/client/unversioned"
)

type launchedTask struct {
	offerId  mesos.OfferID
	taskInfo *mesos.TaskInfo
}

type lifecycleTest struct {
	apiServer     *testServer
	driver        *driver.JoinableDriver
	eventObs      *eventObserver
	plugin        scheduler.PluginInterface
	podsListWatch *mockPodsListWatch
	scheduler     *scheduler.KubernetesScheduler
	schedulerProc *ha.SchedulerProcess
	t             *testing.T
}

func newLifecycleTest(t *testing.T) lifecycleTest {
	assert := &eventAssertions{*assert.New(t)}

	// create a fake pod watch. We use that below to submit new pods to the scheduler
	podsListWatch := newMockPodsListWatch(api.PodList{})

	// create fake apiserver
	apiServer := newTestServer(t, api.NamespaceDefault, podsListWatch)

	// create executor with some data for static pods if set
	executor := util.NewExecutorInfo(
		util.NewExecutorID("executor-id"),
		util.NewCommandInfo("executor-cmd"),
	)
	executor.Data = []byte{0, 1, 2}

	// create scheduler
	strategy := scheduler.NewAllocationStrategy(
		podtask.DefaultPredicate,
		podtask.NewDefaultProcurement(
			mresource.DefaultDefaultContainerCPULimit,
			mresource.DefaultDefaultContainerMemLimit,
		),
	)

	sched := scheduler.New(scheduler.Config{
		Executor: executor,
		Client: client.NewOrDie(&client.Config{
			Host:    apiServer.server.URL,
			Version: testapi.Default.Version(),
		}),
		Scheduler: scheduler.NewFCFSPodScheduler(strategy),
		Schedcfg:  *schedcfg.CreateDefaultConfig(),
	})

	assert.NotNil(sched.client, "client is nil")
	assert.NotNil(sched.executor, "executor is nil")
	assert.NotNil(sched.offers, "offer registry is nil")

	// create scheduler process
	schedProc := ha.New(sched)

	// get plugin config from it
	config := sched.NewPluginConfig(
		schedProc.Terminal(),
		http.DefaultServeMux,
		&podsListWatch.listWatch,
	)
	assert.NotNil(config)

	// make events observable
	eventObs := newEventObserver()
	config.Recorder = eventObs

	// create plugin
	plugin := scheduler.NewPlugin(config)
	assert.NotNil(plugin)

	return lifecycleTest{
		apiServer:     apiServer,
		driver:        &driver.JoinableDriver{},
		eventObs:      eventObs,
		plugin:        plugin,
		podsListWatch: podsListWatch,
		scheduler:     sched,
		schedulerProc: schedProc,
		t:             t,
	}
}

func (lt lifecycleTest) Start() <-chan launchedTask {
	assert := &eventAssertions{*assert.New(lt.t)}
	lt.plugin.Run(lt.schedulerProc.Terminal())

	// init scheduler
	err := lt.scheduler.Init(
		lt.schedulerProc.Master(),
		lt.plugin,
		http.DefaultServeMux,
	)
	assert.NoError(err)

	lt.driver.On("Start").Return(mesos.Status_DRIVER_RUNNING, nil).Once()
	started := lt.driver.Upon()

	lt.driver.On("ReconcileTasks",
		mock.AnythingOfType("[]*mesosproto.TaskStatus"),
	).Return(mesos.Status_DRIVER_RUNNING, nil)

	lt.driver.On("SendFrameworkMessage",
		mock.AnythingOfType("*mesosproto.ExecutorID"),
		mock.AnythingOfType("*mesosproto.SlaveID"),
		mock.AnythingOfType("string"),
	).Return(mesos.Status_DRIVER_RUNNING, nil)

	launchedTasks := make(chan launchedTask, 1)

	launchTasksFunc := func(args mock.Arguments) {
		offerIDs := args.Get(0).([]*mesos.OfferID)
		taskInfos := args.Get(1).([]*mesos.TaskInfo)
		assert.Equal(1, len(offerIDs))
		assert.Equal(1, len(taskInfos))

		launchedTasks <- launchedTask{
			offerId:  *offerIDs[0],
			taskInfo: taskInfos[0],
		}
	}

	lt.driver.On("LaunchTasks",
		mock.AnythingOfType("[]*mesosproto.OfferID"),
		mock.AnythingOfType("[]*mesosproto.TaskInfo"),
		mock.AnythingOfType("*mesosproto.Filters"),
	).Return(mesos.Status_DRIVER_RUNNING, nil).Run(launchTasksFunc)

	lt.driver.On("DeclineOffer",
		mock.AnythingOfType("*mesosproto.OfferID"),
		mock.AnythingOfType("*mesosproto.Filters"),
	).Return(mesos.Status_DRIVER_RUNNING, nil)

	// elect master with mock driver
	driverFactory := ha.DriverFactory(func() (bindings.SchedulerDriver, error) {
		return lt.driver, nil
	})
	lt.schedulerProc.Elect(driverFactory)
	elected := lt.schedulerProc.Elected()

	// driver will be started
	<-started

	// tell scheduler to be registered
	lt.scheduler.Registered(
		lt.driver,
		util.NewFrameworkID("kubernetes-id"),
		util.NewMasterInfo("master-id", (192<<24)+(168<<16)+(0<<8)+1, 5050),
	)

	// wait for being elected
	<-elected
	return launchedTasks
}

func (lt lifecycleTest) Close() {
	lt.apiServer.server.Close()
}

func (lt lifecycleTest) End() <-chan struct{} {
	return lt.schedulerProc.End()
}
