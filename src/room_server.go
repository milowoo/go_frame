package new_frame

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"new_frame/src/dao/mongo"
	"new_frame/src/dao/redis"
	"new_frame/src/lgames"

	"github.com/gorilla/websocket"
)

type RoomServer struct {
	Log       *lgames.Logger
	Config    *RoomServerConfig
	RoomMgr   *RoomMgr
	WaitGroup *sync.WaitGroup

	MongoDao *mongo.MongoDAO
	RedisDao *redis.RedisDao

	svr    *http.Server
	isQuit bool
}

var g_roomServer *RoomServer

func NewRoomServer(log *lgames.Logger) (*RoomServer, error) {
	config, err := NewRoomServerConfig(log)
	if err != nil {
		log.Error("NewRoomServerConfig err")
		return nil, err
	}

	g_roomServer = &RoomServer{
		Config:    config,
		WaitGroup: &sync.WaitGroup{},
		svr:       nil,
		Log:       log,
	}
	g_roomServer.WaitGroup.Add(1) // 对应Quit中的Done

	roomMgr := NewRoomMgr(g_roomServer)
	g_roomServer.RoomMgr = roomMgr
	if config.UseMongo{
		log.Info("mongo address %v name %v", config.Mongo.Address, config.Mongo.Name)
		databaseDAO , err := mongo.NewDataSource(config.Mongo.Address, config.Mongo.Name, log)
		if err != nil {
			log.Error("NewDataSource err %v", err)
			return nil, err
		}

		newMongoDao := mongo.NewMongoDao(databaseDAO, log)
		g_roomServer.MongoDao = newMongoDao
	}

	if config.UseRedis{
		log.Info("redis address %v masterName %v", config.Redis.Address, config.Redis.MasterName)
		redisDao  := redis.NewRedis(config.Redis.Address, config.Redis.MasterName, log)
		g_roomServer.RedisDao = redisDao
	}

	g_roomServer.ListenAndServe()

	return g_roomServer, nil
}

var UPGRADER = websocket.Upgrader{
	ReadBufferSize:  10 * 1024,
	WriteBufferSize: 10 * 1024,
}

// 通知服务器退出

func (self *RoomServer) Quit() {
	if self.isQuit {
		return
	}

	self.isQuit = true

	// 关闭agent监听，不再允许进入新连接
	self.StopWebsocketServer()

	// 通知所有RoomMgr强制存储，并退出；查询中的agent直接通知服务器关闭
	self.RoomMgr.Quit()

	self.WaitGroup.Done()
}

func (self *RoomServer) StopWebsocketServer() {
	if self.svr != nil {
		ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
		self.svr.Shutdown(ctx)
	}
}

func (self *RoomServer) Join() {
	self.WaitGroup.Wait()
}

func (self *RoomServer) ListenAndServe() {
	if self.svr != nil {
		self.Log.Error("error: ListenAndServe() repeatedly")
		return
	}

	UPGRADER.CheckOrigin = func(r *http.Request) bool {
		return true
	}

	clientConfig := self.Config.Client
	addr := fmt.Sprintf("%s:%d", clientConfig.ListenIp, clientConfig.ListenPort)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawConn, err := UPGRADER.Upgrade(w, r, nil)
		if err != nil {
			self.Log.Error("UPGRADER.Upgrade(): %+v", err)
			return
		}

		conn := NewAgent(rawConn, self.RoomMgr, r.URL.Query())
		go conn.Run()
	})
	self.svr = &http.Server{Addr: addr, Handler: handler}
	go func() {
		self.WaitGroup.Add(1) // 对应本携程
		defer self.WaitGroup.Done()

		self.Log.Info("%s\tlisten on\t%s", self.Config.GameID, addr)

		err := self.svr.ListenAndServe()
		if err != nil {
			self.Log.Error("error, ListenAndServe: %+v", err)
		}

		self.svr = nil
	}()
}

func (self *RoomServer) Run() {
	if self.Config == nil {
		self.Log.Error("Config is nil svr ...")
		self.Quit()
	}

	if self.RoomMgr == nil {
		self.Log.Error("RoomMgr is nil svr ...")
		self.Quit()
	}

	go self.RoomMgr.Run()

	// wait exit
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		<-c
		self.Log.Warn("exit svr by signal ...")
		self.Quit()
	}()

	self.Join()
}
