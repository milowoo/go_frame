package new_frame

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"runtime/debug"
	"new_frame/src/dao/mongo"
	"new_frame/src/domain"
	"new_frame/src/lgames"
	"new_frame/src/pb"
	"time"

	"github.com/golang/protobuf/proto"
)

const (
	ROOM_FPS            = 8
	ROOM_FRAME_INTERVAL = time.Second / ROOM_FPS

	ROOM_WAITING_PLAYERS_READY_TIME = time.Second * 20
)

const (
	ROOM_STATE_UNKNOWN               = "ROOM_STATE_UNKNOWN"
	ROOM_STATE_WAITING_PLAYERS_READY = "ROOM_STATE_WAITING_PLAYERS_READY"
	ROOM_STATE_RUNNING               = "ROOM_STATE_RUNNING"
	ROOM_STATE_END                   = "ROOM_STATE_END"
)

type Room struct {
	GlobalConfig *GlobalConfig
	ServerConfg  *RoomServerConfig
	MongoDao     *mongo.MongoDAO

	roomMgr *RoomMgr
	Log     *lgames.Logger

	FrameID        int
	gameRunFrameId int //游戏运行的帧数
	offLineFrameId int //掉线的帧数
	beginFramId    int //开始计时的帧数,用于计算每走一步棋的时间

	uid2PlayerInfo   map[string]*PlayerInfo
	agent2PlayerInfo map[*Agent]*PlayerInfo
	playerInfoList   []*PlayerInfo
	frameTicker      *time.Ticker
	msgsFromAgent    chan Closure
	msgsFromMgr      chan Closure
	rand             *rand.Rand
	protocol2Method  map[string]reflect.Value

	state             string
	roomId            string
	stateTimeoutFrame int
	TotalPoints       int
	IsRobot           bool
	RobotUid          string //AI的UID
	WinUid            string //赢棋的UID

	exit chan bool
}

func NewRoom(roomMgr *RoomMgr, roomId string, reclaim bool) (*Room, error) {
	if roomMgr == nil {
		return nil, fmt.Errorf("RoomMgr is nil")
	}

	timer := time.NewTimer(0)
	timer.Stop()

	now := time.Now()
	self := &Room{
		roomMgr:        roomMgr,
		Log:            roomMgr.Log,
		MongoDao:       g_roomServer.MongoDao,
		FrameID:        0,
		gameRunFrameId: 0,
		offLineFrameId: 0,
		beginFramId:    0,

		// 登录成功，则会重置为0；登录失败则一段时间后自动结束房间。+5s是为了防止WaitReconnectDuration配置成0时，创建房间就自动结束了
		uid2PlayerInfo:    make(map[string]*PlayerInfo),
		agent2PlayerInfo:  make(map[*Agent]*PlayerInfo),
		playerInfoList:    make([]*PlayerInfo, 0, 2),
		frameTicker:       nil,
		msgsFromAgent:     make(chan Closure, 2*1024),
		msgsFromMgr:       make(chan Closure, 512),
		rand:              rand.New(rand.NewSource(now.Unix())),
		state:             ROOM_STATE_UNKNOWN,
		roomId:            roomId,
		stateTimeoutFrame: 0,
		TotalPoints:       0,
		RobotUid:          "",
		WinUid:            "",

		IsRobot: false,
		exit:    make(chan bool, 1),
	}

	self.GlobalConfig = roomMgr.GlobalConfig
	self.ServerConfg = roomMgr.RoomServer.Config

	t := reflect.ValueOf(self)

	protocol2Method := make(map[string]reflect.Value)
	for p, v := range validRoomProtocols {
		m := t.MethodByName(v)
		if !m.IsValid() {
			self.Log.Error("error, protocol handler not found, %s", p)
		}
		protocol2Method[p] = m
	}
	self.protocol2Method = protocol2Method

	return self, nil
}

func (self *Room) ApplyProtoHandler(agent *Agent, p string, msg proto.Message) {
	method, ok := self.protocol2Method[p]
	if !ok {
		self.Log.Info("method not found: %s", p)
		return
	}

	playerInfo := self.agent2PlayerInfo[agent]
	if playerInfo == nil {
		self.Log.Error("error: playerInfo not found, %s, %p", p, agent)
		return
	}

	in := []reflect.Value{
		playerInfo.Value,
		reflect.ValueOf(msg),
	}
	method.Call(in)
}

func (self *Room) GetChanForAgent() chan Closure {
	return self.msgsFromAgent
}

func (self *Room) GetChanForRoomMgr() chan Closure {
	return self.msgsFromMgr
}

func (self *Room) setState(nextState string) {
	self.Log.Info("roomid %s %s -> %s", self.roomId, self.state, nextState)

	self.state = nextState
}

func (self *Room) Run() {
	defer func() {
		p := recover()
		if p != nil {
			self.Log.Info("execute panic recovered and going to stop: %v", p)
		}
	}()

	self.frameTicker = time.NewTicker(ROOM_FRAME_INTERVAL)

	self.setState(ROOM_STATE_WAITING_PLAYERS_READY)
	self.stateTimeoutFrame = self.FrameID + int(ROOM_WAITING_PLAYERS_READY_TIME/ROOM_FRAME_INTERVAL)

	defer func() {
		self.frameTicker.Stop()
	}()

	for {
		// 优先查看exit，
		select {
		case <-self.exit:
			return
		default:
			// do nothing
		}

		select {
		case c := <-self.msgsFromAgent:
			SafeRunClosure(self, c)
		case c := <-self.msgsFromMgr:
			SafeRunClosure(self, c)
		case <-self.frameTicker.C:
			SafeRunClosure(self, func() {
				self.Frame()
				for _, playerInfo := range self.uid2PlayerInfo {
					playerInfo.FlushCachedMsgs()
				}
			})
		}
	}
}

func (self *Room) Frame() {
	self.FrameID++

	if self.gameRunFrameId != 0 {
		self.gameRunFrameId++
	}

	if self.state == ROOM_STATE_WAITING_PLAYERS_READY {
		if self.FrameID >= self.stateTimeoutFrame {
			self.Log.Info("roomId %v waiting ready timeout", self.roomId)
			gameResult := self.getGameResult(WIN_TYPE_WAITING_TIMEOUT)
			self.enterEndState(gameResult, GAME_END_REASON_WAITING_TIME_OUT)
			return
		}
	}

	//判断玩家的总用时是否超时
	if self.checkTotalUseTimeOver() {
		self.Log.Info("roomId %v checkTotalUseTimeOver", self.roomId)
		gameResult := self.getGameResult(WIN_TYPE_USE_TIME_OVER)
		self.enterEndState(gameResult, GAME_END_TOTAL_TIME_OVER)
		return
	}

	//判断是否断线超时
	if self.offLineFrameId > 0 && (self.gameRunFrameId-self.offLineFrameId)/ROOM_FPS > self.GlobalConfig.ReconnectTime {
		self.Log.Info("roomId %v offline end ", self.roomId)
		gameResult := self.getGameResult(WIND_TYPE_OFFLINE)
		self.enterEndState(gameResult, GAME_END_OFFLINE)
		return
	}

	//test need
	if self.FrameID != 0 {
		//一分钟记录一条记录
		if self.FrameID%(ROOM_FPS*60*1) == 0 {
			Id := "1111111"
			if self.gameRunFrameId < 80 {
				Id = "10000000"
			}
			player := domain.NewPlayer(Id)

			err := self.MongoDao.InsertPlayer(player)
			if err != nil {
				self.Log.Debug("InsertPlayer err %v", err)
			} else {
				self.Log.Debug("insert success")
			}

		}

		//两分钟更新一条记录
		if self.FrameID%(ROOM_FPS*120*1+1) == 0 {
			player, err := self.MongoDao.GetPlayer("9347563")
			if err != nil {
				self.Log.Debug("GetPlayer err %v ", err)
			} else if player.Uid == "0" {
				self.Log.Debug("GetPlayer no found  ")
			} else {
				self.Log.Debug("GetPlayer success uid %v", player.Uid)
				player.Sex = 2
				player.Url = "test22222"
				err = self.MongoDao.UpdatePlayer(player)
				if err != nil {
					self.Log.Debug("UpdatePlayer err %v ", err)
				} else {
					self.Log.Debug("UpdatePlayer success")
				}
			}

			//diffTime := int64(60 * 60 * 24)
			//players, err := self.MongoDao.GetPlayerByTime(time.Now().Unix() - diffTime)
			//if err == nil {
			//	for _, player := range players {
			//		self.Log.Debug("uid %v url %v", player.Uid, player.Url)
			//	}
			//}
		}
	}
}

func (self *Room) enterEndState(gameResult *GameResult, reason string) {
	self.setState(ROOM_STATE_END)

	buf, _ := json.Marshal(gameResult)

	pbMsg := &pb.S2CGameEnd{
		Result: string(buf),
		Scores: gameResult.Result.Jifen,
		Users:  gameResult.Result.Users,
		Msg:    reason,
		Winner: self.WinUid,
	}

	go self.UploadResult(gameResult)

	self.Send(pbMsg)

	self.onRoomEnd()
}

// 上报游戏结果
func (self *Room) UploadResult(gameResult *GameResult) {
	defer func() {
		if err := recover(); err != nil {
			self.Log.Error("%+v: %s", err, debug.Stack())
		}
	}()

	if len(self.GlobalConfig.UploadUrl) < 5 {
		return
	}

	buf, _ := json.Marshal(gameResult)
	params := map[string]interface{}{
		"roomId": self.roomId,
		"gameId": self.ServerConfg.GameID,
		"result": string(buf),
	}

	HttpPost(self.GlobalConfig.UploadUrl, params, nil, self.Log)
}

func (self *Room) getReadyPlayerCount() int {
	readyPlayerCount := 0
	for _, playerInfo := range self.playerInfoList {
		if playerInfo.Agent != nil && playerInfo.State == PLAYER_STATE_READY {
			readyPlayerCount++
		}

		if playerInfo.IsRobot {
			readyPlayerCount++
		}
	}
	return readyPlayerCount
}

func (self *Room) addPlayer(playerInfo *PlayerInfo) {
	uid := playerInfo.Base.Uid

	self.uid2PlayerInfo[uid] = playerInfo
	if playerInfo.Agent != nil {
		self.agent2PlayerInfo[playerInfo.Agent] = playerInfo
	}

	self.playerInfoList = append(self.playerInfoList, playerInfo)

}

// TODO: 应当有个相应的removePlayer
func (self *Room) innerLogin(agent *Agent, playerInfo *BasePlayerInfo, reconnectFlag int) (bool, string) {
	self.Log.Info(" innerLogin  uid %v roomid %v", playerInfo.Uid, self.roomId)

	// 处理附带的AI属性信息
	if reconnectFlag != 2 {
		self.parseOpt(playerInfo.Opt)
	}
	self.offLineFrameId = 0

	innerPlayerInfo := self.uid2PlayerInfo[playerInfo.Uid]
	if self.state == ROOM_STATE_WAITING_PLAYERS_READY {
		if innerPlayerInfo == nil {
			playerCount := len(self.playerInfoList)
			if playerCount >= self.ServerConfg.MaxPlayerInRoom {
				return false, "full of players"
			}
			newPlayerInfo := NewPlayer(playerInfo, agent, reconnectFlag, false)
			newPlayerInfo.ResetLoadingState()

			self.addPlayer(newPlayerInfo)
			// 广播玩家列表
			self.sendPlayersInfo("")
			return true, "OK"
		}

		innerPlayerInfo.ConnectFlag = reconnectFlag
		if innerPlayerInfo.Agent == agent {
			return true, "OK"
		}

		self.ReplaceAgent(innerPlayerInfo, agent)
		self.BroadcastOnlineInfo()
		self.sendPlayersInfo(playerInfo.Uid)

		return true, "OK"
	}

	if self.state == ROOM_STATE_RUNNING {
		if innerPlayerInfo == nil {
			return false, "running room reject new player"
		}

		innerPlayerInfo.ConnectFlag = reconnectFlag
		if innerPlayerInfo.Agent == agent {
			return true, "OK"
		}

		//通知前一个登录断线，给新登录的发信息
		self.ReplaceAgent(innerPlayerInfo, agent)
		self.BroadcastOnlineInfo()
		self.sendPlayersInfo(playerInfo.Uid)

		return true, "OK"
	}

	return false, "room ended"
}

//BroadcastOnlineInfo 广播在线信息
func (self *Room) BroadcastOnlineInfo() {
	proto := &pb.S2COnline{}

	for _, playerInfo := range self.uid2PlayerInfo {
		if playerInfo.Agent != nil || playerInfo.IsRobot {
			proto.Uids = append(proto.Uids, &pb.OnlineInfo{Uid: playerInfo.Base.Uid, Online: true})
		} else {
			proto.Uids = append(proto.Uids, &pb.OnlineInfo{Uid: playerInfo.Base.Uid, Online: false})
		}
	}
	self.Send(proto)
}

// 单播
func (self *Room) SendToPlayer(uid string, msg proto.Message) {
	if player, ok := self.uid2PlayerInfo[uid]; ok {
		if player.IsRobot {
			return
		}

		player.Send(msg)
	}
}

func (self *Room) sendPlayersInfo(toUid string) {
	if len(self.playerInfoList) < g_roomServer.Config.MaxPlayerInRoom {
		return
	}

	playersInfos := &pb.S2CPlayerInfo{}
	for _, playerInfo := range self.uid2PlayerInfo {
		playersInfos.Players = append(playersInfos.Players, &pb.PlayerInfo{
			Uid:       playerInfo.Base.Uid,
			Name:      playerInfo.Base.Name,
			Avatarurl: playerInfo.Base.AvatarUrl,
			Teamid:    playerInfo.Base.TeamId,
			Opt:       playerInfo.Base.Opt,
			Sex:       int32(playerInfo.Base.Sex),
			Ai:        playerInfo.IsRobot,
		})
	}
	if toUid == "" {
		self.Send(playersInfos)
	} else {
		self.SendToPlayer(toUid, playersInfos)
	}

}

func (self *Room) ReplaceAgent(player *PlayerInfo, newAgent *Agent) {
	// 先断开旧的
	oldAgent := player.Agent
	if oldAgent != nil {
		RunOnAgent(oldAgent.MsgsFromRoom, oldAgent, func(agent *Agent) {
			agent.DelayDisconnect(time.Second * 5)
		})
	}
	delete(self.agent2PlayerInfo, oldAgent)

	// 补上新的
	player.ResetLoadingState()
	player.Agent = newAgent
	self.agent2PlayerInfo[newAgent] = player
}

func (self *Room) ready(playerInfo *PlayerInfo) {
	playerInfo.State = PLAYER_STATE_READY
	if self.getReadyPlayerCount() < self.ServerConfg.MaxPlayerInRoom {
		return
	}

	//游戏开始
	self.gameRunFrameId = 1
	self.Log.Debug("roomId %s game ready to begin uid %s ", self.roomId, playerInfo.Base.Uid)

	self.setState(ROOM_STATE_RUNNING)
	self.initData(playerInfo)
}

func (self *Room) Login(agent *Agent, roomId string, playerInfo *BasePlayerInfo, reconnectFlag int) {
	if self.roomId != roomId {
		self.roomId = roomId
	}

	result, reason := self.innerLogin(agent, playerInfo, reconnectFlag)
	RunOnRoomMgr(self.roomMgr.MsgsFromRoom, self.roomMgr, func(roomMgr *RoomMgr) {
		roomMgr.OnLoginReply(result, reason, agent, self)
	})
}

func (self *Room) getGameResult(WindType int) *GameResult {
	users := self.getUseIds()
	winners := self.getWinnerUid(WindType)

	timestamp := time.Now().Unix()
	nonStr := RandomString(self.rand, 5, 9)
	jifen := make([]string, 0)

	result := &InnerGameResult{
		GameId:     self.ServerConfg.GameID,
		RoomId:     self.roomId,
		ChannelId:  self.ServerConfg.ChannelID,
		ResultType: self.getResultType(WindType),
		Users:      users,
		Jifen:      jifen,
		Winners:    winners,
		GameSpec:   self.getResultPoint(),
	}

	resultJsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil
	}

	resultJson := string(resultJsonBytes)

	sign := GetSha1HexSign(timestamp, nonStr, resultJson, self.ServerConfg.AppKey)
	return &GameResult{
		Timestamp:     timestamp,
		NonStr:        nonStr,
		Sign:          sign,
		ResultRawData: resultJson,
		Result:        result,
		GameType:      "1v1_pk",
	}
}

func (self *Room) RoomMgrRun(cb func(roomMgr *RoomMgr)) {
	roomMgr := self.roomMgr
	roomMgr.MsgsFromRoom <- func() {
		cb(roomMgr)
	}
}

func (self *Room) onRoomEnd() {
	self.Log.Info("roomid %s end", self.roomId)
	for _, playerInfo := range self.uid2PlayerInfo {
		if playerInfo != nil {
			playerInfo.FlushCachedMsgs()
		}
	}

	for _, playerInfo := range self.playerInfoList {
		playerInfo.Run(func(agent *Agent) {
			agent.DelayDisconnect(time.Second * 5)
		})
	}

	self.RoomMgrRun(func(roomMgr *RoomMgr) {
		roomMgr.MarkRoomEnd(self.roomId)
	})

	self.exit <- true
}

//Rebroadcast 转播给非uid的玩家
func (self *Room) TransToPlayer(uid string, msg proto.Message) {
	binary, err := GetBinary(msg, self.Log)
	if err != nil {
		return
	}

	for _, playerInfo := range self.uid2PlayerInfo {
		if uid == playerInfo.Base.Uid {
			continue
		}

		playerInfo.SendBin(binary)
	}
}

//广播给房间的玩家
func (self *Room) Send(msg proto.Message) {
	binary, err := GetBinary(msg, self.Log)
	if err != nil {
		return
	}

	for _, player := range self.uid2PlayerInfo {
		if player.IsRobot {
			continue
		}
		player.SendBin(binary)
	}
}

func (self *Room) initData(playerInfo *PlayerInfo) {
	self.beginFramId = self.gameRunFrameId
	for _, player := range self.uid2PlayerInfo {
		player.SyncData = true
		self.beginFramId = self.gameRunFrameId

		if player.ConnectFlag == 0 || player.ConnectFlag == 1 {
			self.SyncRoomInfo(player)
		} else {
			self.ReSyncRoomInfo(player)
		}
	}

	self.beginFramId = self.gameRunFrameId
}

func (self *Room) LostAgent(agent *Agent) {
	self.Log.Info("LostAgent uid %s ", agent.Uid)

	playerInfo := self.agent2PlayerInfo[agent]
	if playerInfo == nil {
		self.Log.Warn("error: playerInfo not found, %s", agent.Uid)
		if len(self.agent2PlayerInfo) == 0 && !self.IsRobot {
			self.WinUid = self.RobotUid
			gameResult := self.getGameResult(WIND_TYPE_OFFLINE)
			self.enterEndState(gameResult, GAME_END_OFFLINE)
		}

		return
	}

	self.offLineFrameId = self.gameRunFrameId
	playerInfo.Agent = nil
	delete(self.agent2PlayerInfo, agent)

	if len(self.agent2PlayerInfo) == 0 && !self.IsRobot {
		//两个人同时下线， 需要房间关闭，游戏结束
		self.WinUid = self.RobotUid
		gameResult := self.getGameResult(WIND_TYPE_OFFLINE)
		self.enterEndState(gameResult, GAME_END_OFFLINE)
	} else {
		//通知对方离线
		self.BroadcastOnlineInfo()
	}
}

func (self *Room) CanHandleProtocol(protocol string) bool {
	_, ok := self.protocol2Method[protocol]
	return ok
}

func (self *Room) SaveAndQuit() {
	self.onRoomEnd()
}

// 房间请求响应接口
func (self *Room) FunctionReply(uid string, msg proto.Message) {
	self.SendToPlayer(uid, msg)
}

const (
	WIN_TYPE_WAITING_TIMEOUT = 0 //等待超时
	WIN_TYPE_DRAW            = 1 //平局
	WIN_TYPE_BATTLE_WIN      = 2 //赢了对方
	WIN_TYPE_USE_TIME_OVER   = 3 //使用时间超限
	WIND_TYPE_OFFLINE        = 4 //对方断线
)

func (self *Room) getUseIds() []string {
	users := make([]string, 0)
	for uid, _ := range self.uid2PlayerInfo {
		users = append(users, uid)
	}

	return users
}

func (self *Room) getWinnerUid(WinType int) []string {
	users := make([]string, 0)
	switch WinType {
	case WIN_TYPE_WAITING_TIMEOUT:
		for uid, player := range self.uid2PlayerInfo {
			if player.State == PLAYER_STATE_READY {
				users = append(users, uid)
				break
			}
		}
	case WIN_TYPE_BATTLE_WIN, WIND_TYPE_OFFLINE, WIN_TYPE_USE_TIME_OVER:
		users = append(users, self.WinUid)
	case WIN_TYPE_DRAW:
		for uid, _ := range self.uid2PlayerInfo {
			users = append(users, uid)
		}

	}

	return users
}

func (self *Room) getOppUid(uid string) string {
	for _, player := range self.uid2PlayerInfo {
		if player.Base.Uid == uid {
			continue
		}

		return player.Base.Uid
	}

	return ""
}

func (self *Room) getOppPlayer(uid string) *PlayerInfo {
	for _, player := range self.uid2PlayerInfo {
		if player.Base.Uid == uid {
			continue
		}

		return player
	}

	return nil
}

func (self *Room) C2SLoading(player *PlayerInfo, msg *pb.C2SLoading) {
	progress := int(msg.Progress)

	if player.State != PLAYER_STATE_LOADING {
		self.Log.Error("uid %v error: player state is not loading, %p", player.Base.Uid, player.Agent)
		return
	}

	progress = Clamp(progress, 0, 100)
	player.LoadingProgress = progress

	if self.state == ROOM_STATE_WAITING_PLAYERS_READY {
		self.TransToPlayer(player.Base.Uid, &pb.S2COpponentLoading{Progress: int32(progress)})

		if progress >= 100 {
			self.ready(player)
		}
	} else if self.state == ROOM_STATE_RUNNING {
		if progress < 100 {
			return
		}

		player.State = PLAYER_STATE_READY
		self.ReSyncRoomInfo(player)
	}

	return

}
