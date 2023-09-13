package httpclient

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

var _ = Describe("HTTP Client Test", func() {
	const (
		testCatalogName = "httpclienttest"
	)

	Context("HTTP Server responds with 200 response code and a valid JSON stream", func() {
		var (
			srv *httptest.Server
			cli CatalogServerClient
		)
		BeforeEach(func() {
			mux := http.NewServeMux()
			mux.Handle(
				fmt.Sprintf("/catalogs/%s/all.json", testCatalogName),
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte(allJSON))
				},
				))
			srv = httptest.NewServer(mux)
			cli = NewClient(WithBaseURL(srv.URL))
		})
		AfterEach(func() {
			srv.Close()
		})

		It("Should return all contents", func() {
			expectedMetas := []*declcfg.Meta{}
			err := declcfg.WalkMetasReader(bytes.NewReader([]byte(allJSON)), func(meta *declcfg.Meta, err error) error {
				if err != nil {
					return err
				}
				expectedMetas = append(expectedMetas, meta)
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			metas, err := cli.GetCatalogContents(testCatalogName)
			Expect(err).NotTo(HaveOccurred())
			Expect(metas).To(Equal(expectedMetas))
		})

		It("Should return only blobs that have schema == \"olm.package\"", func() {
			expectedMetas := []*declcfg.Meta{}
			err := declcfg.WalkMetasReader(bytes.NewReader([]byte(allJSON)), func(meta *declcfg.Meta, err error) error {
				if err != nil {
					return err
				}
				if meta.Schema == "olm.package" {
					expectedMetas = append(expectedMetas, meta)
				}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			metas, err := cli.GetCatalogContents(testCatalogName, func(meta *declcfg.Meta) bool {
				return meta.Schema == "olm.package"
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(metas).To(Equal(expectedMetas))
		})
	})

	Context("HTTP Server responds with a 200 response code and an invalid JSON stream", func() {
		var (
			srv *httptest.Server
			cli CatalogServerClient
		)
		BeforeEach(func() {
			mux := http.NewServeMux()
			mux.Handle(
				fmt.Sprintf("/catalogs/%s/all.json", testCatalogName),
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte(invalidJSON))
				},
				))
			srv = httptest.NewServer(mux)
			cli = NewClient(WithBaseURL(srv.URL))
		})
		AfterEach(func() {
			srv.Close()
		})

		It("Should return an error", func() {
			metas, err := cli.GetCatalogContents(testCatalogName)
			Expect(err).To(HaveOccurred())
			Expect(metas).To(BeNil())
		})
	})

	Context("HTTP Server responds with a non-2xx response code", func() {
		var (
			srv *httptest.Server
			cli CatalogServerClient
		)
		BeforeEach(func() {
			mux := http.NewServeMux()
			mux.Handle(
				fmt.Sprintf("/catalogs/%s/all.json", testCatalogName),
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				},
				))
			srv = httptest.NewServer(mux)
			cli = NewClient(WithBaseURL(srv.URL))
		})
		AfterEach(func() {
			srv.Close()
		})

		It("Should return an error", func() {
			metas, err := cli.GetCatalogContents(testCatalogName)
			Expect(err).To(HaveOccurred())
			Expect(metas).To(BeNil())
		})
	})
})

const invalidJSON = `
{
	"schema": olm.package
	"name": bar
	defaultChannel: "stable"
}
`

const allJSON = `
{
	"schema": "olm.package",
	"name": "foo",
	"defaultChannel": "alpha"
}
{
	"schema": "olm.bundle",
	"name": "foo-bundle.v0.0.1",
	"image": "quay.io/foo-operator/foo-bundle:v.0.0.1",
	"package": "foo"
}
{
	"schema": "olm.channel",
	"package": "foo",
	"name": "alpha",
	"entries": [
		{
			"name": "foo-bundle.v0.0.1"
		}
	]
}
`
