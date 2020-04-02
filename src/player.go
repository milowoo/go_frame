package new_frame

import (
	"reflect"

	"github.com/golang/protobuf/proto"
)

const (
	PLAYER_STATE_LOADING = 0 // 加载资源阶段
	PLAYER_STATE_READY   = 1 // 准备就绪阶段
	PLAYER_STATE_LOST    = 2 // 连接丢失
)

type PlayerInfo struct {
	Base  BasePlayerInfo
	Agent *Agent
	Value reflect.Value

	ConnectFlag int

	State            int
	LoadingProgress  int
	TotalUseTime     int
	IsRobot          bool
	GoldEndNotice    bool
	ShowBeginFrameId int
	WhiteGold        int
	BlackGold        int
	RedGold          int
	RedCount         bool //红球是否需要计算分数
	SyncData         bool
	QueenInPocket    int32
	NoStrikeRound    int
}

func NewPlayer(base *BasePlayerInfo, agent *Agent, reconnectFlag int, IsRobot bool) *PlayerInfo {
	p := &PlayerInfo{
		Base:        *base,
		Agent:       agent,
		ConnectFlag: reconnectFlag,

		IsRobot:          IsRobot,
		TotalUseTime:     0,
		LoadingProgress:  0,
		GoldEndNotice:    false,
		ShowBeginFrameId: 0,
		WhiteGold:        0,
		BlackGold:        0,
		RedGold:          0,
		RedCount:         false,
		SyncData:         false,
		QueenInPocket:    0,
		NoStrikeRound:    0,
	}
	p.Value = reflect.ValueOf(p)

	return p
}

func (self *PlayerInfo) Send(msg proto.Message) {
	agent := self.Agent
	if agent == nil {
		return
	}

	binary, err := GetBinary(msg, agent.Log)
	if err != nil {
		return
	}

	agent.MsgsFromRoom <- func() { agent.SendBinary(binary) }
}

func (self *PlayerInfo) Run(cb func(inputAgent *Agent)) {
	agent := self.Agent
	if agent == nil {
		return
	}

	agent.MsgsFromRoom <- func() { cb(agent) }
}

// 发送pb二进制编码
func (self *PlayerInfo) SendBin(msg []byte) {
	agent := self.Agent
	if agent == nil {
		return
	}

	agent.MsgsFromRoom <- func() { agent.SendBinary(msg) }
}

func (self *PlayerInfo) IsReady() bool {
	return self.State == PLAYER_STATE_READY //只有ready才是准备好的
}

func (self *PlayerInfo) ResetLoadingState() {
	self.LoadingProgress = 0
	self.State = PLAYER_STATE_LOADING
}

func (self *PlayerInfo) FlushCachedMsgs() {
	agent := self.Agent
	if agent == nil {
		return
	}

	agent.MsgsFromRoom <- func() { agent.FlushCachedMsgs() }
}
