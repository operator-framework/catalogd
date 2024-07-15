package storage

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing/fstest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

const urlPrefix = "/catalogs/"

var ctx = context.Background()

var _ = Describe("LocalDir Storage Test", func() {
	var (
		catalog                     = "test-catalog"
		store                       Instance
		rootDir                     string
		baseURL                     *url.URL
		testBundleName              = "bundle.v0.0.1"
		testBundleImage             = "quaydock.io/namespace/bundle:0.0.3"
		testBundleRelatedImageName  = "test"
		testBundleRelatedImageImage = "testimage:latest"
		testBundleObjectData        = "dW5pbXBvcnRhbnQK"
		testPackageDefaultChannel   = "preview_test"
		testPackageName             = "webhook_operator_test"
		testChannelName             = "preview_test"
		testPackage                 = fmt.Sprintf(testPackageTemplate, testPackageDefaultChannel, testPackageName)
		testBundle                  = fmt.Sprintf(testBundleTemplate, testBundleImage, testBundleName, testPackageName, testBundleRelatedImageName, testBundleRelatedImageImage, testBundleObjectData)
		testChannel                 = fmt.Sprintf(testChannelTemplate, testPackageName, testChannelName, testBundleName)

		unpackResultFS fs.FS
	)
	BeforeEach(func() {
		d, err := os.MkdirTemp(GinkgoT().TempDir(), "cache")
		Expect(err).ToNot(HaveOccurred())
		rootDir = d

		baseURL = &url.URL{Scheme: "http", Host: "test-addr", Path: urlPrefix}
		store = LocalDir{RootDir: rootDir, BaseURL: baseURL}
		unpackResultFS = &fstest.MapFS{
			"bundle.yaml":  &fstest.MapFile{Data: []byte(testBundle), Mode: os.ModePerm},
			"package.yaml": &fstest.MapFile{Data: []byte(testPackage), Mode: os.ModePerm},
			"channel.yaml": &fstest.MapFile{Data: []byte(testChannel), Mode: os.ModePerm},
		}
	})
	When("An unpacked FBC is stored using LocalDir", func() {
		BeforeEach(func() {
			err := store.Store(context.Background(), catalog, unpackResultFS)
			Expect(err).To(Not(HaveOccurred()))
		})
		It("should store the content in the RootDir correctly", func() {
			fbcFile := filepath.Join(rootDir, catalog, "all.json")
			_, err := os.Stat(fbcFile)
			Expect(err).To(Not(HaveOccurred()))

			gotConfig, err := declcfg.LoadFS(ctx, unpackResultFS)
			Expect(err).To(Not(HaveOccurred()))
			storedConfig, err := declcfg.LoadFile(os.DirFS(filepath.Join(rootDir, catalog)), "all.json")
			Expect(err).To(Not(HaveOccurred()))
			diff := cmp.Diff(gotConfig, storedConfig)
			Expect(diff).To(Equal(""))
		})
		It("should form the content URL correctly", func() {
			Expect(store.ContentURL(catalog)).To(Equal(fmt.Sprintf("%s%s/all.json", baseURL, catalog)))
		})
		It("should report content exists", func() {
			Expect(store.ContentExists(catalog)).To(BeTrue())
		})
		When("The stored content is deleted", func() {
			BeforeEach(func() {
				err := store.Delete(catalog)
				Expect(err).To(Not(HaveOccurred()))
			})
			It("should delete the FBC from the cache directory", func() {
				fbcFile := filepath.Join(rootDir, catalog)
				_, err := os.Stat(fbcFile)
				Expect(err).To(HaveOccurred())
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
			It("should report content does not exist", func() {
				Expect(store.ContentExists(catalog)).To(BeFalse())
			})
		})
	})
})

var _ = Describe("LocalDir Server Handler tests", func() {
	var (
		testServer *httptest.Server
		store      LocalDir
	)
	BeforeEach(func() {
		d, err := os.MkdirTemp(GinkgoT().TempDir(), "cache")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.Mkdir(filepath.Join(d, "test-catalog"), 0700)).To(Succeed())
		store = LocalDir{RootDir: d, BaseURL: &url.URL{Path: urlPrefix}}
		testServer = httptest.NewServer(store.StorageServerHandler())

	})
	It("gets 404 for the path /", func() {
		expectNotFound(testServer.URL)
	})
	It("gets 404 for the path /catalogs/", func() {
		expectNotFound(fmt.Sprintf("%s/%s", testServer.URL, "/catalogs/"))
	})
	It("gets 404 for the path /catalogs/test-catalog/", func() {
		expectNotFound(fmt.Sprintf("%s/%s", testServer.URL, "/catalogs/test-catalog/"))
	})
	It("gets 404 for the path /test-catalog/foo.txt", func() {
		// This ensures that even if the file exists, the URL must contain the /catalogs/ prefix
		Expect(os.WriteFile(filepath.Join(store.RootDir, "test-catalog", "foo.txt"), []byte("bar"), 0600)).To(Succeed())
		expectNotFound(fmt.Sprintf("%s/%s", testServer.URL, "/test-catalog/foo.txt"))
	})
	It("gets 404 for the path /catalogs/test-catalog/non-existent.txt", func() {
		expectNotFound(fmt.Sprintf("%s/%s", testServer.URL, "/catalogs/test-catalog/non-existent.txt"))
	})
	It("gets 200 for the path /catalogs/foo.txt", func() {
		expectedContent := []byte("bar")
		Expect(os.WriteFile(filepath.Join(store.RootDir, "foo.txt"), expectedContent, 0600)).To(Succeed())
		expectFound(fmt.Sprintf("%s/%s", testServer.URL, "/catalogs/foo.txt"), expectedContent)
	})
	It("gets 200 for the path /catalogs/test-catalog/foo.txt", func() {
		expectedContent := []byte("bar")
		Expect(os.WriteFile(filepath.Join(store.RootDir, "test-catalog", "foo.txt"), expectedContent, 0600)).To(Succeed())
		expectFound(fmt.Sprintf("%s/%s", testServer.URL, "/catalogs/test-catalog/foo.txt"), expectedContent)
	})
	It("ignores accept-encoding for the path /catalogs/test-catalog/all.json with size < 1400 bytes", func() {
		expectedContent := []byte("bar")
		Expect(os.WriteFile(filepath.Join(store.RootDir, "test-catalog", "all.json"), expectedContent, 0600)).To(Succeed())
		expectFound(fmt.Sprintf("%s/%s", testServer.URL, "/catalogs/test-catalog/all.json"), expectedContent)
	})
	It("provides gzipped content for the path /catalogs/test-catalog/all.json with size > 1400 bytes", func() {
		expectedContent := []byte(testCompressableJSON)
		Expect(os.WriteFile(filepath.Join(store.RootDir, "test-catalog", "all.json"), expectedContent, 0600)).To(Succeed())
		expectFound(fmt.Sprintf("%s/%s", testServer.URL, "/catalogs/test-catalog/all.json"), expectedContent)
	})
	It("provides json-lines format for the served JSON catalog", func() {
		catalog := "test-catalog"
		unpackResultFS := &fstest.MapFS{
			"catalog.json": &fstest.MapFile{Data: []byte(testCompressableJSON), Mode: os.ModePerm},
		}
		err := store.Store(context.Background(), catalog, unpackResultFS)
		Expect(err).To(Not(HaveOccurred()))

		expectedContent := []byte(jsonLinesFormattedCatalogData)
		expectFound(fmt.Sprintf("%s/%s", testServer.URL, "/catalogs/test-catalog/all.json"), expectedContent)
	})
	It("provides json-lines format for the served YAML catalog", func() {
		catalog := "test-catalog"
		unpackResultFS := &fstest.MapFS{
			"catalog.yaml": &fstest.MapFile{Data: []byte(testCompressableYAML), Mode: os.ModePerm},
		}
		err := store.Store(context.Background(), catalog, unpackResultFS)
		Expect(err).To(Not(HaveOccurred()))

		expectedContent := []byte(jsonLinesFormattedCatalogData)
		expectFound(fmt.Sprintf("%s/%s", testServer.URL, "/catalogs/test-catalog/all.json"), expectedContent)
	})
	AfterEach(func() {
		testServer.Close()
	})
})

func expectNotFound(url string) {
	resp, err := http.Get(url) //nolint:gosec
	Expect(err).To(Not(HaveOccurred()))
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	Expect(resp.Body.Close()).To(Succeed())
}

func expectFound(url string, expectedContent []byte) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	Expect(err).To(Not(HaveOccurred()))
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	Expect(err).To(Not(HaveOccurred()))
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	var actualContent []byte
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		Expect(len(expectedContent)).To(BeNumerically(">", 1400))
		gz, err := gzip.NewReader(resp.Body)
		Expect(err).To(Not(HaveOccurred()))
		actualContent, err = io.ReadAll(gz)
		Expect(err).To(Not(HaveOccurred()))
	default:
		Expect(len(expectedContent)).To(BeNumerically("<", 1400))
		actualContent, err = io.ReadAll(resp.Body)
		Expect(err).To(Not(HaveOccurred()))
	}

	Expect(actualContent).To(Equal(expectedContent))
	Expect(resp.Body.Close()).To(Succeed())
}

const testBundleTemplate = `---
image: %s
name: %s
schema: olm.bundle
package: %s
relatedImages:
  - name: %s
    image: %s
properties:
  - type: olm.bundle.object
    value:
      data: %s
  - type: some.other
    value:
      data: arbitrary-info
`

const testPackageTemplate = `---
defaultChannel: %s
name: %s
schema: olm.package
`

const testChannelTemplate = `---
schema: olm.channel
package: %s
name: %s
entries:
  - name: %s
`

// by default the compressor will only trigger for files larger than 1400 bytes
const testCompressableJSON = `{
  "defaultChannel": "stable-v6.x",
  "name": "cockroachdb",
  "schema": "olm.package"
}
{
  "entries": [
    {
      "name": "cockroachdb.v5.0.3"
    },
    {
      "name": "cockroachdb.v5.0.4",
      "replaces": "cockroachdb.v5.0.3"
    }
  ],
  "name": "stable-5.x",
  "package": "cockroachdb",
  "schema": "olm.channel"
}
{
  "entries": [
    {
      "name": "cockroachdb.v6.0.0",
      "skipRange": "<6.0.0"
    }
  ],
  "name": "stable-v6.x",
  "package": "cockroachdb",
  "schema": "olm.channel"
}
{
  "image": "quay.io/openshift-community-operators/cockroachdb@sha256:a5d4f4467250074216eb1ba1c36e06a3ab797d81c431427fc2aca97ecaf4e9d8",
  "name": "cockroachdb.v5.0.3",
  "package": "cockroachdb",
  "properties": [
    {
      "type": "olm.gvk",
      "value": {
        "group": "charts.operatorhub.io",
        "kind": "Cockroachdb",
        "version": "v1alpha1"
      }
    },
    {
      "type": "olm.package",
      "value": {
        "packageName": "cockroachdb",
        "version": "5.0.3"
      }
    }
  ],
  "relatedImages": [
    {
      "name": "",
      "image": "gcr.io/kubebuilder/kube-rbac-proxy:v0.5.0"
    },
    {
      "name": "",
      "image": "quay.io/helmoperators/cockroachdb:v5.0.3"
    },
    {
      "name": "",
      "image": "quay.io/openshift-community-operators/cockroachdb@sha256:a5d4f4467250074216eb1ba1c36e06a3ab797d81c431427fc2aca97ecaf4e9d8"
    }
  ],
  "schema": "olm.bundle"
}
{
  "image": "quay.io/openshift-community-operators/cockroachdb@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63",
  "name": "cockroachdb.v5.0.4",
  "package": "cockroachdb",
  "properties": [
    {
      "type": "olm.gvk",
      "value": {
        "group": "charts.operatorhub.io",
        "kind": "Cockroachdb",
        "version": "v1alpha1"
      }
    },
    {
      "type": "olm.package",
      "value": {
        "packageName": "cockroachdb",
        "version": "5.0.4"
      }
    }
  ],
  "relatedImages": [
    {
      "name": "",
      "image": "gcr.io/kubebuilder/kube-rbac-proxy:v0.5.0"
    },
    {
      "name": "",
      "image": "quay.io/helmoperators/cockroachdb:v5.0.4"
    },
    {
      "name": "",
      "image": "quay.io/openshift-community-operators/cockroachdb@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63"
    }
  ],
  "schema": "olm.bundle"
}
{
  "image": "quay.io/openshift-community-operators/cockroachdb@sha256:d3016b1507515fc7712f9c47fd9082baf9ccb070aaab58ed0ef6e5abdedde8ba",
  "name": "cockroachdb.v6.0.0",
  "package": "cockroachdb",
  "properties": [
    {
      "type": "olm.gvk",
      "value": {
        "group": "charts.operatorhub.io",
        "kind": "Cockroachdb",
        "version": "v1alpha1"
      }
    },
    {
      "type": "olm.package",
      "value": {
        "packageName": "cockroachdb",
        "version": "6.0.0"
      }
    }
  ],
  "relatedImages": [
    {
      "name": "",
      "image": "gcr.io/kubebuilder/kube-rbac-proxy:v0.5.0"
    },
    {
      "name": "",
      "image": "quay.io/cockroachdb/cockroach-helm-operator:6.0.0"
    },
    {
      "name": "",
      "image": "quay.io/openshift-community-operators/cockroachdb@sha256:d3016b1507515fc7712f9c47fd9082baf9ccb070aaab58ed0ef6e5abdedde8ba"
    }
  ],
  "schema": "olm.bundle"
}
`

const testCompressableYAML = `---
defaultChannel: stable-v6.x
name: cockroachdb
schema: olm.package
---
entries:
  - name: cockroachdb.v5.0.3
  - name: cockroachdb.v5.0.4
    replaces: cockroachdb.v5.0.3
name: stable-5.x
package: cockroachdb
schema: olm.channel
---
entries:
  - name: cockroachdb.v6.0.0
    skipRange: <6.0.0
name: stable-v6.x
package: cockroachdb
schema: olm.channel
---
image: quay.io/openshift-community-operators/cockroachdb@sha256:a5d4f4467250074216eb1ba1c36e06a3ab797d81c431427fc2aca97ecaf4e9d8
name: cockroachdb.v5.0.3
package: cockroachdb
properties:
  - type: olm.gvk
    value:
      group: charts.operatorhub.io
      kind: Cockroachdb
      version: v1alpha1
  - type: olm.package
    value:
      packageName: cockroachdb
      version: 5.0.3
relatedImages:
  - name: ""
    image: gcr.io/kubebuilder/kube-rbac-proxy:v0.5.0
  - name: ""
    image: quay.io/helmoperators/cockroachdb:v5.0.3
  - name: ""
    image: quay.io/openshift-community-operators/cockroachdb@sha256:a5d4f4467250074216eb1ba1c36e06a3ab797d81c431427fc2aca97ecaf4e9d8
schema: olm.bundle
---
image: quay.io/openshift-community-operators/cockroachdb@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63
name: cockroachdb.v5.0.4
package: cockroachdb
properties:
  - type: olm.gvk
    value:
      group: charts.operatorhub.io
      kind: Cockroachdb
      version: v1alpha1
  - type: olm.package
    value:
      packageName: cockroachdb
      version: 5.0.4
relatedImages:
  - name: ""
    image: gcr.io/kubebuilder/kube-rbac-proxy:v0.5.0
  - name: ""
    image: quay.io/helmoperators/cockroachdb:v5.0.4
  - name: ""
    image: quay.io/openshift-community-operators/cockroachdb@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63
schema: olm.bundle
---
image: quay.io/openshift-community-operators/cockroachdb@sha256:d3016b1507515fc7712f9c47fd9082baf9ccb070aaab58ed0ef6e5abdedde8ba
name: cockroachdb.v6.0.0
package: cockroachdb
properties:
  - type: olm.gvk
    value:
      group: charts.operatorhub.io
      kind: Cockroachdb
      version: v1alpha1
  - type: olm.package
    value:
      packageName: cockroachdb
      version: 6.0.0
relatedImages:
  - name: ""
    image: gcr.io/kubebuilder/kube-rbac-proxy:v0.5.0
  - name: ""
    image: quay.io/cockroachdb/cockroach-helm-operator:6.0.0
  - name: ""
    image: quay.io/openshift-community-operators/cockroachdb@sha256:d3016b1507515fc7712f9c47fd9082baf9ccb070aaab58ed0ef6e5abdedde8ba
schema: olm.bundle
  `

// the JSONlines-compliant format for testCompressableJSON/testCompressableYAML.
// Transforms to that format are as follows:
// 1. Remove all newlines
// 2. Remove all spaces
// 3. Add a newline after each complete JSON object
//
// the last of these prevents just doing
// var jsonLinesFormattedCatalogData string = strings.ReplaceAll(strings.ReplaceAll(testCompressableJSON, "\n", ""), " ", "")
//
// while the source format (expanded JSON) is resilient to extraneous whitespace or EOL commas for terminating entries, this format is not
//
// NB: Please update this to align with any changes to testCompressableJSON/testCompressableYAML.
const jsonLinesFormattedCatalogData = `{"defaultChannel":"stable-v6.x","name":"cockroachdb","schema":"olm.package"}
{"entries":[{"name":"cockroachdb.v5.0.3"},{"name":"cockroachdb.v5.0.4","replaces":"cockroachdb.v5.0.3"}],"name":"stable-5.x","package":"cockroachdb","schema":"olm.channel"}
{"entries":[{"name":"cockroachdb.v6.0.0","skipRange":"<6.0.0"}],"name":"stable-v6.x","package":"cockroachdb","schema":"olm.channel"}
{"image":"quay.io/openshift-community-operators/cockroachdb@sha256:a5d4f4467250074216eb1ba1c36e06a3ab797d81c431427fc2aca97ecaf4e9d8","name":"cockroachdb.v5.0.3","package":"cockroachdb","properties":[{"type":"olm.gvk","value":{"group":"charts.operatorhub.io","kind":"Cockroachdb","version":"v1alpha1"}},{"type":"olm.package","value":{"packageName":"cockroachdb","version":"5.0.3"}}],"relatedImages":[{"image":"gcr.io/kubebuilder/kube-rbac-proxy:v0.5.0","name":""},{"image":"quay.io/helmoperators/cockroachdb:v5.0.3","name":""},{"image":"quay.io/openshift-community-operators/cockroachdb@sha256:a5d4f4467250074216eb1ba1c36e06a3ab797d81c431427fc2aca97ecaf4e9d8","name":""}],"schema":"olm.bundle"}
{"image":"quay.io/openshift-community-operators/cockroachdb@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63","name":"cockroachdb.v5.0.4","package":"cockroachdb","properties":[{"type":"olm.gvk","value":{"group":"charts.operatorhub.io","kind":"Cockroachdb","version":"v1alpha1"}},{"type":"olm.package","value":{"packageName":"cockroachdb","version":"5.0.4"}}],"relatedImages":[{"image":"gcr.io/kubebuilder/kube-rbac-proxy:v0.5.0","name":""},{"image":"quay.io/helmoperators/cockroachdb:v5.0.4","name":""},{"image":"quay.io/openshift-community-operators/cockroachdb@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63","name":""}],"schema":"olm.bundle"}
{"image":"quay.io/openshift-community-operators/cockroachdb@sha256:d3016b1507515fc7712f9c47fd9082baf9ccb070aaab58ed0ef6e5abdedde8ba","name":"cockroachdb.v6.0.0","package":"cockroachdb","properties":[{"type":"olm.gvk","value":{"group":"charts.operatorhub.io","kind":"Cockroachdb","version":"v1alpha1"}},{"type":"olm.package","value":{"packageName":"cockroachdb","version":"6.0.0"}}],"relatedImages":[{"image":"gcr.io/kubebuilder/kube-rbac-proxy:v0.5.0","name":""},{"image":"quay.io/cockroachdb/cockroach-helm-operator:6.0.0","name":""},{"image":"quay.io/openshift-community-operators/cockroachdb@sha256:d3016b1507515fc7712f9c47fd9082baf9ccb070aaab58ed0ef6e5abdedde8ba","name":""}],"schema":"olm.bundle"}
`
