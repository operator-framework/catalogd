package catalogserver

import (
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"

	"github.com/operator-framework/catalogd/api/optional/v1alpha1"
)

const defaultEtcdPathPrefix = "/storage/optional.catalogd.operator-framework.info"

type ServerOptions struct {
	RecommendedOptions *genericoptions.RecommendedOptions
}

func NewServerOptions() *ServerOptions {
	o := &ServerOptions{
		RecommendedOptions: genericoptions.NewRecommendedOptions(
			defaultEtcdPathPrefix,
			Codecs.LegacyCodec(v1alpha1.GroupVersion),
		),
	}
	return o
}

func (o ServerOptions) Validate() error {
	errors := []error{}
	errors = append(errors, o.RecommendedOptions.Validate()...)
	return utilerrors.NewAggregate(errors)
}

func (o *ServerOptions) Complete() error {
	return nil
}

func (o *ServerOptions) ConfigWithOptions() (*Config, error) {
	serverConfig := genericapiserver.NewRecommendedConfig(Codecs)
	if err := o.RecommendedOptions.ApplyTo(serverConfig); err != nil {
		return nil, err
	}

	config := &Config{
		GenericConfig: serverConfig,
	}
	return config, nil
}

func (o ServerOptions) RunServerWithOptions(stopCh <-chan struct{}) error {
	config, err := o.ConfigWithOptions()
	if err != nil {
		return err
	}

	server, err := config.Complete().NewServerWithCompletedConfig()
	if err != nil {
		return err
	}

	return server.GenericAPIServer.PrepareRun().Run(stopCh)
}
