package db

import (
	"context"

	v1alpha1rds "github.com/aws-controllers-k8s/rds-controller/apis/v1alpha1"
	"github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	cloudfirstv1alpha1 "github.com/mathianasj/idp-operator/api/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func GetRDSDbHost(rdbmsInstance *cloudfirstv1alpha1.Rdbms, client client.Client, ctx context.Context) (*string, error) {
	instance := &v1alpha1rds.DBInstance{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: rdbmsInstance.Namespace,
		Name:      rdbmsInstance.Name,
	}, instance)
	if err != nil {
		return nil, err
	}

	return instance.Status.Endpoint.Address, nil
}

func NewRDS(ctx context.Context, req ctrl.Request, client client.Client, scheme *runtime.Scheme, instance *cloudfirstv1alpha1.Rdbms) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// get argo app since local cluster
	dbInstance := &v1alpha1rds.DBInstance{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      instance.Name,
	}, dbInstance)

	// constants
	engine := "mysql"
	instanceClass := "db.t2.small"
	dbSubnetGroupName := "example"
	var allocatedStorage int64 = 20
	instanceIdentifier := instance.Namespace + "-" + instance.Name
	masterUsername := "admin"
	publiclyAccessible := true

	dbInstance.Name = instance.Name
	dbInstance.Namespace = instance.Namespace
	dbInstance.Spec.Engine = &engine
	dbInstance.Spec.DBInstanceClass = &instanceClass
	dbInstance.Spec.MasterUserPassword = &v1alpha1.SecretKeyReference{
		Key: "password",
	}
	dbInstance.Spec.MasterUserPassword.Name = instance.Name + "-" + "password"
	dbInstance.Spec.MasterUserPassword.Namespace = instance.Namespace
	dbInstance.Spec.DBSubnetGroupName = &dbSubnetGroupName
	dbInstance.Spec.AllocatedStorage = &allocatedStorage
	dbInstance.Spec.DBInstanceIdentifier = &instanceIdentifier
	dbInstance.Spec.MasterUsername = &masterUsername
	dbInstance.Spec.PubliclyAccessible = &publiclyAccessible
	dbInstance.Spec.DBName = &instance.Spec.Database

	logger.Info("updating dbinstance", "dbinstance", dbInstance.Spec)

	// update status to which instance type
	instance.Status.DatabaseType = "AWS_RDS"

	controllerutil.SetOwnerReference(instance, dbInstance, scheme)

	if err != nil {
		// not found so create it
		if apierrors.IsNotFound(err) {
			// set the owner as the rdbms cr
			logger.Info("No dbinstance creating one")
			err = client.Create(ctx, dbInstance)

			if err != nil {
				logger.Error(err, "Error creating argo mysql app", "appInstance", dbInstance, "instance", instance)
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, err
		}
	} else {
		// this will be update stuff
		logger.Info("dbinstance found updating it")
		err = client.Update(ctx, dbInstance)
		if err != nil {
			logger.Error(err, "could not update dbinstance")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
