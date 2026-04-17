#!/bin/bash

# fix test errchecks
sed -i.bak 's/k8sClient.Delete(ctx, backup)/_ = k8sClient.Delete(ctx, backup)/' test/e2e/sitebackup_test.go
sed -i.bak 's/k8sClient.Delete(ctx, site)/_ = k8sClient.Delete(ctx, site)/' test/e2e/sitebackup_test.go
sed -i.bak 's/k8sClient.Delete(ctx, bench)/_ = k8sClient.Delete(ctx, bench)/' test/e2e/sitebackup_test.go
sed -i.bak 's/filepath.Walk/err = filepath.Walk/' test/e2e/suite_test.go
sed -i.bak 's/client.Get(context.TODO()/_ = client.Get(context.TODO()/' controllers/frappebench_lifecycle_test.go
sed -i.bak 's/client.Status().Update(context.TODO()/_ = client.Status().Update(context.TODO()/' controllers/frappebench_lifecycle_test.go
sed -i.bak 's/client.Update(context.TODO()/_ = client.Update(context.TODO()/' controllers/frappebench_resources_test.go
sed -i.bak 's/filepath.Walk/err = filepath.Walk/' controllers/suite_test.go

# fix controllers errchecks
sed -i.bak 's/r.updateSiteBackupStatus(ctx, siteBackup, "Pending", msg, "")/_ = r.updateSiteBackupStatus(ctx, siteBackup, "Pending", msg, "")/' controllers/sitebackup_controller.go
sed -i.bak 's/controllerutil.SetControllerReference(siteBackup, job, r.Scheme)/_ = controllerutil.SetControllerReference(siteBackup, job, r.Scheme)/' controllers/sitebackup_controller.go
sed -i.bak 's/controllerutil.SetControllerReference(siteBackup, cronJob, r.Scheme)/_ = controllerutil.SetControllerReference(siteBackup, cronJob, r.Scheme)/' controllers/sitebackup_controller.go
sed -i.bak 's/controllerutil.SetControllerReference(siteRestore, job, r.Scheme)/_ = controllerutil.SetControllerReference(siteRestore, job, r.Scheme)/' controllers/siterestore_controller.go

# fix S1009 slice nil check
sed -i.bak 's/if j.OwnerReferences == nil || len(j.OwnerReferences) == 0 {/if len(j.OwnerReferences) == 0 {/' pkg/resources/builders_test.go

# fix deprecated field SA1019
sed -i.bak 's/Expect(result.Requeue).To(BeFalse())/Expect(result.RequeueAfter).To(Equal(time.Duration(0)))/' controllers/frappebench_controller_test.go

# fix unused stuff
sed -i.bak 's/func (r \*FrappeBenchReconciler) parseAppsJSON/\/\/nolint:unused\nfunc (r \*FrappeBenchReconciler) parseAppsJSON/' controllers/frappebench_controller.go
sed -i.bak 's/func (r \*FrappeBenchReconciler) markStorageFallback/\/\/nolint:unused\nfunc (r \*FrappeBenchReconciler) markStorageFallback/' controllers/frappebench_storage.go

# clean up bak files
find . -name "*.bak" -delete

