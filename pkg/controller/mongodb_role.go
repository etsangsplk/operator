package controller

import (
	"fmt"
	"time"

	"github.com/appscode/go/encoding/json/types"
	core_util "github.com/appscode/kutil/core/v1"
	meta_util "github.com/appscode/kutil/meta"
	"github.com/appscode/kutil/tools/queue"
	"github.com/golang/glog"
	"github.com/kubedb/apimachinery/apis"
	api "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	patchutil "github.com/kubedb/apimachinery/client/clientset/versioned/typed/authorization/v1alpha1/util"
	vsapis "github.com/kubevault/operator/apis"
	"github.com/kubevault/operator/pkg/vault/role/database"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MongoDBRolePhaseSuccess    api.MongoDBRolePhase = "Success"
	MongoDBRoleConditionFailed                      = "Failed"
	finalizerInterval                               = 5 * time.Second
	finalizerTimeout                                = 30 * time.Second
)

func (c *VaultController) initMongoDBRoleWatcher() {
	c.mgRoleInformer = c.dbInformerFactory.Authorization().V1alpha1().MongoDBRoles().Informer()
	c.mgRoleQueue = queue.New(api.ResourceKindMongoDBRole, c.MaxNumRequeues, c.NumThreads, c.runMongoDBRoleInjector)
	c.mgRoleInformer.AddEventHandler(queue.NewObservableHandler(c.mgRoleQueue.GetQueue(), apis.EnableStatusSubresource))
	c.mgRoleLister = c.dbInformerFactory.Authorization().V1alpha1().MongoDBRoles().Lister()
}

func (c *VaultController) runMongoDBRoleInjector(key string) error {
	obj, exist, err := c.mgRoleInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("MongoDBRole %s does not exist anymore", key)

	} else {
		mRole := obj.(*api.MongoDBRole).DeepCopy()

		glog.Infof("Sync/Add/Update for MongoDBRole %s/%s", mRole.Namespace, mRole.Name)

		if mRole.DeletionTimestamp != nil {
			if core_util.HasFinalizer(mRole.ObjectMeta, apis.Finalizer) {
				go c.runMongoDBRoleFinalizer(mRole, finalizerTimeout, finalizerInterval)
			}
		} else {
			if !core_util.HasFinalizer(mRole.ObjectMeta, apis.Finalizer) {
				// Add finalizer
				_, _, err := patchutil.PatchMongoDBRole(c.dbClient.AuthorizationV1alpha1(), mRole, func(role *api.MongoDBRole) *api.MongoDBRole {
					role.ObjectMeta = core_util.AddFinalizer(role.ObjectMeta, apis.Finalizer)
					return role
				})
				if err != nil {
					return errors.Wrapf(err, "failed to set MongoDBRole finalizer for %s/%s", mRole.Namespace, mRole.Name)
				}
			}

			dbRClient, err := database.NewDatabaseRoleForMongodb(c.kubeClient, c.appCatalogClient, mRole)
			if err != nil {
				return err
			}

			err = c.reconcileMongoDBRole(dbRClient, mRole)
			if err != nil {
				return errors.Wrapf(err, "for MongoDBRole %s/%s:", mRole.Namespace, mRole.Name)
			}
		}
	}
	return nil
}

// Will do:
//	For vault:
//	  - enable the database secrets engine if it is not already enabled
//	  - configure Vault with the proper Mongodb plugin and connection information
// 	  - configure a role that maps a name in Vault to an SQL statement to execute to create the database credential.
//    - sync role
//	  - revoke previous lease of all the respective mongodbRoleBinding and reissue a new lease
func (c *VaultController) reconcileMongoDBRole(dbRClient database.DatabaseRoleInterface, mgRole *api.MongoDBRole) error {
	status := mgRole.Status
	// enable the database secrets engine if it is not already enabled
	err := dbRClient.EnableDatabase()
	if err != nil {
		status.Conditions = []api.MongoDBRoleCondition{
			{
				Type:    MongoDBRoleConditionFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "FailedToEnableDatabase",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMongoDBRoleStatus(&status, mgRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to enable database secret engine")
	}

	// create database config for Mongodb
	err = dbRClient.CreateConfig()
	if err != nil {
		status.Conditions = []api.MongoDBRoleCondition{
			{
				Type:    MongoDBRoleConditionFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "FailedToCreateDatabaseConfig",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMongoDBRoleStatus(&status, mgRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to create database connection config")
	}

	// create role
	err = dbRClient.CreateRole()
	if err != nil {
		status.Conditions = []api.MongoDBRoleCondition{
			{
				Type:    MongoDBRoleConditionFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "FailedToCreateRole",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMongoDBRoleStatus(&status, mgRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to create role")
	}

	status.Conditions = []api.MongoDBRoleCondition{}
	status.Phase = MongoDBRolePhaseSuccess
	status.ObservedGeneration = types.NewIntHash(mgRole.Generation, meta_util.GenerationHash(mgRole))

	err = c.updatedMongoDBRoleStatus(&status, mgRole)
	if err != nil {
		return errors.Wrapf(err, "failed to update MongoDBRole status")
	}
	return nil
}

func (c *VaultController) updatedMongoDBRoleStatus(status *api.MongoDBRoleStatus, mRole *api.MongoDBRole) error {
	_, err := patchutil.UpdateMongoDBRoleStatus(c.dbClient.AuthorizationV1alpha1(), mRole, func(s *api.MongoDBRoleStatus) *api.MongoDBRoleStatus {
		s = status
		return s
	}, vsapis.EnableStatusSubresource)
	if err != nil {
		return err
	}
	return nil
}

func (c *VaultController) runMongoDBRoleFinalizer(mRole *api.MongoDBRole, timeout time.Duration, interval time.Duration) {
	if mRole == nil {
		glog.Infoln("MongoDBRole is nil")
		return
	}

	id := getMongoDBRoleId(mRole)
	if c.finalizerInfo.IsAlreadyProcessing(id) {
		// already processing
		return
	}

	glog.Infof("Processing finalizer for MongoDBRole %s/%s", mRole.Namespace, mRole.Name)
	// Add key to finalizerInfo, it will prevent other go routine to processing for this MongoDBRole
	c.finalizerInfo.Add(id)

	stopCh := time.After(timeout)
	finalizationDone := false
	timeOutOccured := false
	attempt := 0

	for {
		glog.Infof("MongoDBRole %s/%s finalizer: attempt %d\n", mRole.Namespace, mRole.Name, attempt)

		select {
		case <-stopCh:
			timeOutOccured = true
		default:
		}

		if timeOutOccured {
			break
		}

		if !finalizationDone {
			d, err := database.NewDatabaseRoleForMongodb(c.kubeClient, c.appCatalogClient, mRole)
			if err != nil {
				glog.Errorf("MongoDBRole %s/%s finalizer: %v", mRole.Namespace, mRole.Name, err)
			} else {
				err = c.finalizeMongoDBRole(d, mRole)
				if err != nil {
					glog.Errorf("MongoDBRole %s/%s finalizer: %v", mRole.Namespace, mRole.Name, err)
				} else {
					finalizationDone = true
				}
			}
		}

		if finalizationDone {
			err := c.removeMongoDBRoleFinalizer(mRole)
			if err != nil {
				glog.Errorf("MongoDBRole %s/%s finalizer: removing finalizer %v", mRole.Namespace, mRole.Name, err)
			} else {
				break
			}
		}

		select {
		case <-stopCh:
			timeOutOccured = true
		case <-time.After(interval):
		}
		attempt++
	}

	err := c.removeMongoDBRoleFinalizer(mRole)
	if err != nil {
		glog.Errorf("MongoDBRole %s/%s finalizer: removing finalizer %v", mRole.Namespace, mRole.Name, err)
	} else {
		glog.Infof("Removed finalizer for MongoDBRole %s/%s", mRole.Namespace, mRole.Name)
	}

	// Delete key from finalizer info as processing is done
	c.finalizerInfo.Delete(id)
}

// Do:
//	- delete role in vault
//	- revoke lease of all the corresponding mongodbRoleBinding
func (c *VaultController) finalizeMongoDBRole(dbRClient database.DatabaseRoleInterface, mRole *api.MongoDBRole) error {
	err := dbRClient.DeleteRole(mRole.RoleName())
	if err != nil {
		return errors.Wrap(err, "failed to database role")
	}
	return nil
}

func (c *VaultController) removeMongoDBRoleFinalizer(mRole *api.MongoDBRole) error {
	m, err := c.dbClient.AuthorizationV1alpha1().MongoDBRoles(mRole.Namespace).Get(mRole.Name, metav1.GetOptions{})
	if kerr.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	// remove finalizer
	_, _, err = patchutil.PatchMongoDBRole(c.dbClient.AuthorizationV1alpha1(), m, func(role *api.MongoDBRole) *api.MongoDBRole {
		role.ObjectMeta = core_util.RemoveFinalizer(role.ObjectMeta, apis.Finalizer)
		return role
	})
	return err
}

func getMongoDBRoleId(mRole *api.MongoDBRole) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourceMongoDBRole, mRole.Namespace, mRole.Name)
}