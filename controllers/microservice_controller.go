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

	corev1 "k8s.io/api/core/v1"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	apimtypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/mathianasj/idp-operator/db"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	v1alpha1rds "github.com/aws-controllers-k8s/rds-controller/apis/v1alpha1"
	cloudfirstv1alpha1 "github.com/mathianasj/idp-operator/api/v1alpha1"

	v1alpha1 "knative.dev/serving/pkg/apis/serving/v1"
)

// MicroserviceReconciler reconciles a Microservice object
type MicroserviceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cloudfirst.cloudfirst.dev,resources=microservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudfirst.cloudfirst.dev,resources=microservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudfirst.cloudfirst.dev,resources=microservices/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Microservice object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *MicroserviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("namespaceconfig", req.NamespacedName)
	logger.Info("reconciling started")

	instance := &cloudfirstv1alpha1.Microservice{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	logger.Info("got", "instance", instance.Spec)

	// check for service
	servingInstance := &v1alpha1.Service{}
	servingInstanceErr := r.Client.Get(ctx, req.NamespacedName, servingInstance)

	// need to create the serverless
	servingInstance.Name = instance.Name
	servingInstance.Namespace = instance.Namespace
	servingInstance.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Image: instance.Spec.Image,
		},
	}

	if instance.Spec.RDBMSReference != nil {
		// get the rdbms reference
		rdbmsInstance := &cloudfirstv1alpha1.Rdbms{}
		err = r.Client.Get(ctx, apimtypes.NamespacedName{
			Namespace: instance.Namespace,
			Name:      instance.Spec.RDBMSReference.Name,
		}, rdbmsInstance)
		if err != nil {
			logger.Error(err, "error fetching referenced rdbmsInstance")
		}
		logger.Info("found an rdbms instance", "rdbmsInstance", rdbmsInstance)

		// lookup and set the password secret envs
		if rdbmsInstance.Status.DatabaseType == "ON_PREM" {
			servingInstance.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "DB_USERNAME",
					Value: "root",
				},
				{
					Name: "DB_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: rdbmsInstance.Namespace + "-" + rdbmsInstance.Name + "-mysql",
							},
							Key: "mysql-root-password",
						},
					},
				},
				{
					Name:  "DB_NAME",
					Value: rdbmsInstance.Spec.Database,
				},
			}
		} else if rdbmsInstance.Status.DatabaseType == "AWS_RDS" {
			servingInstance.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name: "DB_USERNAME",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: rdbmsInstance.Name + "-password",
							},
							Key: "password",
						},
					},
				},
				{
					Name: "DB_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: rdbmsInstance.Name + "-password",
							},
							Key: "password",
						},
					},
				},
				{
					Name:  "DB_NAME",
					Value: rdbmsInstance.Spec.Database,
				},
			}
		}

		// create db instance
		var dbHost *string
		if rdbmsInstance.Status.DatabaseType == "ON_PREM" {
			dbHost, err = db.GetOnPremDbHost(rdbmsInstance, r.Client, ctx)
		} else if rdbmsInstance.Status.DatabaseType == "AWS_RDS" {
			dbHost, err = db.GetRDSDbHost(rdbmsInstance, r.Client, ctx)
		}

		if err != nil {
			logger.Error(err, "failed to get db host")
			return ctrl.Result{}, err
		}

		// lookup and set the password secret envs
		if dbHost != nil {
			envFound := false
			for _, env := range servingInstance.Spec.Template.Spec.Containers[0].Env {
				if env.Name == "DB_HOST" {
					env.Value = *dbHost
					envFound = true
				}
			}

			if !envFound {
				servingInstance.Spec.Template.Spec.Containers[0].Env = append(servingInstance.Spec.Template.Spec.Containers[0].Env,
					corev1.EnvVar{
						Name:  "DB_HOST",
						Value: *dbHost,
					},
				)
			}
		}
	}

	controllerutil.SetControllerReference(instance, servingInstance, r.Scheme)

	if servingInstanceErr != nil {
		if apierrors.IsNotFound(servingInstanceErr) {
			logger.Info("No serverless found creating a new one")

			err = r.Client.Create(ctx, servingInstance)

			if err != nil {
				logger.Error(err, "error creating serverless", "serviceInstance", servingInstance)
				return ctrl.Result{}, err
			}
		} else {
			// Error reading the object - requeue the request.
			return ctrl.Result{}, err
		}
	} else {
		servingInstance.Spec.Template.Spec.Containers[0].Image = instance.Spec.Image

		err = r.Client.Update(ctx, servingInstance)

		if err != nil {
			logger.Error(err, "error creating serverless", "serviceInstance", servingInstance)
			return ctrl.Result{}, err
		}
	}

	logger.Info("got", "serverless", servingInstance.Spec)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MicroserviceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	_ = mgr.GetFieldIndexer().IndexField(context.Background(), &cloudfirstv1alpha1.Microservice{}, "spec.rdbms.name", func(rawObj client.Object) []string {
		event := rawObj.(*cloudfirstv1alpha1.Microservice)
		return []string{event.Spec.RDBMSReference.Name}
	})

	// filter if something changed (database is all for now)
	detailsChanged := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			newMicroservice, _ := e.ObjectNew.DeepCopyObject().(*cloudfirstv1alpha1.Microservice)
			oldMicroservice, _ := e.ObjectOld.DeepCopyObject().(*cloudfirstv1alpha1.Microservice)

			shouldUpdate := false

			if newMicroservice.Spec.Image != oldMicroservice.Spec.Image {
				shouldUpdate = true
			}

			if newMicroservice.Spec.Serverless != oldMicroservice.Spec.Serverless {
				shouldUpdate = true
			}

			if newMicroservice.Spec.RDBMSReference.Name != oldMicroservice.Spec.RDBMSReference.Name {
				shouldUpdate = true
			}

			return shouldUpdate
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudfirstv1alpha1.Microservice{}, builder.WithPredicates(detailsChanged)).
		Watches(&source.Kind{Type: &v1alpha1rds.DBInstance{
			TypeMeta: v1meta.TypeMeta{
				Kind: "DBInstance",
			},
		}}, &enqueueRequestForReferencingDbInstance{
			Client: mgr.GetClient(),
		}).
		Complete(r)
}

type enqueueRequestForReferencingDbInstance struct {
	client.Client
}

func (e *enqueueRequestForReferencingDbInstance) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	microserviceList, _ := e.matchMicroInstances(e.Client, types.NamespacedName{
		Name:      evt.Object.GetName(),
		Namespace: evt.Object.GetNamespace(),
	})

	for _, microservice := range microserviceList.Items {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: microservice.GetNamespace(),
			Name:      microservice.GetName(),
		}})
	}
}

func (e *enqueueRequestForReferencingDbInstance) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {

}

func (e *enqueueRequestForReferencingDbInstance) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {

}

func (e *enqueueRequestForReferencingDbInstance) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	microserviceList, _ := e.matchMicroInstances(e.Client, types.NamespacedName{
		Name:      evt.ObjectNew.GetName(),
		Namespace: evt.ObjectNew.GetNamespace(),
	})

	for _, microservice := range microserviceList.Items {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: microservice.GetNamespace(),
			Name:      microservice.GetName(),
		}})
	}
}

func (e *enqueueRequestForReferencingDbInstance) matchMicroInstances(c client.Client, microservice types.NamespacedName) (*cloudfirstv1alpha1.MicroserviceList, error) {
	microservicelist := &cloudfirstv1alpha1.MicroserviceList{}
	err := c.List(context.TODO(), microservicelist, client.MatchingFields{"spec.rdbms.name": microservice.Name}, &client.ListOptions{Namespace: microservice.Namespace})
	if err != nil {
		// e.logger.Error(err, "got an error finding microservices")
	}

	return microservicelist, err
}
