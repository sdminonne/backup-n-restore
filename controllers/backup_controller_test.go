package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/open-cluster-management/backup-n-restore/api/v1alpha1"
	vapi "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Backup controller", func() {
	const (
		BackupName = "the-ocm-backup"

		VeleroNamespaceName = "velero"
		HiveNamespaceName   = "hive"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)
	var (
		ctx             context.Context
		managedClusters []clusterv1.ManagedCluster
		veleroNamespace *corev1.Namespace
		hiveNamespace   *corev1.Namespace
	)

	BeforeEach(func() {
		ctx = context.Background()
		managedClusters = []clusterv1.ManagedCluster{
			clusterv1.ManagedCluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cluster.open-cluster-management.io/v1",
					Kind:       "ManagedCluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
			},
			clusterv1.ManagedCluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cluster.open-cluster-management.io/v1",
					Kind:       "ManagedCluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "managed1",
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
			},
		}
		veleroNamespace = &corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: VeleroNamespaceName,
			},
		}
		hiveNamespace = &corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: HiveNamespaceName,
			},
		}
	})

	JustBeforeEach(func() {
		for i := range managedClusters {
			Expect(k8sClient.Create(ctx, &managedClusters[i])).Should(Succeed())
		}
		if veleroNamespace != nil {
			Expect(k8sClient.Create(ctx, veleroNamespace)).Should(Succeed())
		}
		if hiveNamespace != nil {
			Expect(k8sClient.Create(ctx, hiveNamespace)).Should(Succeed())
		}
	})

	Context("When creating a Backup", func() {
		It("Should creating a Velero Backup updating the Status", func() {

			managedClusterList := clusterv1.ManagedClusterList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, &managedClusterList, &client.ListOptions{})
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(len(managedClusterList.Items)).To(BeNumerically("==", 2))

			createdVeleroNamespace := corev1.Namespace{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: VeleroNamespaceName, Namespace: ""}, &createdVeleroNamespace); err != nil {
					return false
				}
				if createdVeleroNamespace.Status.Phase == "Active" {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			ocmBackup := v1alpha1.Backup{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "cluster.open-cluster-management.io/v1alpha1",
					Kind:       "Backup",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: BackupName,
				},
				Spec: v1alpha1.BackupSpec{
					VeleroConfig: &v1alpha1.VeleroConfigBackupProxy{
						Namespace: VeleroNamespaceName,
					},
				},
			}

			Expect(k8sClient.Create(ctx, &ocmBackup)).Should(Succeed())

			backupLookupKey := types.NamespacedName{Name: BackupName, Namespace: ""}
			createdBackup := v1alpha1.Backup{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, backupLookupKey, &createdBackup)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// createdBackup should be created
			Expect(createdBackup.CreationTimestamp.Time).NotTo(BeNil())

			//By("created backup should contain velero backup in status")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, backupLookupKey, &createdBackup)
				if err != nil {
					return false
				}
				return createdBackup.Status.VeleroBackup != nil
			}, timeout, interval).ShouldNot(BeNil())

			// Getting veleroBackup resource
			veleroBackup := vapi.Backup{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: createdBackup.Name, Namespace: VeleroNamespaceName}, &veleroBackup)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(len(veleroBackup.Spec.IncludedNamespaces)).To(BeNumerically("==", 5))

			Expect(veleroBackup.Status.Phase).To(BeEmpty())

			veleroBackup.Namespace = VeleroNamespaceName
			veleroBackup.Status.Phase = "InProgress"
			Expect(k8sClient.Update(ctx, &veleroBackup, &client.UpdateOptions{})).Should(Succeed())
			//Expect(k8sClient.Status().Update(ctx, &veleroBackup, &client.UpdateOptions{})).Should(Succeed())
			Eventually(func() bool {
				b := vapi.Backup{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: createdBackup.Name, Namespace: VeleroNamespaceName}, &b); err != nil {
					return false
				}
				return b.Status.Phase == "InProgress"
			}, timeout, interval).Should(BeTrue())

			veleroBackup.Status.Phase = "Completed"
			Expect(k8sClient.Update(ctx, &veleroBackup, &client.UpdateOptions{})).Should(Succeed())
			Eventually(func() bool {
				b := vapi.Backup{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: createdBackup.Name, Namespace: VeleroNamespaceName}, &b); err != nil {
					return false
				}
				return b.Status.Phase == "Completed"
			}, timeout, interval).Should(BeTrue())
		})
	})
})
