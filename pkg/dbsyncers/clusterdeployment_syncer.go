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
	cdv1 "github.com/openshift/hive/apis/hive/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	controllerName             = "clusterdeployment-status-syncer"
	componentClusterdeployment = "clusterdeployments"
)

type clusterdeploymentDBSyncer struct {
	client                 client.Client
	log                    logr.Logger
	databaseConnectionPool *pgxpool.Pool
	syncInterval           time.Duration
	statusTableName        string
	specTableName          string
}

func (syncer *clusterdeploymentDBSyncer) Start(stopChannel <-chan struct{}) error {
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
func (syncer *clusterdeploymentDBSyncer) sync(ctx context.Context) {
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

		instance := &cdv1.ClusterDeployment{}
		err = syncer.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, instance)

		if err != nil {
			syncer.log.Error(err, "error in getting CR", "name", name, "namespace", namespace)
			continue
		}

		go syncer.handle(ctx, instance)
	}
}

// handle, read the clusterdeployment's status from DB, then write the status to the cluster's CR.

func (syncer *clusterdeploymentDBSyncer) handle(ctx context.Context, instance *cdv1.ClusterDeployment) {
	syncer.log.Info(fmt.Sprintf("handling %s, instance: %s/%s, with uuid: %s", componentClusterdeployment, instance.GetNamespace(), instance.GetName(), string(instance.GetUID())))

	rows, err := syncer.databaseConnectionPool.Query(ctx,
		fmt.Sprintf(`SELECT leaf_hub_name, payload FROM status.%s
			     WHERE id = '%s' ORDER BY leaf_hub_name`,
			syncer.statusTableName, string(instance.GetUID())))
	if err != nil {
		syncer.log.Error(err, "error in getting policy statuses from DB")
	}

	for rows.Next() {
		var leafHubName, statusInDBStr string

		err := rows.Scan(&leafHubName, &statusInDBStr)
		if err != nil {
			syncer.log.Error(err, "error in select", "table", syncer.statusTableName)
			continue
		}

		cdStatus := &cdv1.ClusterDeploymentStatus{}

		if err := json.Unmarshal([]byte(statusInDBStr), &cdStatus); err != nil {
			syncer.log.Error(err, "failed to Unmarshal %s status string", componentClusterdeployment)
			continue
		}

		syncer.log.Info(fmt.Sprintf("handling a line in %s with leaf hub cluster: %s, with status payload: %s", syncer.statusTableName, leafHubName, statusInDBStr))

		if err := syncer.updateStatusFromDBtoCluter(ctx, instance, instance.DeepCopy(), cdStatus); err != nil {
			syncer.log.Error(err, "Failed to update %s status", syncer.statusTableName)
		}

	}

}

func (syncer *clusterdeploymentDBSyncer) updateStatusFromDBtoCluter(ctx context.Context, instance, originalInstance *cdv1.ClusterDeployment,
	statusInDB *cdv1.ClusterDeploymentStatus) error {
	instance.Status = *statusInDB

	err := syncer.client.Status().Patch(ctx, instance, client.MergeFrom(originalInstance))
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to update %s CR %s/%s: %w", syncer.statusTableName, instance.GetNamespace(), instance.GetName(), err)
	}

	return nil
}

func addClusterdeploymentDBSyncer(mgr ctrl.Manager, databaseConnectionPool *pgxpool.Pool, syncInterval time.Duration) error {
	err := mgr.Add(&clusterdeploymentDBSyncer{
		client:                 mgr.GetClient(),
		log:                    ctrl.Log.WithName(controllerName),
		databaseConnectionPool: databaseConnectionPool,
		syncInterval:           syncInterval,
		statusTableName:        componentClusterdeployment,
		specTableName:          componentClusterdeployment,
	})
	if err != nil {
		return fmt.Errorf("failed to add %s syncer to the manager, err: %w", componentClusterdeployment, err)
	}

	return nil
}
