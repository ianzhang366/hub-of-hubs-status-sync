// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v4/pgxpool"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cdv1 "github.com/openshift/hive/apis/hive/v1"
)

func addClusterdeploymentDBSyncer(mgr ctrl.Manager, databaseConnectionPool *pgxpool.Pool, syncInterval time.Duration) error {
	name := "clusterdeployments"

	cd := NewClusterlifecycleDBSyncer(mgr, databaseConnectionPool, syncInterval,
		withComponentNameAsTableNames(name),
		withGVK(metav1.GroupVersionKind{Group: cdv1.HiveAPIGroup, Version: cdv1.HiveAPIVersion, Kind: "ClusterDeployment"}))
	if err := mgr.Add(cd); err != nil {
		return fmt.Errorf("failed to add %s syncer to the manager, err: %w", name, err)
	}

	return nil
}

type clusterlifecycleDBSyncer struct {
	client                 client.Client
	log                    logr.Logger
	databaseConnectionPool *pgxpool.Pool
	syncInterval           time.Duration
	gvk                    metav1.GroupVersionKind
	statusTableName        string
	specTableName          string
}

type Option func(*clusterlifecycleDBSyncer)

func withComponentNameAsTableNames(name string) Option {
	return func(c *clusterlifecycleDBSyncer) {
		c.log = ctrl.Log.WithName(fmt.Sprintf("%s-status-syncer", name))
		c.specTableName = name
		c.statusTableName = name
	}
}

func withGVK(gvk metav1.GroupVersionKind) Option {
	return func(c *clusterlifecycleDBSyncer) {
		c.gvk = gvk
	}
}

func NewClusterlifecycleDBSyncer(mgr ctrl.Manager, databaseConnectionPool *pgxpool.Pool, syncInterval time.Duration, ops ...Option) *clusterlifecycleDBSyncer {
	c := &clusterlifecycleDBSyncer{
		client:                 mgr.GetClient(),
		databaseConnectionPool: databaseConnectionPool,
		syncInterval:           syncInterval,
	}

	for _, op := range ops {
		op(c)
	}

	return c
}

func (syncer *clusterlifecycleDBSyncer) Start(stopChannel <-chan struct{}) error {
	ticker := time.NewTicker(syncer.syncInterval)

	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	for {
		select {
		case <-stopChannel:
			ticker.Stop()

			syncer.log.Info("stop performing sync", "table", syncer.statusTableName)
			cancelContext()

			return nil
		case <-ticker.C:
			go syncer.sync(ctx)
		}
	}
}

// from spec table find the instance with delete flag==false,
// for these rows, do handle of each
func (syncer *clusterlifecycleDBSyncer) sync(ctx context.Context) {
	syncer.log.Info("performing sync", "table", syncer.statusTableName)

	rows, err := syncer.databaseConnectionPool.Query(ctx,
		fmt.Sprintf(`SELECT id, payload -> 'metadata' ->> 'name' as name, payload -> 'metadata' ->> 'namespace'
			    as namespace FROM spec.%s WHERE deleted = FALSE`, syncer.specTableName))
	if err != nil {
		syncer.log.Error(err, "error in getting policies spec")
		return
	}

	for rows.Next() {
		var id, name, namespace string

		err := rows.Scan(&id, &name, &namespace)
		if err != nil {
			syncer.log.Error(err, "error in select", "table", syncer.specTableName)
			continue
		}

		instance := &unstructured.Unstructured{}
		instance.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   syncer.gvk.Group,
			Version: syncer.gvk.Version,
			Kind:    syncer.gvk.Kind,
		})

		err = syncer.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, instance)

		if err != nil {
			syncer.log.Error(err, "error in getting CR", "name", name, "namespace", namespace)
			continue
		}

		go syncer.handle(ctx, instance)
	}
}

// handle, read the clusterdeployment's status from DB, then write the status to the cluster's CR.

func (syncer *clusterlifecycleDBSyncer) handle(ctx context.Context, clusterIns *unstructured.Unstructured) {
	syncer.log.Info(fmt.Sprintf("handling instance: %s/%s, with uuid: %s", clusterIns.GetNamespace(), clusterIns.GetName(), string(clusterIns.GetUID())))

	rows, err := syncer.databaseConnectionPool.Query(ctx,
		fmt.Sprintf(`SELECT leaf_hub_name, payload FROM status.%s
			     WHERE id = '%s' ORDER BY leaf_hub_name`,
			syncer.statusTableName, string(clusterIns.GetUID())))
	if err != nil {
		syncer.log.Error(err, "error in getting policy statuses from DB")
	}

	for rows.Next() {
		var leafHubName, instanceInDBStr string

		err := rows.Scan(&leafHubName, &instanceInDBStr)
		if err != nil {
			syncer.log.Error(err, "error in select", "table", syncer.statusTableName)
			continue
		}

		dbInstance := &unstructured.Unstructured{}

		if err := json.Unmarshal([]byte(instanceInDBStr), &dbInstance); err != nil {
			syncer.log.Error(err, "failed to Unmarshal")
			continue
		}

		syncer.log.Info(fmt.Sprintf("handling a line in %s with leaf hub cluster: %s", syncer.statusTableName, leafHubName))

		if err := syncer.updateStatusFromDBtoCluter(ctx, clusterIns, dbInstance); err != nil {
			syncer.log.Error(err, "Failed to update %s status", syncer.statusTableName)
		}

	}

}

func (syncer *clusterlifecycleDBSyncer) updateStatusFromDBtoCluter(ctx context.Context, clusterIns, dbInstance *unstructured.Unstructured) error {
	originalIns := clusterIns.DeepCopy()

	dbInstanceStatus, found, err := unstructured.NestedStringMap(dbInstance.Object, "status")
	if !found || err != nil {
		return fmt.Errorf("failed to get status field of DB object, either not found %v, or had error: %w", found, err)
	}

	unstructured.SetNestedStringMap(clusterIns.Object, dbInstanceStatus, "status")

	err = syncer.client.Status().Patch(ctx, clusterIns, client.MergeFrom(originalIns))
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to update %s CR %s/%s: %w", syncer.statusTableName, clusterIns.GetNamespace(), clusterIns.GetName(), err)
	}

	return nil
}
