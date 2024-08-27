package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/operator-framework/catalogd/api/core/v1alpha1"
)

func TestClusterCatalogDefaulting(t *testing.T) {
	tests := map[string]struct {
		clusterCatalog *v1alpha1.ClusterCatalog
		expectedLabels map[string]string
	}{
		"no labels provided, name label added": {
			clusterCatalog: &v1alpha1.ClusterCatalog{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-catalog",
				},
			},
			expectedLabels: map[string]string{
				"name": "test-catalog",
			},
		},
		"labels already present, name label added": {
			clusterCatalog: &v1alpha1.ClusterCatalog{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-catalog",
					Labels: map[string]string{
						"existing": "label",
					},
				},
			},
			expectedLabels: map[string]string{
				"name":     "test-catalog",
				"existing": "label",
			},
		},
		"name label already present, no changes": {
			clusterCatalog: &v1alpha1.ClusterCatalog{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-catalog",
					Labels: map[string]string{
						"name": "existing-name",
					},
				},
			},
			expectedLabels: map[string]string{
				"name": "test-catalog", // Defaulting should still override this to match the object name
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Arrange
			clusterCatalogWrapper := &ClusterCatalog{}

			// Act
			err := clusterCatalogWrapper.Default(context.TODO(), tc.clusterCatalog)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedLabels, tc.clusterCatalog.Labels)
		})
	}
}
