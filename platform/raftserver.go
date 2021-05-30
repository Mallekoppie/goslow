package platform

import (
	"fmt"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"go.uber.org/zap"
	"net"
	"os"
	"time"
)

func StartRaftServer(fsm raft.FSM) {
	conf, err := getPlatformConfiguration()
	if err != nil {
		Logger.Fatal("Unable to get platform configuration to check if Raft server can be used")
	}

	if conf.Raft.Enabled == false {
		Logger.Info("Raft not enabled")

		return
	}

	provided := conf.Raft

	serverId := raft.ServerID(conf.Raft.NodeId)
	config := raft.DefaultConfig()
	config.LocalID = serverId

	readConfigFromFileAndConfigure(provided, config)

	store, err := raftboltdb.NewBoltStore(conf.Raft.StoreDir)
	if err != nil {
		Logger.Fatal("Creating new Raftboltstore", zap.Error(err))
	}

	cacheStore, err := raft.NewLogCache(512, store)
	if err != nil {
		Logger.Fatal("Error creating log cache", zap.Error(err))
	}

	snapshotStore, err := raft.NewFileSnapshotStore(provided.SnapshotDir, 2, os.Stdout)
	if err != nil {
		Logger.Fatal("Error creating snaphotstore ", zap.Error(err))
	}

	raftBindAddr := fmt.Sprintf("%s:%s", provided.BindAddress, provided.BindPort)
	addr, err := net.ResolveTCPAddr("tcp", raftBindAddr)
	if err != nil {
		Logger.Fatal("Error resolving tcp address for raft: ", zap.Error(err))
	}

	tcpTimeout := time.Second * 30
	if len(provided.TcpTimeout) > 1 {
		tcpTimeout, err = time.ParseDuration(provided.TcpTimeout)
		if err != nil {
			Logger.Fatal("Parsing raft TcpTimeout setting from config file", zap.Error(err))
		}
	}

	transport, err := raft.NewTCPTransport(raftBindAddr, addr, provided.TcpMaxPool, tcpTimeout, os.Stdout)
	if err != nil {
		Logger.Fatal("Error creating raft transport: ", zap.Error(err))
	}

	newRaft, err := raft.NewRaft(config, fsm, cacheStore, store, snapshotStore, transport)
	if err != nil {
		Logger.Fatal("Error creating raft server: ", zap.Error(err))
	}

	raftNodes := getRaftNodes(provided)
	configuration := raft.Configuration{
		Servers: raftNodes,
	}

	newRaft.BootstrapCluster(configuration)
}

func getRaftNodes(provided raftConfiguration) []raft.Server {
	result := make([]raft.Server, 0)

	for _, v := range provided.RaftNodes {
		server := raft.Server{
			ID:      raft.ServerID(v.NodeID),
			Address: raft.ServerAddress(v.Address),
		}

		result = append(result, server)
	}

	return result
}

func readConfigFromFileAndConfigure(provided raftConfiguration, config *raft.Config) {
	if len(provided.CommitTimeout) > 0 {
		commitTimeout, err := time.ParseDuration(provided.CommitTimeout)
		if err != nil {
			Logger.Fatal("Parsing raft CommitTimeoutMS setting from config file", zap.Error(err))
		}
		config.CommitTimeout = commitTimeout
	}

	if len(provided.ElectionTimeout) > 0 {
		electionTimeout, err := time.ParseDuration(provided.ElectionTimeout)
		if err != nil {
			Logger.Fatal("Parsing raft ElectionTimeout setting from config file", zap.Error(err))
		}

		config.ElectionTimeout = electionTimeout
	}

	if len(provided.HeartbeatTimeout) > 0 {
		heartbeatTimeout, err := time.ParseDuration(provided.HeartbeatTimeout)
		if err != nil {
			Logger.Fatal("Parsing raft heartbeatTimeout setting from config file", zap.Error(err))
		}
		config.HeartbeatTimeout = heartbeatTimeout
	}

	if provided.MaxAppendEntries > 0 {
		config.MaxAppendEntries = provided.MaxAppendEntries
	}

	if provided.ShutdownOnRemove != nil {
		config.ShutdownOnRemove = *provided.ShutdownOnRemove
	}

	if provided.TrailingLogs > 0 {
		config.TrailingLogs = provided.TrailingLogs
	}

	if len(provided.SnapshotInterval) > 0 {
		snapshotInterval, err := time.ParseDuration(provided.SnapshotInterval)
		if err != nil {
			Logger.Fatal("Parsing raft SnapshotInterval setting from config file", zap.Error(err))
		}
		config.SnapshotInterval = snapshotInterval
	}

	if provided.SnapshotThreshold > 0 {
		config.SnapshotThreshold = provided.SnapshotThreshold
	}

	if len(provided.LeaderLeaseTimeout) > 0 {
		leaderLeaseTimeout, err := time.ParseDuration(provided.LeaderLeaseTimeout)
		if err != nil {
			Logger.Fatal("Parsing raft LeaderLeaseTimeout setting from config file", zap.Error(err))
		}
		config.LeaderLeaseTimeout = leaderLeaseTimeout
	}

	if len(provided.LogLevel) > 0 {
		config.LogLevel = provided.LogLevel
	}
}
