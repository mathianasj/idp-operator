package db

import (
	"context"

	v1alpha1argo "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	cloudfirstv1alpha1 "github.com/mathianasj/idp-operator/api/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func GetOnPremDbHost(rdbmsInstance *cloudfirstv1alpha1.Rdbms, client client.Client, ctx context.Context) (*string, error) {
	dbHost := rdbmsInstance.Namespace + "-" + rdbmsInstance.Name + "-mysql"
	return &dbHost, nil
}

func NewOnPrem(ctx context.Context, req ctrl.Request, client client.Client, scheme *runtime.Scheme, instance *cloudfirstv1alpha1.Rdbms) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("updating the on prem rdbms instance")

	// get argo app since local cluster
	appInstance := &v1alpha1argo.Application{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: "openshift-gitops",
		Name:      instance.Namespace + "-" + instance.Name,
	}, appInstance)

	appInstance.Name = instance.Namespace + "-" + instance.Name
	appInstance.Namespace = "openshift-gitops"
	appInstance.Spec.Destination.Namespace = instance.Namespace
	appInstance.Spec.Destination.Server = "https://kubernetes.default.svc"
	appInstance.Spec.Source.Chart = "mysql"
	appInstance.Spec.Source.RepoURL = "https://charts.bitnami.com/bitnami"
	appInstance.Spec.Source.TargetRevision = "9.4.1"
	appInstance.Spec.Source.Helm = &v1alpha1argo.ApplicationSourceHelm{
		Parameters: []v1alpha1argo.HelmParameter{
			{
				Name:  "primary.containerSecurityContext.enabled",
				Value: "false",
			},
			{
				Name:  "primary.podSecurityContext.enabled",
				Value: "false",
			},
			{
				Name:  "auth.database",
				Value: instance.Spec.Database,
			},
		},
	}
	appInstance.Spec.Project = "default"
	appInstance.Spec.SyncPolicy = &v1alpha1argo.SyncPolicy{
		Automated: &v1alpha1argo.SyncPolicyAutomated{
			Prune:    true,
			SelfHeal: true,
		},
	}

	// update status to which instance type
	instance.Status.DatabaseType = "ON_PREM"

	// set the owner as the rdbms cr
	controllerutil.SetOwnerReference(instance, appInstance, scheme)

	if err != nil {
		// not found so create it
		if apierrors.IsNotFound(err) {
			logger.Info("No argo app for mysql creating one")

			err = client.Create(ctx, appInstance)

			if err != nil {
				logger.Error(err, "Error creating argo mysql app", "appInstance", appInstance, "instance", instance)
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, err
		}
	} else {
		// this will be update stuff
		return ctrl.Result{}, client.Update(ctx, appInstance)
	}

	return ctrl.Result{}, nil
}
