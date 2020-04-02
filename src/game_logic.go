package new_frame

import (
	"encoding/json"
	"fmt"
	"new_frame/src/pb"
)

const (
	GAME_END_REASON_WAITING_TIME_OUT  = "ROOM_WAITING_READY_TIMEOUT"        // 等待超时
	GAME_END_BATTLE_WIND              = "BATTLE_WIND"                       //赢
	GAME_END_DRAW                     = "GAME_DRAW"                         //平局
	GAME_END_OFFLINE                  = "OFFLINE"                           //对方断线
	GAME_END_TOTAL_TIME_OVER          = "TOTAL_TIME_OVER"                   //用时完
	GAME_END_OPP_NO_STRIKE_OVER_ROUND = "GAME_END_OPP_NO_STRIKE_OVER_ROUND" //对方连续不击球
)

var validRoomProtocols = map[string]string{
	"pb.c2sLoading": "C2SLoading",

	// 添加Room允许client访问的成员函数名
}

type AiInfo struct {
	Uid       string `json:"uid,omitempty"`
	Nick      string `json:"nick,omitempty"`
	Avatarurl string `json:"avatarurl,omitempty"`
}

type OptData struct {
	Ai     int     `json:"ai,omitempty"`
	Level  int     `json:"level,omitempty"`
	AiInfo *AiInfo `json:"ai_info,omitempty"`
}

func (self *Room) parseOpt(opt string) {
	if len(opt) == 0 {
		return
	}

	optData := new(OptData)
	if err := json.Unmarshal([]byte(opt), optData); err != nil {
		return
	}

	// 非ai信息
	if optData.Ai == 0 {
		return
	}

	// 增加ai用户信息
	if optData.AiInfo == nil {
		return
	}

	//防止重复处理AI
	self.RobotUid = optData.AiInfo.Uid
	robotPlayer := self.uid2PlayerInfo[self.RobotUid]
	if robotPlayer != nil {
		return
	}

	baseInfo := &BasePlayerInfo{
		Uid:       optData.AiInfo.Uid,
		Name:      optData.AiInfo.Nick,
		AvatarUrl: optData.AiInfo.Avatarurl,
	}

	newPlayerInfo := NewPlayer(baseInfo, nil, 0, true)
	self.addPlayer(newPlayerInfo)
	self.IsRobot = true
}

func (self *Room) getResultType(WindType int) string {
	switch WindType {
	case WIN_TYPE_WAITING_TIMEOUT:
		return "notstart"
	case WIN_TYPE_BATTLE_WIN, WIND_TYPE_OFFLINE:
		return "not_draw"
	case WIN_TYPE_DRAW, WIN_TYPE_USE_TIME_OVER:
		return "draw"
	default:
		return "not_draw"
	}
}

func (self *Room) getResultPoint() string {
	pointsInfo := ""
	for _, player := range self.uid2PlayerInfo {
		point := 0
		str := fmt.Sprintf("%s:%d", player.Base.Uid, point)
		if pointsInfo == "" {
			pointsInfo = str
		} else {
			pointsInfo += ";"
			pointsInfo += str
		}
	}
	return pointsInfo
}

func (self *Room) SyncRoomInfo(playerInfo *PlayerInfo) {
	uid := playerInfo.Base.Uid

	//下发配置信息
	cfgProto := &pb.S2CParams{
		MaxGameTimeSecs:   int32(self.GlobalConfig.TotalUseTime),
	}
	self.SendToPlayer(uid, cfgProto)

}

func (self *Room) ReSyncRoomInfo(playerInfo *PlayerInfo) {

}

func (self *Room) checkTotalUseTimeOver() bool {
	if self.gameRunFrameId%6 != 0 && self.gameRunFrameId != 0 {
		return false
	}

	if self.gameRunFrameId/ROOM_FPS >= self.GlobalConfig.TotalUseTime {
		self.WinUid = ""
		return true
	}

	return false
}

func (self *Room) checkWin() {
	//for _, player := range self.uid2PlayerInfo {
	//	point := 0
	//	if point >= self.GlobalConfig.WinPoints {
	//		self.WinUid = player.Base.Uid
	//		gameResult := self.getGameResult(WIN_TYPE_BATTLE_WIN)
	//		self.enterEndState(gameResult, GAME_END_BATTLE_WIND)
	//		return
	//	}
	//}
}
