package naming

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
)

// local servers
var localss map[string]string

func Heartbeat(ctx context.Context) error {
	localss = make(map[string]string)
	if err := heartbeat(); err != nil {
		fmt.Println("failed to heartbeat:", err)
		return err
	}

	go loopHeartbeat()
	return nil
}

func loopHeartbeat() {
	interval := time.Duration(config.C.Heartbeat.Interval) * time.Millisecond
	for {
		time.Sleep(interval)
		if err := heartbeat(); err != nil {
			logger.Warning(err)
		}
	}
}

func heartbeat() error {
	var clusters []string
	var err error
	if config.C.ReaderFrom == "config" {
		// 在配置文件维护实例和集群的对应关系
		clusters = strings.Split(config.C.ClusterName, " ")
		for i := 0; i < len(clusters); i++ {
			err := models.AlertingEngineHeartbeat(config.C.Heartbeat.Endpoint, clusters[i])
			if err != nil {
				return err
			}
		}
	} else {
		// 在页面上维护实例和集群的对应关系
		clusters, err = models.AlertingEngineGetClusters(config.C.Heartbeat.Endpoint)
		if err != nil {
			return err
		}
		if len(clusters) == 0 {
			// 实例刚刚部署，还没有在页面配置 cluster 的情况，先使用配置文件中的 cluster 上报心跳
			clusters = strings.Split(config.C.ClusterName, " ")
			for i := 0; i < len(clusters); i++ {
				err := models.AlertingEngineHeartbeat(config.C.Heartbeat.Endpoint, clusters[i])
				if err != nil {
					return err
				}
			}
		}

		err := models.AlertingEngineHeartbeat(config.C.Heartbeat.Endpoint, "")
		if err != nil {
			return err
		}
	}

	for i := 0; i < len(clusters); i++ {
		servers, err := ActiveServers(clusters[i])
		if err != nil {
			logger.Warningf("hearbeat %s get active server err:", clusters[i], err)
			continue
		}

		sort.Strings(servers)
		newss := strings.Join(servers, " ")

		oldss, exists := localss[clusters[i]]
		if exists && oldss == newss {
			continue
		}

		RebuildConsistentHashRing(clusters[i], servers)
		localss[clusters[i]] = newss
	}

	return nil
}

func ActiveServers(cluster string) ([]string, error) {
	var err error
	if cluster == "" {
		cluster, err = models.AlertingEngineGetCluster(config.C.Heartbeat.Endpoint)
		if err != nil {
			return nil, err
		}
	}

	// 30秒内有心跳，就认为是活的
	return models.AlertingEngineGetsInstances("cluster = ? and clock > ?", cluster, time.Now().Unix()-30)
}
