package detectors

import (
	"time"

	"github.com/iyzitrace/inventory-service/internal/semrev/adapter"
	"github.com/iyzitrace/inventory-service/internal/semrev/model"
)

// K8sDetector detects Kubernetes entities and relationships.
type K8sDetector struct{}

// NewK8sDetector creates a new Kubernetes detector.
func NewK8sDetector() *K8sDetector {
	return &K8sDetector{}
}

// Name returns the detector name.
func (d *K8sDetector) Name() string {
	return "k8s"
}

// Detect inspects attributes and emits Kubernetes entities and relations.
func (d *K8sDetector) Detect(item *model.TelemetryItem, keys adapter.SemconvKeys, enums adapter.SemconvEnums) *model.ClassificationResult {
	result := &model.ClassificationResult{
		Entities:  make([]*model.Entity, 0),
		Relations: make([]*model.Relation, 0),
	}

	attrs := item.Attrs
	now := time.Now()

	// Skip if no K8s attributes
	if !hasAttr(attrs, keys.K8sClusterName) && !hasAttr(attrs, keys.K8sPodName) && !hasAttr(attrs, keys.K8sNamespaceName) {
		return result
	}

	// Detect Cluster
	var clusterEntity *model.Entity
	if hasAttr(attrs, keys.K8sClusterName) {
		clusterName := getAttr(attrs, keys.K8sClusterName)
		clusterUID := getAttr(attrs, keys.K8sClusterUID)

		builder := model.NewEntityBuilder(model.EntityTypeK8sCluster).
			WithName(clusterName).
			WithAttr("k8s.cluster.name", clusterName).
			WithEvidence(createEvidence(item, "1.26.0", keys.K8sClusterName, clusterName))

		if clusterUID != "" {
			builder.WithAttr("k8s.cluster.uid", clusterUID)
		}

		clusterEntity = builder.Build()
		result.Entities = append(result.Entities, clusterEntity)
	}

	// Detect Namespace
	var namespaceEntity *model.Entity
	if hasAttr(attrs, keys.K8sNamespaceName) {
		nsName := getAttr(attrs, keys.K8sNamespaceName)
		clusterName := getAttr(attrs, keys.K8sClusterName)

		builder := model.NewEntityBuilder(model.EntityTypeK8sNamespace).
			WithName(nsName).
			WithAttr("k8s.namespace.name", nsName).
			WithEvidence(createEvidence(item, "1.26.0", keys.K8sNamespaceName, nsName))

		if clusterName != "" {
			builder.WithAttr("k8s.cluster.name", clusterName)
		}

		namespaceEntity = builder.Build()
		result.Entities = append(result.Entities, namespaceEntity)

		// Relation: namespace part_of cluster
		if clusterEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    namespaceEntity.ID,
				ToID:      clusterEntity.ID,
				Type:      model.RelationTypePartOf,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sNamespaceName, nsName)},
			})
		}
	}

	// Detect Node
	var nodeEntity *model.Entity
	if hasAttr(attrs, keys.K8sNodeName) {
		nodeName := getAttr(attrs, keys.K8sNodeName)
		nodeUID := getAttr(attrs, keys.K8sNodeUID)
		clusterName := getAttr(attrs, keys.K8sClusterName)

		builder := model.NewEntityBuilder(model.EntityTypeK8sNode).
			WithName(nodeName).
			WithAttr("k8s.node.name", nodeName).
			WithEvidence(createEvidence(item, "1.26.0", keys.K8sNodeName, nodeName))

		if nodeUID != "" {
			builder.WithAttr("k8s.node.uid", nodeUID)
		}
		if clusterName != "" {
			builder.WithAttr("k8s.cluster.name", clusterName)
		}

		nodeEntity = builder.Build()
		result.Entities = append(result.Entities, nodeEntity)

		// Relation: node part_of cluster
		if clusterEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    nodeEntity.ID,
				ToID:      clusterEntity.ID,
				Type:      model.RelationTypePartOf,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sNodeName, nodeName)},
			})
		}
	}

	// Detect Deployment
	var deploymentEntity *model.Entity
	if hasAttr(attrs, keys.K8sDeploymentName) {
		deploymentName := getAttr(attrs, keys.K8sDeploymentName)
		nsName := getAttr(attrs, keys.K8sNamespaceName)
		clusterName := getAttr(attrs, keys.K8sClusterName)

		builder := model.NewEntityBuilder(model.EntityTypeK8sDeployment).
			WithName(deploymentName).
			WithAttr("k8s.deployment.name", deploymentName).
			WithEvidence(createEvidence(item, "1.26.0", keys.K8sDeploymentName, deploymentName))

		if nsName != "" {
			builder.WithAttr("k8s.namespace.name", nsName)
		}
		if clusterName != "" {
			builder.WithAttr("k8s.cluster.name", clusterName)
		}

		deploymentEntity = builder.Build()
		result.Entities = append(result.Entities, deploymentEntity)

		// Relation: deployment part_of namespace
		if namespaceEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    deploymentEntity.ID,
				ToID:      namespaceEntity.ID,
				Type:      model.RelationTypePartOf,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sDeploymentName, deploymentName)},
			})
		}
	}

	// Detect ReplicaSet
	var replicasetEntity *model.Entity
	if hasAttr(attrs, keys.K8sReplicaSetName) {
		rsName := getAttr(attrs, keys.K8sReplicaSetName)
		nsName := getAttr(attrs, keys.K8sNamespaceName)
		clusterName := getAttr(attrs, keys.K8sClusterName)

		builder := model.NewEntityBuilder(model.EntityTypeK8sReplicaSet).
			WithName(rsName).
			WithAttr("k8s.replicaset.name", rsName).
			WithEvidence(createEvidence(item, "1.26.0", keys.K8sReplicaSetName, rsName))

		if nsName != "" {
			builder.WithAttr("k8s.namespace.name", nsName)
		}
		if clusterName != "" {
			builder.WithAttr("k8s.cluster.name", clusterName)
		}

		replicasetEntity = builder.Build()
		result.Entities = append(result.Entities, replicasetEntity)

		// Relation: replicaset managed_by deployment (if deployment is known)
		if deploymentEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    replicasetEntity.ID,
				ToID:      deploymentEntity.ID,
				Type:      model.RelationTypeManagedBy,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sReplicaSetName, rsName)},
			})
		}
	}

	// Detect StatefulSet
	var statefulsetEntity *model.Entity
	if hasAttr(attrs, keys.K8sStatefulSetName) {
		stsName := getAttr(attrs, keys.K8sStatefulSetName)
		nsName := getAttr(attrs, keys.K8sNamespaceName)
		clusterName := getAttr(attrs, keys.K8sClusterName)

		builder := model.NewEntityBuilder(model.EntityTypeK8sStatefulSet).
			WithName(stsName).
			WithAttr("k8s.statefulset.name", stsName).
			WithEvidence(createEvidence(item, "1.26.0", keys.K8sStatefulSetName, stsName))

		if nsName != "" {
			builder.WithAttr("k8s.namespace.name", nsName)
		}
		if clusterName != "" {
			builder.WithAttr("k8s.cluster.name", clusterName)
		}

		statefulsetEntity = builder.Build()
		result.Entities = append(result.Entities, statefulsetEntity)

		// Relation: statefulset part_of namespace
		if namespaceEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    statefulsetEntity.ID,
				ToID:      namespaceEntity.ID,
				Type:      model.RelationTypePartOf,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sStatefulSetName, stsName)},
			})
		}
	}

	// Detect DaemonSet
	var daemonsetEntity *model.Entity
	if hasAttr(attrs, keys.K8sDaemonSetName) {
		dsName := getAttr(attrs, keys.K8sDaemonSetName)
		nsName := getAttr(attrs, keys.K8sNamespaceName)
		clusterName := getAttr(attrs, keys.K8sClusterName)

		builder := model.NewEntityBuilder(model.EntityTypeK8sDaemonSet).
			WithName(dsName).
			WithAttr("k8s.daemonset.name", dsName).
			WithEvidence(createEvidence(item, "1.26.0", keys.K8sDaemonSetName, dsName))

		if nsName != "" {
			builder.WithAttr("k8s.namespace.name", nsName)
		}
		if clusterName != "" {
			builder.WithAttr("k8s.cluster.name", clusterName)
		}

		daemonsetEntity = builder.Build()
		result.Entities = append(result.Entities, daemonsetEntity)

		// Relation: daemonset part_of namespace
		if namespaceEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    daemonsetEntity.ID,
				ToID:      namespaceEntity.ID,
				Type:      model.RelationTypePartOf,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sDaemonSetName, dsName)},
			})
		}
	}

	// Detect Pod
	if hasAttr(attrs, keys.K8sPodName) {
		podName := getAttr(attrs, keys.K8sPodName)
		podUID := getAttr(attrs, keys.K8sPodUID)
		nsName := getAttr(attrs, keys.K8sNamespaceName)
		clusterName := getAttr(attrs, keys.K8sClusterName)

		builder := model.NewEntityBuilder(model.EntityTypeK8sPod).
			WithName(podName).
			WithAttr("k8s.pod.name", podName).
			WithEvidence(createEvidence(item, "1.26.0", keys.K8sPodName, podName))

		if podUID != "" {
			builder.WithAttr("k8s.pod.uid", podUID)
			// Track the pod UID as an instance
			builder.WithInstance(podUID, map[string]string{
				"k8s.pod.name": podName,
			})
		}
		if nsName != "" {
			builder.WithAttr("k8s.namespace.name", nsName)
		}
		if clusterName != "" {
			builder.WithAttr("k8s.cluster.name", clusterName)
		}

		podEntity := builder.Build()
		result.Entities = append(result.Entities, podEntity)

		// Relation: pod part_of namespace
		if namespaceEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    podEntity.ID,
				ToID:      namespaceEntity.ID,
				Type:      model.RelationTypePartOf,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sPodName, podName)},
			})
		}

		// Relation: pod runs_on node
		if nodeEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    podEntity.ID,
				ToID:      nodeEntity.ID,
				Type:      model.RelationTypeRunsOn,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sPodName, podName)},
			})
		}

		// Relation: pod managed_by controller (deployment, replicaset, statefulset, daemonset)
		if replicasetEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    podEntity.ID,
				ToID:      replicasetEntity.ID,
				Type:      model.RelationTypeManagedBy,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sPodName, podName)},
			})
		} else if deploymentEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    podEntity.ID,
				ToID:      deploymentEntity.ID,
				Type:      model.RelationTypeManagedBy,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sPodName, podName)},
			})
		} else if statefulsetEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    podEntity.ID,
				ToID:      statefulsetEntity.ID,
				Type:      model.RelationTypeManagedBy,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sPodName, podName)},
			})
		} else if daemonsetEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    podEntity.ID,
				ToID:      daemonsetEntity.ID,
				Type:      model.RelationTypeManagedBy,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.K8sPodName, podName)},
			})
		}
	}

	return result
}
