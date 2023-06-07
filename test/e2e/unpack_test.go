package e2e

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	catalogd "github.com/operator-framework/catalogd/pkg/apis/core/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	testCatalogRef  = "localhost/testdata/catalogs/test-catalog:e2e"
	testCatalogName = "test-catalog"
	pkg             = "prometheus"
	version         = "0.47.0"
	channel         = "beta"
	bundle          = "prometheus-operator.0.47.0"
	bundleImage     = "localhost/testdata/bundles/registry-v1/prometheus-operator:v0.47.0"
)

var _ = Describe("Catalog Unpacking", func() {
	var (
		ctx     context.Context
		catalog *catalogd.Catalog
	)
	When("A Catalog is created", func() {
		BeforeEach(func() {
			ctx = context.Background()
			var err error
			catalog, err = createTestCatalog(ctx, testCatalogName, testCatalogRef)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Successfully unpacks catalog contents", func() {
			By("Ensuring Catalog has Status.Condition of Unpacked with a status == True")
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{Name: catalog.Name}, catalog)
				g.Expect(err).ToNot(HaveOccurred())
				cond := meta.FindStatusCondition(catalog.Status.Conditions, catalogd.TypeUnpacked)
				g.Expect(cond).ToNot(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(Equal(catalogd.ReasonUnpackSuccessful))
				g.Expect(cond.Message).To(ContainSubstring("successfully unpacked the catalog image"))
			}).Should(Succeed())

			By("Ensuring the expected Package resource is created")
			pack := &catalogd.Package{}
			err := c.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-%s", catalog.Name, pkg)}, pack)
			Expect(err).ToNot(HaveOccurred())
			Expect(pack.Spec.Catalog.Name).To(Equal(catalog.Name))
			Expect(pack.Spec.Channels).To(HaveLen(1))
			Expect(pack.Spec.Channels[0].Name).To(Equal(channel))
			Expect(pack.Spec.Channels[0].Entries).To(HaveLen(1))
			Expect(pack.Spec.Channels[0].Entries[0].Name).To(Equal(bundle))
			Expect(pack.Spec.DefaultChannel).To(Equal(channel))
			Expect(pack.Spec.Name).To(Equal(pkg))

			By("Ensuring the expected BundleMetadata resource is created")
			bm := &catalogd.BundleMetadata{}
			err = c.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-%s", catalog.Name, bundle)}, bm)
			Expect(err).ToNot(HaveOccurred())
			Expect(bm.Spec.Catalog.Name).To(Equal(catalog.Name))
			Expect(bm.Spec.Package).To(Equal(pkg))
			Expect(bm.Spec.Image).To(Equal(bundleImage))
			Expect(bm.Spec.Properties).To(HaveLen(1))
			Expect(bm.Spec.Properties[0].Type).To(Equal("olm.package"))
			Expect(bm.Spec.Properties[0].Value).To(Equal(json.RawMessage(`{"packageName":"prometheus","version":"0.47.0"}`)))
		})

		AfterEach(func() {
			err := c.Delete(ctx, catalog)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func(g Gomega) {
				err = c.Get(ctx, types.NamespacedName{Name: catalog.Name}, &catalogd.Catalog{})
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})
	})
})

// createTestCatalog will create a new catalog on the test cluster, provided
// the context, catalog name, and the image reference. It returns the created catalog
// or an error if any errors occurred while creating the catalog.
func createTestCatalog(ctx context.Context, name string, imageRef string) (*catalogd.Catalog, error) {
	catalog := &catalogd.Catalog{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: catalogd.CatalogSpec{
			Source: catalogd.CatalogSource{
				Type: catalogd.SourceTypeImage,
				Image: &catalogd.ImageSource{
					Ref: imageRef,
				},
			},
		},
	}

	err := c.Create(ctx, catalog)
	return catalog, err
}
