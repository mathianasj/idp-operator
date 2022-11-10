package util

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	cloudfirstv1alpha1 "github.com/mathianasj/idp-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func UpdateSecret(ctx context.Context, req ctrl.Request, client client.Client, scheme *runtime.Scheme, instance *cloudfirstv1alpha1.Rdbms) error {
	secret := &corev1.Secret{}

	// see if the secret exists
	err := client.Get(ctx, req.NamespacedName, secret)

	// set values
	secret.Name = instance.Name + "-password"
	secret.Namespace = instance.Namespace
	secret.StringData = map[string]string{
		"password": "password",
	}
	controllerutil.SetOwnerReference(instance, secret, scheme)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// create new one
			return client.Create(ctx, secret)
		}
		return err
	}

	return client.Update(ctx, secret)
}
