/*
Copyright 2017 The Kubernetes Authors.

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

package storage

import (
	"fmt"
	"strings"

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/storage/testpatterns"
	"k8s.io/kubernetes/test/e2e/storage/testsuites"
	"k8s.io/kubernetes/test/e2e/storage/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func csiTunePattern(patterns []testpatterns.TestPattern) []testpatterns.TestPattern {
	tunedPatterns := []testpatterns.TestPattern{}

	for _, pattern := range patterns {
		// Skip inline volume and pre-provsioned PV tests for csi drivers
		if pattern.VolType == testpatterns.InlineVolume || pattern.VolType == testpatterns.PreprovisionedPV {
			continue
		}
		tunedPatterns = append(tunedPatterns, pattern)
	}

	return tunedPatterns
}

var _ = Describe("PMEM Volumes", func() {
	// List of testDrivers to be executed in below loop
	var csiTestDrivers = []func() testsuites.TestDriver{
		// pmem-csi
		func() testsuites.TestDriver {
			return &manifestDriver{
				driverInfo: testsuites.DriverInfo{
					Name:        "pmem-csi",
					MaxFileSize: testpatterns.FileSizeMedium,
					SupportedFsType: sets.NewString(
						"", // Default fsType
					),
					Capabilities: map[testsuites.Capability]bool{
						testsuites.CapPersistence: true,
						testsuites.CapFsGroup:     true,
						testsuites.CapExec:        true,
					},
				},
				scManifest: "deploy/kubernetes-1.13/pmem-storageclass-ext4.yaml",
				// Renaming of the driver *not* enabled. It doesn't support
				// that because there is only one instance of the registry
				// and on each node the driver assumes that it has exclusive
				// control of the PMEM. As a result, tests have to be run
				// sequentially becaust each test creates and removes
				// the driver deployment.
				claimSize: "1Mi",
			}
		},
	}

	// List of testSuites to be executed in below loop
	var csiTestSuites = []func() testsuites.TestSuite{
		// TODO: investigate how useful these tests are and enable them.
		// testsuites.InitMultiVolumeTestSuite,
		testsuites.InitProvisioningTestSuite,
		// testsuites.InitSnapshottableTestSuite,
		// testsuites.InitSubPathTestSuite,
		// testsuites.InitVolumeIOTestSuite,
		// testsuites.InitVolumeModeTestSuite,
		// testsuites.InitVolumesTestSuite,
	}

	for _, initDriver := range csiTestDrivers {
		curDriver := initDriver()
		Context(testsuites.GetDriverNameWithFeatureTags(curDriver), func() {
			testsuites.DefineTestSuite(curDriver, csiTestSuites)
		})
	}
})

type manifestDriver struct {
	driverInfo   testsuites.DriverInfo
	patchOptions utils.PatchCSIOptions
	manifests    []string
	scManifest   string
	claimSize    string
	cleanup      func()
}

var _ testsuites.TestDriver = &manifestDriver{}
var _ testsuites.DynamicPVTestDriver = &manifestDriver{}

func (m *manifestDriver) GetDriverInfo() *testsuites.DriverInfo {
	return &m.driverInfo
}

func (m *manifestDriver) SkipUnsupportedTest(testpatterns.TestPattern) {
}

func (m *manifestDriver) GetDynamicProvisionStorageClass(config *testsuites.PerTestConfig, fsType string) *storagev1.StorageClass {
	f := config.Framework

	items, err := f.LoadFromManifests(m.scManifest)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(items)).To(Equal(1), "exactly one item from %s", m.scManifest)

	err = f.PatchItems(items...)
	Expect(err).NotTo(HaveOccurred())
	err = utils.PatchCSIDeployment(f, m.finalPatchOptions(f), items[0])

	sc, ok := items[0].(*storagev1.StorageClass)
	Expect(ok).To(BeTrue(), "storage class from %s", m.scManifest)
	return sc
}

func (m *manifestDriver) GetClaimSize() string {
	return m.claimSize
}

func (m *manifestDriver) PrepareTest(f *framework.Framework) (*testsuites.PerTestConfig, func()) {
	By(fmt.Sprintf("deploying %s driver", m.driverInfo.Name))
	config := &testsuites.PerTestConfig{
		Driver:    m,
		Prefix:    "pmem",
		Framework: f,
	}
	cleanup, err := f.CreateFromManifests(func(item interface{}) error {
		return utils.PatchCSIDeployment(f, m.finalPatchOptions(f), item)
	},
		m.manifests...,
	)
	framework.ExpectNoError(err, "deploying driver %s", m.driverInfo.Name)
	return config, func() {
		By(fmt.Sprintf("uninstalling %s driver", m.driverInfo.Name))
		cleanup()
	}
}

func (m *manifestDriver) finalPatchOptions(f *framework.Framework) utils.PatchCSIOptions {
	o := m.patchOptions
	// Unique name not available yet when configuring the driver.
	if strings.HasSuffix(o.NewDriverName, "-") {
		o.NewDriverName += f.UniqueName
	}
	return o
}
