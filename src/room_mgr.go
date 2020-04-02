package new_frame

import (
	"fmt"
	"new_frame/src/domain"
	"new_frame/src/lgames"
	"runtime"
	"time"
)

const (
	RoomMgrFrameInterval = time.Millisecond * 500
	LogRoomCountInterval = time.Second * 30
	RoomMgrFPS           = time.Second / RoomMgrFrameInterval
)

type RoomMgr struct {
	RoomServer      *RoomServer
	Log             *lgames.Logger
	MsgsFromAgent   chan Closure
	MsgsFromDBAgent chan Closure
	MsgsFromRoom    chan Closure

	GlobalConfig *GlobalConfig

	frameTimer            *time.Ticker
	id2Room               map[string]*Room
	innerId2Room          map[string]*Room
	innerId2RoomId        map[string]string
	roomId2InnerId        map[string]string
	frameID               int
	nextLogRoomCountFrame int
}

func NewRoomMgr(roomServer *RoomServer) *RoomMgr {
	globalConfig, err := NewGlobalConfig(roomServer.Log)
	if err != nil {
		roomServer.Log.Error("NewGlobalConfig err")
		return nil
	}

	roomMgr := &RoomMgr{
		RoomServer:      roomServer,
		Log:             roomServer.Log,
		MsgsFromAgent:   make(chan Closure, 16*1024),
		MsgsFromDBAgent: make(chan Closure, 16*1024),
		MsgsFromRoom:    make(chan Closure, 16*1024),
		GlobalConfig:    globalConfig,

		frameTimer:            time.NewTicker(RoomMgrFrameInterval),
		id2Room:               make(map[string]*Room, 0),
		frameID:               0,
		nextLogRoomCountFrame: 0,
	}

	return roomMgr
}

func (self *RoomMgr) Run() {
	defer func() {
		p := recover()
		if p != nil {
			self.Log.Info("execute panic recovered and going to stop: %v", p)
		}
	}()

	self.RoomServer.WaitGroup.Add(1)
	defer func() {
		self.RoomServer.WaitGroup.Done()
	}()

ALL:
	for {
		select {
		case c, ok := <-self.MsgsFromAgent:
			if !ok {
				break ALL
			}
			SafeRunClosure(self, c)
		case c := <-self.MsgsFromDBAgent:
			SafeRunClosure(self, c)
		case c := <-self.MsgsFromRoom:
			SafeRunClosure(self, c)
		case <-self.frameTimer.C:
			SafeRunClosure(self, func() {
				self.Frame()
			})
		}
	}

	self.OnQuit()
}

func (self *RoomMgr) SetNextFramId() {
	self.frameID++
	if self.frameID < 0 {
		self.frameID = 1
		self.nextLogRoomCountFrame = self.frameID + int(LogRoomCountInterval/RoomMgrFrameInterval)
	}
}

func (self *RoomMgr) SetLogRoomCountFrameId() {
	self.nextLogRoomCountFrame = self.frameID + int(LogRoomCountInterval/RoomMgrFrameInterval)
	if self.nextLogRoomCountFrame < 0 {
		self.frameID = 1
		self.nextLogRoomCountFrame = self.frameID + int(LogRoomCountInterval/RoomMgrFrameInterval)
	}
}

func (self *RoomMgr) Frame() {
	self.SetNextFramId()
	if self.frameID >= self.nextLogRoomCountFrame {
		self.SetLogRoomCountFrameId()
		memStats := new(runtime.MemStats)
		runtime.ReadMemStats(memStats)
		msg := fmt.Sprintf("room count=%d, goroutine count=%d, memory alloc=%.02fk", len(self.id2Room), runtime.NumGoroutine(), float32(memStats.Alloc)/1024)
		self.Log.Info("%s", msg)

		self.dbTest()
		self.redisTest()
	}

}

func (self *RoomMgr) MarkRoomEnd(roomId string) {
	_, ok := self.id2Room[roomId]
	if !ok {
		self.Log.Warn("error, roomId not found, %s", roomId)
		return
	}

	delete(self.id2Room, roomId)
	return
}

func (self *RoomMgr) GetAvaliableRoom(roomId string, reconnectFlag int) *Room {
	//先判断房间是否在使用中
	room, ok := self.id2Room[roomId]
	if ok {
		return room
	}

	//如果是重连， 并且房间不存在，需要结束
	if reconnectFlag != 0 {
		return nil
	}

	room, err := NewRoom(self, roomId, true)
	if err != nil {
		self.Log.Error("big err create room err %s", roomId)
		return nil
	}
	self.id2Room[roomId] = room

	go room.Run()
	return room
}

func (self *RoomMgr) Login(agent *Agent, roomId string, playerInfo *BasePlayerInfo, reconnectFlag int) {
	room := self.GetAvaliableRoom(roomId, reconnectFlag)
	if room == nil {
		self.Log.Warn("roomid %s end uid %s", roomId, playerInfo.Uid)
		self.OnLoginReply(false, "game_end", agent, nil)
		return
	}

	//直接进入房间
	RunOnRoom(room.GetChanForRoomMgr(), room, func(room *Room) {
		room.Login(agent, roomId, playerInfo, reconnectFlag)
	})

	return

}

func (self *RoomMgr) LostAgent(agent *Agent, roomId string) {
	self.Log.Info("LostAgent = %s uid %v", roomId, agent.Uid)

	room, ok := self.id2Room[roomId]
	if !ok {
		return
	}

	room.GetChanForRoomMgr() <- func() {
		room.LostAgent(agent)
	}

}

func (self *RoomMgr) OnLoginReply(result bool, reason string, agent *Agent, room *Room) {
	RunOnAgent(agent.MsgsFromRoomMgr, agent, func(agent *Agent) {
		agent.OnLoginReply(result, reason, room)
	})
}

func (self *RoomMgr) OnQuit() {
	// 通知所有room强制存储并退出
	for _, room := range self.id2Room {
		RunOnRoom(room.msgsFromMgr, room, func(input *Room) {
			input.SaveAndQuit()
		})
	}

	for len(self.id2Room) > 0 {
		c := <-self.MsgsFromRoom
		SafeRunClosure(self, c)
	}

}

func (self *RoomMgr) Quit() {
	close(self.MsgsFromAgent)
}

func (self *RoomMgr) dbTest(){
	_, err := self.dbFind("111111")
	if err != nil {
		self.Log.Error("GetPlayer err %v ", err)
	}

	player := domain.NewPlayer("2222222")
	err = self.dbUpset(player)
	if err != nil {
		self.Log.Debug("InsertPlayer err %v", err)
	} else {
		self.Log.Debug("insert success")
	}

	_, err = self.dbFind("2222222")
	if err != nil {
		self.Log.Error("GetPlayer err %v ", err)
	}
	self.Log.Info("find uid 2222222 success")
}


func (self *RoomMgr) dbFind(uid string) (*domain.Player, error){
	player, err := self.RoomServer.MongoDao.GetPlayer(uid)
	if err != nil {
		self.Log.Error("GetPlayer err %v ", err)
		return nil, err
	}

	self.Log.Info("dbFind uid %v ", player.Uid)
	return player, nil
}

func (self *RoomMgr) dbUpset(player *domain.Player) error{
	err := self.RoomServer.MongoDao.UpdatePlayer(player)
	if err != nil {
		self.Log.Error("GetPlayer err %v ", err)
		return err
	}

	self.Log.Info(" dbUpset uid %v ", player.Uid)
	return nil
}


func (self *RoomMgr) dbDelete(uid string ) error{
	err := self.RoomServer.MongoDao.DeletePlayer(uid)
	if err != nil {
		self.Log.Error("GetPlayer err %v ", err)
	}

	self.Log.Info(" dbDelete uid %v ", uid)
	return nil
}


func (self *RoomMgr) redisTest(){
	self.Log.Info("test 11111 %v ", self.RoomServer.Config.UseRedis)
	value , err := self.RoomServer.RedisDao.Get("111111")
	if err != nil {
		self.Log.Info(" redisTest get 111111 err %v ", err)
	}

	self.RoomServer.RedisDao.Set("222222", "3333333")
	value , err = self.RoomServer.RedisDao.Get("222222")
	if err != nil {
		self.Log.Info(" redisTest get 222222 err %v ", err)
	} else {
		self.Log.Info(" redisTest get 222222 successs %v ", B2S(value.([]uint8)))
	}

}
