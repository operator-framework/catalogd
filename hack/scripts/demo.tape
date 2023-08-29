
# the .tape file used by charmbracelet/vhs to generate the demo GIF

Output ./docs/demo.gif

Set TypingSpeed 75ms
Set FontSize 22
Set Width 1300
Set Height 650

Type "# Welcome to the catalogd demo"
Enter

#Type "git clone $REPO"
#Enter
#Type "cd catalogd"
#Enter

Hide
Type "kind delete clusters"
Show

Type "make kind-cluster"
Enter

Type "kubectl cluster-info --context kind-catalogd"
Enter

Sleep 10

Type "make install"
Enter

Sleep 10

# inspect crds (catalog, catalogmetadata)
Type 'kubectl get crds -A'
Enter

Type "# create a catalog"
Enter

Type "kubectl apply -f config/samples/core_v1alpha1_catalog.yaml" # or other
Enter

Type "kubectl get catalog -A" # shows catalog-sample
Enter

Type "# waiting for catalog to report ready status"
Enter

Type "kubectl wait --for=condition=Unpacked catalog/operatorhubio --timeout=1h"
Enter

# inspect packages, and then details on one package
Type "# check what 'packages' are available in this catalog and then inspect the content of one of the packages"
Enter

Type "kubectl get catalogmetadata -l schema=olm.package"
Enter

Type "kubectl get catalogmetadata operatorhubio-olm.package-wavefront -o yaml"
Enter

# inspect channels, and then details on one channel
Type "# check what channels are included in the wavefront package and then inspect the content of the alpha channel"
Enter

Type "kubectl get catalogmetadata -l schema=olm.channel,package=wavefront"
Enter

Type "kubectl get catalogmetadata operatorhubio-olm.channel-wavefront-alpha -o yaml"
Enter

# inspect bundles, and then details on one bundle
Type "# check what bundles are included in the wavefront package and then inspect the content of the wavefront-operator.v0.1.0 bundle"
Enter

Type "kubectl get catalogmetadata -l schema=olm.bundle,package=wavefront"
Enter

Type "kubectl get catalogmetadata operatorhubio-olm.bundle-wavefront-wavefront-operator.v0.1.0 -o yaml"
Enter

