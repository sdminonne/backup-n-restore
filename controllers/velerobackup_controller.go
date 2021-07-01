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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-cluster-management/backup-n-restore/api/v1alpha1"
	vapi "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type VeleroBackupController struct {
	client.Client
	Scheme *runtime.Scheme
}

func (c *VeleroBackupController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	instance := vapi.Backup{}
	var (
		backup *v1alpha1.Backup = nil
		err    error
	)
	if err = c.Get(ctx, req.NamespacedName, &instance); err != nil {
		logger.Error(err, "Could not Get velero.Backup ")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	backup, err = getBackupFromVeleroBackup(&instance)
	if err != nil {
		switch {
		case apierrors.IsNotFound(err):
			_, err = createBackupFromVeleroBackup(&instance)
			return ctrl.Result{}, err
		default:
			return ctrl.Result{}, err
		}
	}
	backup, err = updateBackupFromVeleroBackup(backup, &instance)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to update backup %s/%s: %v", backup.Namespace, backup.Name, err)
	}
	return ctrl.Result{}, nil

}

func filterVeleroBackup() predicate.Predicate {
	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"ocm-backup": "velero"}}
	always := true
	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		always = false
	}
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return always && selector.Matches(labels.Set(e.Object.GetLabels()))
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return always && selector.Matches(labels.Set(e.ObjectNew.GetLabels()))
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return always && selector.Matches(labels.Set(e.Object.GetLabels()))
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return always && selector.Matches(labels.Set(e.Object.GetLabels()))
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (c *VeleroBackupController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(filterVeleroBackup()).
		For(&vapi.Backup{}).
		Complete(c)
}
