#!/bin/bash

# fix controllers/frappebench_resources_test.go
sed -i.bak 's/client.Get(context.TODO()/_ = client.Get(context.TODO()/' controllers/frappebench_resources_test.go

# fix controllers/frappesite_credentials_test.go line 367
sed -i.bak 's/client.Get(context.TODO(), req.NamespacedName, updatedSite)/_ = client.Get(context.TODO(), req.NamespacedName, updatedSite)/' controllers/frappesite_credentials_test.go

# fix controllers/frappesite_credentials_test.go line 619 ineffassign
sed -i.bak 's/err = client.Get(context.TODO(), types.NamespacedName{Name: siteName + "-init-secrets", Namespace: namespace}, secret)/_ = client.Get(context.TODO(), types.NamespacedName{Name: siteName + "-init-secrets", Namespace: namespace}, secret)/' controllers/frappesite_credentials_test.go

# fix controllers/sitebackup_controller_test.go errchecks
sed -i.bak 's/k8sClient.Delete(ctx, siteBackup)/_ = k8sClient.Delete(ctx, siteBackup)/' controllers/sitebackup_controller_test.go
sed -i.bak 's/k8sClient.Delete(ctx, site)/_ = k8sClient.Delete(ctx, site)/' controllers/sitebackup_controller_test.go
sed -i.bak 's/k8sClient.Delete(ctx, bench)/_ = k8sClient.Delete(ctx, bench)/' controllers/sitebackup_controller_test.go

find . -name "*.bak" -delete
