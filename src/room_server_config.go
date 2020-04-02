package new_frame

import (
	"fmt"
	"new_frame/src/lgames"
	"time"

	"github.com/go-ini/ini"
)

type AgentConfig struct {
	EnableLogSend          bool
	EnableLogRecv          bool
	EnableCheckPing        bool
	EnableCheckLoginParams bool

	EnableCachedMsg   bool
	EnableCompressMsg bool

	CachedMsgMaxCount        int
	CompressMsgSizeThreshold int

	TimestampExpireDuration time.Duration
}

type MongoConf struct {
	Address string
	Name    string
}

type RedisConf struct {
	Address       string
	MasterName    string
}

type RoomServerConfig struct {
	GameID                string
	ChannelID             string
	AppKey                string
	MaxClient             int
	MaxPlayerInRoom       int
	WaitReconnectDuration time.Duration
	SaveRoomDataInterval  time.Duration

	Client *NetworkAsServer

	Agent    AgentConfig
	Mongo    MongoConf
	UseMongo bool
	Redis    RedisConf
	UseRedis bool


}

const (
	CFG_DIR = "../conf"
)

func NewRoomServerConfig(log *lgames.Logger) (*RoomServerConfig, error) {
	cfg, err := ini.Load(fmt.Sprintf("%s/game.ini", CFG_DIR))
	if err != nil {
		log.Error("load file game.ini err ")
		return nil, err
	}

	serverConfig := &RoomServerConfig{}
	section, err := cfg.GetSection("room")
	if err != nil {
		log.Error("get room section  err ")
		return nil, err
	}

	key, err := section.GetKey("gameID")
	if err != nil {
		log.Error("get room section key gameId  err ")
		return nil, err
	}
	serverConfig.GameID = key.String()

	key, err = section.GetKey("channelID")
	if err != nil {
		log.Error("get room section key channelID  err ")
		return nil, err
	}
	serverConfig.ChannelID = key.String()

	key, err = section.GetKey("level")
	if err != nil {
		log.ResetLevel("info")
	} else {
		logLevel := key.String()
		log.ResetLevel(logLevel)
	}

	key, err = section.GetKey("appKey")
	if err != nil {
		log.Error("get room section key appKey  err ")
		return nil, err
	}
	serverConfig.AppKey = key.String()

	key, err = section.GetKey("maxClient")
	if err != nil {
		log.Error("get room section key maxClient  err ")
		return nil, err
	}
	serverConfig.MaxClient = key.MustInt()
	if serverConfig.MaxClient <= 0 {
		log.Error("get room section key maxClient  err ")
		return nil, fmt.Errorf("unexpected maxClient=%d", serverConfig.MaxClient)
	}

	key, err = section.GetKey("maxPlayerInRoom")
	if err != nil {
		log.Error("get room section key maxPlayerInRoom  err ")
		return nil, err
	}
	serverConfig.MaxPlayerInRoom = key.MustInt()
	if serverConfig.MaxPlayerInRoom <= 0 {
		return nil, fmt.Errorf("unexpected maxPlayerInRoom=%d", serverConfig.MaxPlayerInRoom)
	}

	serverConfig.WaitReconnectDuration, err = LoadSecondFromIniSection(section, "waitReconnectSeconds")
	if err != nil {
		log.Error("get room section key waitReconnectSeconds  err ")
		return nil, err
	}

	serverConfig.SaveRoomDataInterval, err = LoadSecondFromIniSection(section, "saveRoomDataIntervalSeconds")
	if err != nil {
		log.Error("get room section key saveRoomDataIntervalSeconds  err ")
		return nil, err
	}

	client, err := NewNetworkAsServerFromIniFile(cfg, "client")
	if err != nil {
		log.Error("get room client  err ")
		return nil, err
	}

	serverConfig.Client = client

	section, err = cfg.GetSection("agent")
	if err != nil {
		log.Error("get room agent  err ")
		return nil, err
	}

	key, err = section.GetKey("enableLogSend")
	if err != nil {
		log.Error("get room agent key enableLogSend err ")
		return nil, err
	}
	serverConfig.Agent.EnableLogSend = key.MustBool(false)

	key, err = section.GetKey("enableLogRecv")
	if err != nil {
		log.Error("get room agent key enableLogRecv err ")
		return nil, err
	}
	serverConfig.Agent.EnableLogRecv = key.MustBool(false)

	key, err = section.GetKey("enableCheckPing")
	if err != nil {
		log.Error("get room agent key enableCheckPing err ")
		return nil, err
	}
	serverConfig.Agent.EnableCheckPing = key.MustBool(true)

	key, err = section.GetKey("enableCheckLoginParams")
	if err != nil {
		log.Error("get room agent key enableCheckLoginParams err ")
		return nil, err
	}
	serverConfig.Agent.EnableCheckLoginParams = key.MustBool(true)

	key, err = section.GetKey("enableCachedMsg")
	if err != nil {
		log.Error("get room agent key enableCachedMsg err ")
		return nil, err
	}
	serverConfig.Agent.EnableCachedMsg = key.MustBool(true)

	key, err = section.GetKey("enableCompressMsg")
	if err != nil {
		log.Error("get room agent key enableCompressMsg err ")
		return nil, err
	}
	serverConfig.Agent.EnableCompressMsg = key.MustBool(false)

	serverConfig.Agent.CachedMsgMaxCount, err = LoadPositiveIntFromIniSection(section, "cachedMsgMaxCount")
	if err != nil {
		log.Error("get room agent key cachedMsgMaxCount err ")
		return nil, err
	}

	serverConfig.Agent.CompressMsgSizeThreshold, err = LoadPositiveIntFromIniSection(section, "compressMsgSizeThreshold")
	if err != nil {
		log.Error("get room agent key compressMsgSizeThreshold err ")
		return nil, err
	}

	serverConfig.Agent.TimestampExpireDuration, err = LoadSecondFromIniSection(section, "timestampExpireSeconds")
	if err != nil {
		log.Error("get room agent key timestampExpireSeconds err ")
		return nil, err
	}

	serverConfig.UseMongo = false
	section, err = cfg.GetSection("mongo")
	if err == nil {
		key, err = section.GetKey("address")
		if err == nil {
			serverConfig.Mongo.Address = key.MustString("127.0.0.1:27017")

			key, err = section.GetKey("dbname")
			if err == nil {
				serverConfig.Mongo.Name = key.MustString("zhaohuanyuhecheng")
				serverConfig.UseMongo = true
			}
		}
	}


	serverConfig.UseRedis = false
	section, err = cfg.GetSection("redis")
	if err == nil {
		key, err = section.GetKey("address")
		if err == nil {
			serverConfig.Redis.Address = key.MustString("127.0.0.1:6377")
			key, err = section.GetKey("masterName")
			if err == nil {
				serverConfig.Redis.MasterName = key.MustString("")
				serverConfig.UseRedis = true
			}
		}
	}

	return serverConfig, nil
}
