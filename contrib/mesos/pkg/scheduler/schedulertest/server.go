package schedulertest

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"k8s.io/kubernetes/pkg/api/testapi"
	"k8s.io/kubernetes/pkg/runtime"
)

// A apiserver mock which partially mocks the pods API
type TestServer struct {
	server *httptest.Server
	stats  map[string]uint
	lock   sync.Mutex
}

func NewTestServer(t *testing.T, namespace string, mockPodListWatch *MockPodsListWatch) *TestServer {
	ts := TestServer{
		stats: map[string]uint{},
	}
	mux := http.NewServeMux()

	podListHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		pods := mockPodListWatch.Pods()
		w.Write([]byte(runtime.EncodeOrDie(testapi.Default.Codec(), &pods)))
	}
	mux.HandleFunc(testapi.Default.ResourcePath("pods", namespace, ""), podListHandler)
	mux.HandleFunc(testapi.Default.ResourcePath("pods", "", ""), podListHandler)

	podsPrefix := testapi.Default.ResourcePath("pods", namespace, "") + "/"
	mux.HandleFunc(podsPrefix, func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[len(podsPrefix):]

		// update statistics for this pod
		ts.lock.Lock()
		defer ts.lock.Unlock()
		ts.stats[name] = ts.stats[name] + 1

		p := mockPodListWatch.Pod(name)
		if p != nil {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(runtime.EncodeOrDie(testapi.Default.Codec(), p)))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	mux.HandleFunc(testapi.Default.ResourcePath("events", namespace, ""), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		t.Errorf("unexpected request: %v", req.RequestURI)
		res.WriteHeader(http.StatusNotFound)
	})

	ts.server = httptest.NewServer(mux)
	return &ts
}

func (ts *TestServer) Stats(name string) uint {
	ts.lock.Lock()
	defer ts.lock.Unlock()

	return ts.stats[name]
}