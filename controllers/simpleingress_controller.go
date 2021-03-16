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
	"github.com/dgraph-io/badger/v3"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SimpleIngressReconciler reconciles a SimpleIngress object
type SimpleIngressReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=sap.simpleingress.io,resources=simpleingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sap.simpleingress.io,resources=simpleingresses/status,verbs=get;update;patch
// TODO:: RBAC STUFF

func (r *SimpleIngressReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("simpleingress", req.NamespacedName)

	// your logic here
	var simpleIngress sapv1alpha1.SimpleIngress
	if err := r.Get(ctx, req.NamespacedName, &simpleIngress); err != nil {
		log.Error(err, "unable to fetch simpleingress")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	CreateOrUpdateDBRules(simpleIngress, log)

	var childSimpleIngress sapv1alpha1.SimpleIngressList
	if err := r.List(ctx, &childSimpleIngress, client.InNamespace(req.Namespace)); err != nil {
		log.Error(err, "unable to list child simple ingress")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *SimpleIngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sapv1alpha1.SimpleIngress{}).
		Complete(r)
}

//func DeleteUnusedRulesFromDB() {
//
//}

func CreateOrUpdateDBRules(simpleIngress sapv1alpha1.SimpleIngress, log logr.Logger) {
	log.Info("HERE HERE")
	db, err := badger.Open(badger.DefaultOptions("/rp/badger"))
	if err != nil {
		log.Error(err, "Failed to open reverse proxy rules database")
	}
	defer db.Close()

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
