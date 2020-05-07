package controllerutil_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/joelanford/helm-operator/pkg/internal/sdk/controllerutil"
)

var _ = Describe("Controllerutil", func() {
	Describe("WaitForDeletion", func() {
		var (
			ctx    context.Context
			cancel context.CancelFunc
			pod    *v1.Pod
			client client.Client
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			pod = &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testName",
					Namespace: "testNamespace",
				},
			}
			client = fake.NewFakeClientWithScheme(scheme.Scheme, pod)
		})

		AfterEach(func() {
			cancel()
		})

		It("should be cancellable", func() {
			cancel()
			Expect(WaitForDeletion(ctx, client, pod)).To(MatchError(wait.ErrWaitTimeout))
		})

		It("should succeed after pod is deleted", func() {
			Expect(client.Delete(ctx, pod)).To(Succeed())
			Expect(WaitForDeletion(ctx, client, pod)).To(Succeed())
		})
	})

	Describe("SupportsOwnerReference", func() {
		var (
			rm              *meta.DefaultRESTMapper
			owner           runtime.Object
			dependent       runtime.Object
			clusterScoped   = schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "ClusterScoped"}
			namespaceScoped = schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "NamespaceScoped"}
		)
		When("GVK REST mappings exist", func() {
			BeforeEach(func() {
				rm = meta.NewDefaultRESTMapper([]schema.GroupVersion{})
				rm.Add(clusterScoped, meta.RESTScopeRoot)
				rm.Add(namespaceScoped, meta.RESTScopeNamespace)
			})
			When("owner is cluster scoped", func() {
				BeforeEach(func() {
					owner = createObject(clusterScoped, types.NamespacedName{Namespace: "", Name: "owner"})
				})
				It("should be true for cluster-scoped dependents", func() {
					dependent = createObject(clusterScoped, types.NamespacedName{Namespace: "", Name: "dependent"})
					supportsOwnerRef, err := SupportsOwnerReference(rm, owner, dependent)
					Expect(supportsOwnerRef).To(BeTrue())
					Expect(err).To(BeNil())
				})
				It("should be true for namespace-scoped dependents", func() {
					dependent = createObject(namespaceScoped, types.NamespacedName{Namespace: "ns1", Name: "dependent"})
					supportsOwnerRef, err := SupportsOwnerReference(rm, owner, dependent)
					Expect(supportsOwnerRef).To(BeTrue())
					Expect(err).To(BeNil())
				})
			})
			When("owner is namespace scoped", func() {
				BeforeEach(func() {
					owner = createObject(namespaceScoped, types.NamespacedName{Namespace: "ns1", Name: "owner"})
				})
				It("should be false for cluster-scoped dependents", func() {
					dependent = createObject(clusterScoped, types.NamespacedName{Namespace: "", Name: "dependent"})
					supportsOwnerRef, err := SupportsOwnerReference(rm, owner, dependent)
					Expect(supportsOwnerRef).To(BeFalse())
					Expect(err).To(BeNil())
				})
				When("dependent is in owner namespace", func() {
					It("should be true", func() {
						dependent = createObject(namespaceScoped, types.NamespacedName{Namespace: "ns1", Name: "dependent"})
						supportsOwnerRef, err := SupportsOwnerReference(rm, owner, dependent)
						Expect(supportsOwnerRef).To(BeTrue())
						Expect(err).To(BeNil())
					})
				})
				When("dependent is not in owner namespace", func() {
					It("should be false", func() {
						dependent = createObject(namespaceScoped, types.NamespacedName{Namespace: "ns2", Name: "dependent"})
						supportsOwnerRef, err := SupportsOwnerReference(rm, owner, dependent)
						Expect(supportsOwnerRef).To(BeFalse())
						Expect(err).To(BeNil())
					})
				})
			})
			When("Objects do not have valid type metadata", func() {

				var (
					validOwner   = createObject(clusterScoped, types.NamespacedName{Namespace: "", Name: "owner"})
					invalidOwner = &object{TypeMeta: metav1.TypeMeta{
						APIVersion: clusterScoped.GroupVersion().String(),
						Kind:       clusterScoped.Kind,
					}}
					invalidDepedent = &object{TypeMeta: metav1.TypeMeta{
						APIVersion: namespaceScoped.GroupVersion().String(),
						Kind:       namespaceScoped.Kind,
					}}
				)
				It("should fail when owner is invalid", func() {
					supportsOwnerRef, err := SupportsOwnerReference(rm, invalidOwner, invalidDepedent)
					Expect(supportsOwnerRef).To(BeFalse())
					Expect(err).NotTo(BeNil())
				})
				It("should fail when dependent is invalid", func() {
					supportsOwnerRef, err := SupportsOwnerReference(rm, validOwner, invalidDepedent)
					Expect(supportsOwnerRef).To(BeFalse())
					Expect(err).NotTo(BeNil())
				})
			})
		})
		When("GVK REST mappings are missing", func() {
			var (
				owner     = createObject(clusterScoped, types.NamespacedName{Namespace: "", Name: "owner"})
				dependent = createObject(namespaceScoped, types.NamespacedName{Namespace: "ns1", Name: "dependent"})
			)

			BeforeEach(func() {
				rm = meta.NewDefaultRESTMapper([]schema.GroupVersion{})
			})
			It("fails when owner REST mapping is missing", func() {
				supportsOwnerRef, err := SupportsOwnerReference(rm, owner, dependent)
				Expect(supportsOwnerRef).To(BeFalse())
				Expect(err).NotTo(BeNil())
			})
			It("fails when dependent REST mapping is missing", func() {
				rm.Add(clusterScoped, meta.RESTScopeRoot)
				supportsOwnerRef, err := SupportsOwnerReference(rm, owner, dependent)
				Expect(supportsOwnerRef).To(BeFalse())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("ContainsFinalizer", func() {
		var (
			obj       metav1.Object
			gvk       = schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Kind"}
			finalizer = "finalizer"
		)
		BeforeEach(func() {
			obj = createObject(gvk, types.NamespacedName{Namespace: "ns1", Name: "myKind"})
		})
		When("object contains finalizer", func() {
			BeforeEach(func() {
				obj.SetFinalizers([]string{finalizer})
			})
			It("should return true", func() {
				Expect(ContainsFinalizer(obj, finalizer)).To(BeTrue())
			})
		})
		When("object contains finalizer", func() {
			It("should return true", func() {
				Expect(ContainsFinalizer(obj, finalizer)).To(BeFalse())
			})
		})
	})
})

func createObject(gvk schema.GroupVersionKind, key types.NamespacedName) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName(key.Name)
	u.SetNamespace(key.Namespace)
	return u
}

type object struct {
	metav1.TypeMeta
}

func (o *object) DeepCopyObject() runtime.Object { return nil }
