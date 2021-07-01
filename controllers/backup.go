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

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/open-cluster-management/backup-n-restore/api/v1alpha1"
)

func isBackupNotYetStarted(backup *v1alpha1.Backup, c client.Client) bool {
	if backup.Status.VeleroBackup == nil {
		return true
	}
	return false
}

func isBackupFinished(backup *v1alpha1.Backup) bool {
	switch {
	case backup.Status.VeleroBackup == nil:
		return false
	case backup.Status.VeleroBackup.Status.Phase == "Completed" ||
		backup.Status.VeleroBackup.Status.Phase == "Failed" ||
		backup.Status.VeleroBackup.Status.Phase == "PartiallyFailed":
		return true
	}
	return false
}

func updateStatus(ctx context.Context, backup *v1alpha1.Backup, c client.Client) (*v1alpha1.Backup, error) {
	if err := c.Status().Update(ctx, backup, &client.UpdateOptions{}); err != nil {
		return backup, fmt.Errorf("unable to update status %s/%s: %v", backup.Namespace, backup.Name, err)
	}
	return backup, nil
}

// TODO: implements this
func generateVeleroBackupName(backupName, backupNamesapce string) string {
	return backupName
}
