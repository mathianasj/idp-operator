/*
Copyright 2022.

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
	"os"

	certutil "github.com/redhat-cop/operator-utils/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudfirstv1alpha1 "github.com/mathianasj/idp-operator/api/v1alpha1"
	db "github.com/mathianasj/idp-operator/db"
	util "github.com/mathianasj/idp-operator/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// RdbmsReconciler reconciles a Rdbms object
type RdbmsReconciler struct {
	certutil.ReconcilerBase
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cloudfirst.cloudfirst.dev,resources=rdbms,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudfirst.cloudfirst.dev,resources=rdbms/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudfirst.cloudfirst.dev,resources=rdbms/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Rdbms object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *RdbmsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling started for rdbms")

	// get instance
	instance := &cloudfirstv1alpha1.Rdbms{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("got", "instance", instance)

	// create secret
	util.UpdateSecret(ctx, req, r.Client, r.Scheme, instance)

	envtype := os.Getenv("RUNTIME_TYPE")

	// create db instance
	if envtype == "ONPREM" {
		_, err = db.NewOnPrem(ctx, req, r.Client, r.Scheme, instance)
	} else if envtype == "AWS" {
		_, err = db.NewRDS(ctx, req, r.Client, r.Scheme, instance)
	}

	if err != nil {

		return ctrl.Result{}, err
	}

	logger.Info("updated status for rdbms to", "rdbms.status", instance.Status)
	return r.ManageSuccess(ctx, instance)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RdbmsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudfirstv1alpha1.Rdbms{}).
		Complete(r)
}
