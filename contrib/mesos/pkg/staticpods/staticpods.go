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

package staticpods

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/golang/glog"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/meta"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/v1"
)

func GZipPodList(list *api.PodList) ([]byte, error) {
	raw, err := v1.Codec.Encode(list)
	if err != nil {
		return nil, err
	}

	zipped := &bytes.Buffer{}
	zw := gzip.NewWriter(zipped)
	_, err = bytes.NewBuffer(raw).WriteTo(zw)
	if err != nil {
		return nil, err
	}

	err = zw.Close()
	if err != nil {
		return nil, err
	}

	return zipped.Bytes(), nil
}

func GUnzipToDir(gzipped []byte, destDir string) error {
	podlist, err := GUnzipPodList(gzipped)
	if err != nil {
		return err
	}
	return writePodsToFiles(podlist, destDir)
}

func GUnzipPodList(gzipped []byte) (*api.PodList, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gzipped))
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	raw, err := ioutil.ReadAll(zr)
	if err != nil {
		return nil, err
	}

	obj, err := api.Scheme.Decode(raw)
	if err != nil {
		return nil, err
	}

	podlist, ok := obj.(*api.PodList)
	if !ok {
		return nil, fmt.Errorf("expected *api.PodList instead of %T", obj)
	}

	return podlist, nil
}

func writePodsToFiles(pods *api.PodList, destDir string) error {
	err := os.MkdirAll(destDir, 0660)
	if err != nil {
		return err
	}

	for i := range pods.Items {
		p := &pods.Items[i]
		filename, ok := p.Annotations[meta.StaticPodFilename]
		if !ok {
			log.Warningf("skipping static pod %s/%s that had no filename", p.Namespace, p.Name)
			continue
		}
		raw, err := v1.Codec.Encode(p)
		if err != nil {
			log.Errorf("failed to encode static pod as v1 object: %v", err)
			continue
		}
		destfile := filepath.Join(destDir, filename)
		err = ioutil.WriteFile(destfile, raw, 0660)
		if err != nil {
			log.Errorf("failed to write static pod file %q: %v", destfile, err)
		}
	}
	return nil
}
