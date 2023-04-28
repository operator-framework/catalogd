package source

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/nlepage/go-tarfs"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	catalogdv1beta1 "github.com/operator-framework/catalogd/pkg/apis/core/v1beta1"
)

// http is a catalog source that sources catalogs from the specified url.
type HTTP struct {
	client.Reader
	SecretNamespace string
}

// Unpack unpacks a catalog by requesting the catalog contents from a specified URL
func (b *HTTP) Unpack(ctx context.Context, catalog *catalogdv1beta1.CatalogSource) (*Result, error) {
	if catalog.Spec.Source.Type != catalogdv1beta1.SourceTypeHTTP {
		return nil, fmt.Errorf("cannot unpack source type %q with %q unpacker", catalog.Spec.Source.Type, catalogdv1beta1.SourceTypeHTTP)
	}

	url := catalog.Spec.Source.HTTP.URL
	action := fmt.Sprintf("%s %s", http.MethodGet, url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create http request %q for catalog content: %v", action, err)
	}
	var userName, password string
	if catalog.Spec.Source.HTTP.Auth.Secret.Name != "" {
		userName, password, err = b.getCredentials(ctx, catalog)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(userName, password)
	}

	httpClient := http.Client{Timeout: 10 * time.Second}
	if catalog.Spec.Source.HTTP.Auth.InsecureSkipVerify {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // nolint:gosec
		httpClient.Transport = tr
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: http request for catalog content failed: %v", action, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: unexpected status %q", action, resp.Status)
	}

	tarReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	fs, err := tarfs.New(tarReader)
	if err != nil {
		return nil, fmt.Errorf("error creating FS: %s", err)
	}

	message := generateMessage("http")

	return &Result{FS: fs, ResolvedSource: catalog.Spec.Source.DeepCopy(), State: StateUnpacked, Message: message}, nil
}

// getCredentials reads credentials from the secret specified in the catalog
// It returns the username ane password when they are in the secret
func (b *HTTP) getCredentials(ctx context.Context, catalog *catalogdv1beta1.CatalogSource) (string, string, error) {
	secret := &corev1.Secret{}
	err := b.Get(ctx, client.ObjectKey{Namespace: b.SecretNamespace, Name: catalog.Spec.Source.HTTP.Auth.Secret.Name}, secret)
	if err != nil {
		return "", "", err
	}
	userName := string(secret.Data["username"])
	password := string(secret.Data["password"])

	return userName, password, nil
}
