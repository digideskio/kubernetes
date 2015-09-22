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

type LaunchedTask struct {
	offerId  mesos.OfferID
	taskInfo *mesos.TaskInfo
}

type LifecycleTest struct {
	ApiServer     *TestServer
	Driver        *driver.JoinableDriver
	EventObs      *EventObserver
	Plugin        scheduler.PluginInterface
	PodsListWatch *MockPodsListWatch
	Scheduler     *scheduler.KubernetesScheduler
	SchedulerProc *ha.SchedulerProcess
	t             *testing.T
}

func NewLifecycleTest(t *testing.T) LifecycleTest {
	assert := &EventAssertions{*assert.New(t)}

	// create a fake pod watch. We use that below to submit new pods to the scheduler
	podsListWatch := NewMockPodsListWatch(api.PodList{})

	// create fake apiserver
	ApiServer := NewTestServer(t, api.NamespaceDefault, podsListWatch)

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
			Host:    ApiServer.server.URL,
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
		&podsListWatch.ListWatch,
	)
	assert.NotNil(config)

	// make events observable
	eventObs := NewEventObserver()
	config.Recorder = eventObs

	// create plugin
	plugin := scheduler.NewPlugin(config)
	assert.NotNil(plugin)

	return LifecycleTest{
		ApiServer:     ApiServer,
		Driver:        &driver.JoinableDriver{},
		EventObs:      eventObs,
		Plugin:        plugin,
		PodsListWatch: podsListWatch,
		Scheduler:     sched,
		SchedulerProc: schedProc,
		t:             t,
	}
}

func (lt LifecycleTest) Start() <-chan LaunchedTask {
	assert := &EventAssertions{*assert.New(lt.t)}
	lt.Plugin.Run(lt.SchedulerProc.Terminal())

	// init scheduler
	err := lt.Scheduler.Init(
		lt.SchedulerProc.Master(),
		lt.Plugin,
		http.DefaultServeMux,
	)
	assert.NoError(err)

	lt.Driver.On("Start").Return(mesos.Status_DRIVER_RUNNING, nil).Once()
	started := lt.Driver.Upon()

	lt.Driver.On("ReconcileTasks",
		mock.AnythingOfType("[]*mesosproto.TaskStatus"),
	).Return(mesos.Status_DRIVER_RUNNING, nil)

	lt.Driver.On("SendFrameworkMessage",
		mock.AnythingOfType("*mesosproto.ExecutorID"),
		mock.AnythingOfType("*mesosproto.SlaveID"),
		mock.AnythingOfType("string"),
	).Return(mesos.Status_DRIVER_RUNNING, nil)

	launchedTasks := make(chan LaunchedTask, 1)

	launchTasksFunc := func(args mock.Arguments) {
		offerIDs := args.Get(0).([]*mesos.OfferID)
		taskInfos := args.Get(1).([]*mesos.TaskInfo)
		assert.Equal(1, len(offerIDs))
		assert.Equal(1, len(taskInfos))

		launchedTasks <- LaunchedTask{
			offerId:  *offerIDs[0],
			taskInfo: taskInfos[0],
		}
	}

	lt.Driver.On("LaunchTasks",
		mock.AnythingOfType("[]*mesosproto.OfferID"),
		mock.AnythingOfType("[]*mesosproto.TaskInfo"),
		mock.AnythingOfType("*mesosproto.Filters"),
	).Return(mesos.Status_DRIVER_RUNNING, nil).Run(launchTasksFunc)

	lt.Driver.On("DeclineOffer",
		mock.AnythingOfType("*mesosproto.OfferID"),
		mock.AnythingOfType("*mesosproto.Filters"),
	).Return(mesos.Status_DRIVER_RUNNING, nil)

	// elect master with mock driver
	driverFactory := ha.DriverFactory(func() (bindings.SchedulerDriver, error) {
		return lt.Driver, nil
	})
	lt.SchedulerProc.Elect(driverFactory)
	elected := lt.SchedulerProc.Elected()

	// driver will be started
	<-started

	// tell scheduler to be registered
	lt.Scheduler.Registered(
		lt.Driver,
		util.NewFrameworkID("kubernetes-id"),
		util.NewMasterInfo("master-id", (192<<24)+(168<<16)+(0<<8)+1, 5050),
	)

	// wait for being elected
	<-elected
	return launchedTasks
}

func (lt LifecycleTest) Close() {
	lt.ApiServer.server.Close()
}

func (lt LifecycleTest) End() <-chan struct{} {
	return lt.SchedulerProc.End()
}
