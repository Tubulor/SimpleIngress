/*


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
	sapv1alpha1 "SimpleIngressSAP/api/v1alpha1"
	"context"
	"fmt"
	"github.com/dgraph-io/badger/v3"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
)

// SimpleIngressReconciler reconciles a SimpleIngress object
type SimpleIngressReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Database *badger.DB
}

var (
	mutex sync.Mutex
)

// +kubebuilder:rbac:groups=sap.simpleingress.io,resources=simpleingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sap.simpleingress.io,resources=simpleingresses/status,verbs=get;update;patch

func (r *SimpleIngressReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("simpleingress", req.NamespacedName)

	var simpleIngress sapv1alpha1.SimpleIngress
	if err := r.Get(ctx, req.NamespacedName, &simpleIngress); err != nil {
		log.Error(err, "unable to fetch simpleingress")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	//isExist := isRuleExist(simpleIngress, r.Database)
	//if isExist {
	//	log.Info("Failed to update reverse proxy rules configuration - rule already exist for this service ip")
	//	return ctrl.Result{}, errors.New("failed to update reverse proxy rules configuration - rule already exist for this service ip")
	//}

	var childSimpleIngress sapv1alpha1.SimpleIngressList
	if err := r.List(ctx, &childSimpleIngress, client.InNamespace(req.Namespace)); err != nil {
		log.Error(err, "unable to list child simple ingress")
		return ctrl.Result{}, err
	}

	CreateOrUpdateDBRules(simpleIngress, log, r.Database)

	simpleIngress.Status.ActiveRules = simpleIngress.Spec.Rules
	if err := r.Status().Update(ctx, &simpleIngress); err != nil {
		log.Error(err, "failed to update simple ingress status")
		return ctrl.Result{}, err
	}

	if err := DeleteInactiveRules(childSimpleIngress, r.Database, log); err != nil {
		log.Error(err, "failed to delete reverse proxy inactive rules")
		return ctrl.Result{}, err
	}

	log.Info("Rules applied successfully")
	return ctrl.Result{}, nil
}

func (r *SimpleIngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sapv1alpha1.SimpleIngress{}).
		Complete(r)
}

func isRuleExist(simpleIngress sapv1alpha1.SimpleIngress, db *badger.DB) bool {
	isExist := false
	for _, rule := range simpleIngress.Spec.Rules {
		err := db.View(func(txn *badger.Txn) error {
			_, err := txn.Get([]byte(rule.ServiceIP))
			if err != nil && err.Error() == "Key not found" {
				return err
			}
			return nil
		})
		if err == nil {
			isExist = true
		}
	}
	// If this rule already exist in Active rule it must be an update from consumer - relate as not exist.
	if isExist {
		for _, rule := range simpleIngress.Spec.Rules {
			for _, activeRule := range simpleIngress.Status.ActiveRules {
				if rule.ServiceIP == activeRule.ServiceIP {
					fmt.Printf("Active rules: %v, %v", rule.ServiceIP, rule.ServiceName)
					isExist = false
				}
			}
		}
	}
	return isExist
}

func DeleteInactiveRules(simpleIngress sapv1alpha1.SimpleIngressList, db *badger.DB, log logr.Logger) error {

	activeRules := make(map[string]string)
	for _, item := range simpleIngress.Items {
		for _, rule := range item.Spec.Rules {
			activeRules[rule.ServiceIP] = rule.ServiceName
		}
	}

	err := db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			err := item.Value(func(v []byte) error {
				if _, ok := activeRules[string(k)]; !ok {
					if err := txn.Delete(k); err != nil {
						log.Error(err, "Could not delete inactive rule.")
					}
					log.Info("Key was delete from database", string(k), string(v))
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func CreateOrUpdateDBRules(simpleIngress sapv1alpha1.SimpleIngress, log logr.Logger, db *badger.DB) {
	txn := db.NewTransaction(true)

	// Add or Update new reverse proxy rules.
	for _, rule := range simpleIngress.Spec.Rules {
		serviceIP := []byte(rule.ServiceIP)
		serviceName := []byte(rule.ServiceName)
		if err := txn.Set(serviceIP, serviceName); err == badger.ErrTxnTooBig {
			_ = txn.Commit()
			txn = db.NewTransaction(true)
			_ = txn.Set(serviceIP, serviceName)
		}
	}
	_ = txn.Commit()
}
