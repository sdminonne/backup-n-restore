/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/open-cluster-management/backup-n-restore/api/v1alpha1"
	vapi "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

// BackupReconciler reconciles a Backup object
type BackupReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=backups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=backups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=backups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Backup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	backupLogger := log.FromContext(ctx)
	backup := v1alpha1.Backup{}
	if err := r.Get(ctx, req.NamespacedName, &backup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	switch {
	case isBackupNotYetStarted(&backup, r.Client):
		_, err := r.startBackup(ctx, &backup, r.Client)
		if err != nil {
			backupLogger.Error(err, "coulnd't start backup")
		}
		return ctrl.Result{}, err
	case isBackupFinished(&backup):
		backupLogger.Info("backup terminated")
		return ctrl.Result{}, nil
	default:
		backupLogger.Info("Backup started and NOT finished")
	}

	return ctrl.Result{}, nil
}

var (
	backupOwnerKey = ".metadata.controller"
	apiGV          = v1alpha1.GroupVersion.String()
)

// SetupWithManager sets up the controller with the Manager.
func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &vapi.Backup{}, backupOwnerKey, func(rawObj client.Object) []string {
		backup := rawObj.(*vapi.Backup)
		owner := metav1.GetControllerOf(backup)
		if owner == nil {
			return nil
		}
		// It must be an open-cluster-management.io Backup
		if owner.APIVersion != apiGV || owner.Kind != "Backup" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Backup{}).
		Complete(r)
}

func (r *BackupReconciler) startBackup(ctx context.Context, backup *v1alpha1.Backup, c client.Client) (*v1alpha1.Backup, error) {
	var (
		err          error
		veleroBackup *vapi.Backup
	)
	switch {
	case backup.Spec.VeleroConfig != nil:
		veleroBackup, err = r.createVeleroBackupFromBackup(ctx, backup, c)
		if err != nil {
			return nil, fmt.Errorf("unable to create Velero backup for %s: %v", backup.Name, err)
		}
		backup.Status.VeleroBackup = veleroBackup
	}
	return updateStatus(ctx, backup, c)
}

//TODO check whether this list change for version (backup) and how we can detect whether ACM or OCM
var backupNamespacesACM = [...]string{"open-cluster-management-agent", "open-cluster-management-hub", "hive", "openshift-operator-lifecycle-manager"}
var backupNamespacesOCM = [...]string{"open-cluster-management", "open-cluster-management-hub", "hive", "openshift-operator-lifecycle-manager"}

func (r *BackupReconciler) createVeleroBackupFromBackup(ctx context.Context, backup *v1alpha1.Backup, c client.Client) (*vapi.Backup, error) {
	veleroBackup := &vapi.Backup{}
	managedClusterList := clusterv1.ManagedClusterList{}
	if err := c.List(ctx, &managedClusterList, &client.ListOptions{}); err != nil {
		return nil, fmt.Errorf("unable to list managedclusters: %v", err)
	}

	veleroBackup.Name = generateVeleroBackupName(backup.Name, backup.Spec.VeleroConfig.Namespace)
	veleroBackup.Namespace = backup.Spec.VeleroConfig.Namespace
	var clusterResource bool = false
	veleroBackup.Spec.IncludeClusterResources = &clusterResource
	veleroBackup.Spec.ExcludedResources = append(veleroBackup.Spec.ExcludedResources, "certificatesigningrequests")

	for i := range managedClusterList.Items {
		if managedClusterList.Items[i].Name == "local-cluster" {
			continue
		}
		veleroBackup.Spec.IncludedNamespaces = append(veleroBackup.Spec.IncludedNamespaces, managedClusterList.Items[i].Name)
	}
	if len(veleroBackup.Spec.IncludedNamespaces) == 0 {
		return nil, fmt.Errorf("no managedclusters found, nothing to backup")
	}
	for i := range backupNamespacesOCM {
		veleroBackup.Spec.IncludedNamespaces = append(veleroBackup.Spec.IncludedNamespaces, backupNamespacesOCM[i])
	}

	if err := ctrl.SetControllerReference(backup, veleroBackup, r.Scheme); err != nil {
		return nil, err
	}

	err := c.Create(ctx, veleroBackup, &client.CreateOptions{})
	return veleroBackup, err
}
