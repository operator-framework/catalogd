package httpclient

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

const OnClusterBaseURL = "http://catalogd-catalogserver.catalogd-system.svc"
const catalogsEndpoint = "catalogs"

// FilterFunc is used to filter declcfg.Meta objects returned
// from a CatalogServerClient.GetCatalogContents call.
// A return value of "true" means it should be included in the
// response.
type FilterFunc func(meta *declcfg.Meta) bool

// CatalogServerClient is an interface that can be used for
// fetching catalog contents from the catalogd HTTP server
type CatalogServerClient interface {
	// GetCatalogContents fetches contents for a provided Catalog name
	// from the catalogd HTTP server that serves catalog contents and returns
	// the results as a slice of *declcfg.Meta. Filters can be applied to filter
	// the results that are returned. If any of the filters return a "false" value
	// indicating the *declcfg.Meta object will not be included in the returned slice.
	// An error will be returned if any occur.
	GetCatalogContents(catalogName string, filters ...FilterFunc) ([]*declcfg.Meta, error)
}

type ClientOpts func(c *client)

// WithBaseURL is an option function that
// sets the base url that is used when
// making a request. If this option is not used
// it defaults to OnClusterBaseURL
func WithBaseURL(base string) ClientOpts {
	return func(c *client) {
		c.BaseURL = base
	}
}

// NewClient returns a new CatalogServerClient configured
// with the provided options (if any provided)
func NewClient(opts ...ClientOpts) CatalogServerClient {
	cli := &client{
		BaseURL: OnClusterBaseURL,
	}

	for _, opt := range opts {
		opt(cli)
	}

	return cli
}

// client is an implementation of the CatalogServerClient interface
type client struct {
	BaseURL string
}

func (c *client) GetCatalogContents(catalogName string, filters ...FilterFunc) ([]*declcfg.Meta, error) {
	catalogURL := strings.Join([]string{c.BaseURL, catalogsEndpoint, catalogName, "all.json"}, "/")

	//nolint:gosec
	resp, err := http.Get(catalogURL)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode > 299 {
		return nil, fmt.Errorf("error: response status code is %d with a response body of %s", resp.StatusCode, resp.Body)
	}

	blobs := []*declcfg.Meta{}
	err = declcfg.WalkMetasReader(resp.Body, func(meta *declcfg.Meta, err error) error {
		if err != nil {
			return fmt.Errorf("error parsing catalog content from response body: %w", err)
		}

		skip := false
		for _, filter := range filters {
			if !filter(meta) {
				skip = true
			}
		}

		if !skip {
			blobs = append(blobs, meta)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return blobs, nil
}
