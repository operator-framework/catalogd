package features

import (
	"fmt"

	"k8s.io/component-base/featuregate"
)

const (
	// Add new feature gates constants (strings)
	// Ex: SomeFeature featuregate.Feature = "SomeFeature"

	OCIArtifactSource featuregate.Feature = "OCIArtifactSource"
)

var catalogdFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	// Add new feature gate definitions
	// Ex: SomeFeature: {...}
	OCIArtifactSource: {Default: false, PreRelease: featuregate.Alpha},
}

var CatalogdFeatureGate featuregate.MutableFeatureGate = featuregate.NewFeatureGate()

func init() {
	CatalogdFeatureGate.Add(catalogdFeatureGates)
}

type ErrNotEnabled struct {
	Feature featuregate.Feature
}

func (e ErrNotEnabled) Error() string {
	return fmt.Sprintf("feature %q is not enabled", e.Feature)
}

func NotEnabledError(feature featuregate.Feature) error {
	return ErrNotEnabled{Feature: feature}
}
