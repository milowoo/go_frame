package new_frame

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"new_frame/src/lgames"
	"new_frame/src/pb"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/gorilla/websocket"
)

const (
	// Maximum message size allowed from peer.
	MaxMsgSize         = 64 * 1024
	ClientPingInterval = time.Millisecond * 2500
	ClientPingTimeout  = ClientPingInterval * 2

	AgentFPS           = 20
	AgentFrameInterval = time.Second / AgentFPS

	SizeBytes = 0 // 对于websocket，没有sizebyte也ok
	MultiFlag = "pb.multi"
)

type Closure = func()

type SendEvent struct {
	t   int
	msg []byte
}

func NewSendTextEvent(msg string) *SendEvent {
	return &SendEvent{
		t:   websocket.TextMessage,
		msg: []byte(msg),
	}
}

func NewSendBinaryEvent(msg []byte) *SendEvent {
	return &SendEvent{
		t:   websocket.BinaryMessage,
		msg: msg,
	}
}

type Agent struct {
	Conn                 *websocket.Conn
	SendQueue            chan *SendEvent
	RecvQueue            chan Closure
	MsgsFromRoomMgr      chan Closure
	Closed               bool
	LastError            error
	RoomMgr              *RoomMgr
	Room                 *Room
	MsgsFromRoom         chan Closure
	LastPingTimeFrame    int
	PostData             *PostData
	protocol2Method      map[string]reflect.Value
	FrameTicker          *time.Ticker
	FrameID              int
	NextCheckPingFrame   int
	IsDisconnected       bool
	delayDisconnectTimer *time.Timer
	CachedMsgs           [][]byte
	Log                  *lgames.Logger
	Uid                  string
	RoomId               string
}

var validAgentProtocols = map[string]string{
	"pb.c2sHeart": "C2SHeart",
	// 添加agent允许client访问的成员函数名
}

//NewAgent new agent
func NewAgent(rawConn *websocket.Conn, roomMgr *RoomMgr, values url.Values) *Agent {
	timer := time.NewTimer(time.Hour)
	timer.Stop()

	log := roomMgr.Log
	self := &Agent{
		Conn:                 rawConn,
		SendQueue:            make(chan *SendEvent, 2*1024),
		RecvQueue:            make(chan Closure, 2*1024),
		MsgsFromRoomMgr:      make(chan Closure, 512),
		RoomMgr:              roomMgr,
		MsgsFromRoom:         make(chan Closure, 2*1024),
		protocol2Method:      make(map[string]reflect.Value),
		NextCheckPingFrame:   int(ClientPingInterval / AgentFrameInterval),
		delayDisconnectTimer: timer,
		CachedMsgs:           make([][]byte, 0, 64),
		Log:                  log,
		Uid:                  "",
		RoomId:               "",
	}

	t := reflect.ValueOf(self)
	for p, v := range validAgentProtocols {
		m := t.MethodByName(v)
		if !m.IsValid() {
			log.Error("uid %v protocol handler not found, %s", self.Uid, p)
		}
		self.protocol2Method[p] = m
	}

	self.MsgsFromRoom <- func() {

		if values.Get("reconnectFlag") == "2" {
			self.OnReconnect(values.Get("post_data"))
		} else {
			self.OnLogin(values.Get("timestamp"), values.Get("nonstr"), values.Get("post_data"), values.Get("sign"), 0)
		}
	}
	return self
}

func (self *Agent) readPump() {

	defer func() {
		self.RecvQueue <- func() { self.onMultiClose() }
		close(self.RecvQueue)
		self.Conn.Close()

	}()

	for {
		t, msg, err := self.Conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
				self.RecvQueue <- func() { self.onMultiError(err) }
			}

			break
		}

		self.LastPingTimeFrame = self.FrameID

		if t == websocket.TextMessage {
			self.RecvQueue <- func() { self.OnText(string(msg[:])) }
			continue
		}

		// t == websocket.BinaryMessage
		self.RecvQueue <- func() { self.OnBinary(msg) }
	}
}

func (self *Agent) writePump() {

	defer func() {
		self.Conn.Close()
	}()

	for {
		s, ok := <-self.SendQueue
		if !ok {
			// close channel.
			self.Conn.WriteMessage(websocket.CloseMessage, []byte{})
			break
		}

		self.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := self.Conn.WriteMessage(s.t, s.msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
				self.Log.Error("uid %v WriteMessage() error, %+v", self.Uid, err)
			}

			break
		}
	}
}

func (self *Agent) onMultiClose() {
	if self.Closed {
		return
	}

	self.Conn.Close()
	self.Closed = true

	self.OnClose()
}

func (self *Agent) onMultiError(err error) {
	if self.LastError != nil {
		return
	}

	self.LastError = err
	self.OnError(err)
}

func (self *Agent) Run() {
	self.Conn.SetReadLimit(MaxMsgSize)
	self.Conn.SetPingHandler(func(string) error {
		self.RecvQueue <- func() { self.SendQueue <- &SendEvent{t: websocket.PongMessage, msg: []byte{}} }
		return nil
	})

	go self.readPump()
	go self.writePump()

	self.FrameTicker = time.NewTicker(AgentFrameInterval)
	defer func() {
		self.FrameTicker.Stop()
	}()

	self.OnOpen()

	for {
		select {
		case c, ok := <-self.RecvQueue:
			if !ok {
				return
			}
			SafeRunClosure(self, c)
		case c := <-self.MsgsFromRoomMgr:
			SafeRunClosure(self, c)
		case c := <-self.MsgsFromRoom:
			SafeRunClosure(self, c)
		case <-self.FrameTicker.C:
			self.Frame()
		case <-self.delayDisconnectTimer.C:
			self.Log.Error("uid %v doDisconnect", self.Uid)
			self.onMultiClose()
			return
		}
	}
}

func (obj *Agent) SendBinary(msg []byte) {
	config := g_roomServer.Config
	if !config.Agent.EnableCachedMsg {
		obj.SendBinaryNow(msg)
		return
	}

	obj.CachedMsgs = append(obj.CachedMsgs, msg)
	if len(obj.CachedMsgs) >= config.Agent.CachedMsgMaxCount {
		obj.FlushCachedMsgs()
		return
	}
}

func (self *Agent) SendBinaryNow(msg []byte) {
	if self.IsDisconnected || self.Closed {
		return
	}

	self.SendQueue <- NewSendBinaryEvent(msg)
}

func (self *Agent) OnOpen() {
	self.Log.Info("...")
}

func (self *Agent) OnBinary(msg []byte) {
	ba := pb.CreateByteArray(msg)

	// 协议名长度
	dataLen, err := ba.ReadUint8()
	if err != nil {
		self.Log.Error("uid %v read proto head err %+v", self.Uid, err)
		return
	}

	// 协议名
	protoName, err := ba.ReadString(int(dataLen))
	if err != nil {
		self.Log.Error("uid %v read proto name err %+v", self.Uid, err)
		return
	}
	protoType := proto.MessageType(protoName)
	if protoType == nil {
		self.Log.Error("uid %v did not find proto ===== %s", self.Uid, protoName)
		return
	}

	// 协议结构体
	protoBody := reflect.New(protoType.Elem()).Interface().(proto.Message)
	pbBytes := make([]byte, ba.Available())
	_, err = ba.Read(pbBytes)
	if err != nil {
		self.Log.Error("uid %v read proto body err", self.Uid, err)
		return
	}
	err = proto.Unmarshal(pbBytes, protoBody)
	if err != nil {
		self.Log.Error("uid %v proto unmarshal err %+v", self.Uid, err)
		return
	}

	if g_roomServer.Config.Agent.EnableLogRecv && protoName != "pb.c2sHeart" && protoName != "pb.c2sStrike" {
		self.Log.Error("uid %vreceive ==== %s %+v", self.Uid, protoName, protoBody)
	}

	method, ok := self.protocol2Method[protoName]
	if ok {
		in := []reflect.Value{reflect.ValueOf(protoBody)} //只接受一个参数
		SafeRunClosure(self, func() {
			method.Call(in)
		})
		return
	}

	if self.Room == nil {
		self.Log.Info("uid %v ROOM is not ready, skip request, %s", self.Uid, protoName)
		return
	}

	if !self.Room.CanHandleProtocol(protoName) {
		self.Log.Info("uid %v unexpected protocol, %s", self.Uid, protoName)
		return
	}

	RunOnRoom(self.Room.GetChanForAgent(), self.Room, func(room *Room) {
		room.ApplyProtoHandler(self, protoName, protoBody)
	})
}

func (self *Agent) OnText(msg string) {
	self.Log.Info("on longer supports text message %s", msg)
}

func (self *Agent) OnError(err error) {
	self.Log.Error("uid %v %+v", self.Uid, err)
}

func (self *Agent) OnClose() {
	self.Log.Info("OnClose uid %v ...", self.Uid)

	close(self.SendQueue)

	if !self.IsDisconnected {
		self.notifyLostAgent()
	}
}

func (self *Agent) CompresssData(data []byte) []byte {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(data)
	w.Close()

	compressedData := b.Bytes()
	compressedDataLen := len(compressedData)
	sendData := make([]byte, 0, 1+SizeBytes+compressedDataLen)
	sendData = append(sendData, 'c')

	if SizeBytes >= 2 {
		sendData = append(sendData, byte(compressedDataLen&0xff)) // 小端
		sendData = append(sendData, byte((compressedDataLen>>8)&0xff))
	}

	if SizeBytes == 4 {
		sendData = append(sendData, byte((compressedDataLen>>16)&0xff))
		sendData = append(sendData, byte((compressedDataLen>>24)&0xff))
	}

	sendData = append(sendData, compressedData...)

	return sendData
}

func (self *Agent) FlushCachedMsgs() {
	msgCount := len(self.CachedMsgs)
	if msgCount <= 0 {
		return
	}

	if msgCount == 1 {
		data := self.CachedMsgs[0]
		self.SendBinaryNow(data)
	} else {
		ba := pb.CreateEmpyByteArray()
		ba.WriteUint8(uint8(len(MultiFlag)))
		ba.WriteString(MultiFlag)
		for _, binary := range self.CachedMsgs {
			ba.WriteInt32(int32(len(binary)))
			ba.WriteBytes(binary)
		}
		self.SendBinaryNow(ba.Bytes())
	}
	self.CachedMsgs = self.CachedMsgs[:0]
	return

}

func (self *Agent) DoLoginReply(result bool, msg string) {
	binary, err := GetBinary(&pb.S2CLogin{Result: result, Msg: msg}, self.Log)
	if err != nil {
		return
	}
	self.SendBinaryNow(binary)
	if result {
		return
	}

	self.Log.Debug("uid %v loginReply, %t, %s", self.Uid, result, msg)
	self.DelayDisconnect(time.Second * 5)
}

//客户端断线重连
func (self *Agent) OnReconnect(postDataStr string) {
	postData := new(PostData)
	config := g_roomServer.Config

	if err := json.Unmarshal([]byte(postDataStr), postData); err != nil {
		self.DoLoginReply(false, "illegal postData")
		return
	}

	if config.Agent.EnableCheckLoginParams && postData.GameId != config.GameID {
		self.DoLoginReply(false, "unexpected gameid")
		return
	}

	if config.Agent.EnableCheckLoginParams && postData.ChannelId != config.ChannelID {
		self.DoLoginReply(false, "unexpected channelid")
		return
	}

	self.PostData = postData

	playerInfo := postData.Player

	self.Uid = playerInfo.Uid

	roomID := postData.RoomId
	self.RoomId = roomID

	self.Log.Info("roomId %v player uid try reconnect %s", roomID, playerInfo.Uid)
	RunOnRoomMgr(self.RoomMgr.MsgsFromAgent, self.RoomMgr, func(roomMgr *RoomMgr) {
		roomMgr.Login(self, roomID, playerInfo, 2)
	})
}

func (self *Agent) OnLogin(t, nonStr, postDataStr, sign string, reconnectFlag int) {
	if g_roomServer.Config.Agent.EnableLogRecv {
		self.Log.Debug(" receive ====  %s %s %s %s %v", t, nonStr, postDataStr, sign, reconnectFlag)
	}

	timestamp, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		self.DoLoginReply(false, "unexpected timestamp")
		return
	}

	config := g_roomServer.Config
	if config.Agent.EnableCheckLoginParams && math.Abs(float64(time.Now().Sub(time.Unix(timestamp, 0)))) >= float64(config.Agent.TimestampExpireDuration) {
		self.DoLoginReply(false, "timestamp expired")
		return
	}

	expectedSign := GetSha1HexSign(timestamp, nonStr, postDataStr, config.AppKey)
	if config.Agent.EnableCheckLoginParams && expectedSign != sign {
		self.Log.Error("unexpected sign, %s, %s", sign, expectedSign)
		self.DoLoginReply(false, "unexpected sign")
		return
	}

	postData := new(PostData)

	if err := json.Unmarshal([]byte(postDataStr), postData); err != nil {
		self.DoLoginReply(false, "illegal postData")
		return
	}

	if config.Agent.EnableCheckLoginParams && postData.GameId != config.GameID {
		self.DoLoginReply(false, "unexpected gameid")
		return
	}

	if config.Agent.EnableCheckLoginParams && postData.ChannelId != config.ChannelID {
		self.DoLoginReply(false, "unexpected channelid")
		return
	}

	self.PostData = postData

	playerInfo := postData.Player

	self.Uid = playerInfo.Uid

	roomID := postData.RoomId
	self.RoomId = roomID

	self.Log.Info("roomId %v player uid try login, %s", roomID, playerInfo.Uid)
	RunOnRoomMgr(self.RoomMgr.MsgsFromAgent, self.RoomMgr, func(roomMgr *RoomMgr) {
		roomMgr.Login(self, roomID, playerInfo, reconnectFlag)
	})
}

// Room不放具体实例过来，仅放一个chan，因为Room类型可以变化
func (self *Agent) OnLoginReply(result bool, msg string, room *Room) {
	self.DoLoginReply(result, msg)
	if !result {
		return
	}
	self.Room = room
}

func (self *Agent) C2SHeart(protocol *pb.C2SHeart) {
	self.LastPingTimeFrame = self.FrameID
	binary, err := GetBinary(&pb.S2CHeart{Timestamp: int64(protocol.Timestamp)}, self.Log)
	if err != nil {
		return
	}
	self.SendBinaryNow(binary)
}

func (self *Agent) notifyLostAgent() {
	if self.Room != nil {
		self.Log.Info("notifyLostAgent room id, %s, %s", self.RoomId, self.Uid)
		RunOnRoom(self.Room.GetChanForAgent(), self.Room, func(room *Room) {
			room.LostAgent(self)
		})
		return
	}

	if self.PostData != nil {
		roomId := self.PostData.RoomId
		RunOnRoomMgr(self.RoomMgr.MsgsFromAgent, self.RoomMgr, func(roomMgr *RoomMgr) {
			roomMgr.LostAgent(self, roomId)
		})
		return
	}
}

func (self *Agent) CheckPing() {
	if !g_roomServer.Config.Agent.EnableCheckPing {
		return
	}

	pingElapseTime := time.Duration(self.FrameID-self.LastPingTimeFrame) * AgentFrameInterval
	if pingElapseTime < ClientPingTimeout {
		return
	}

	self.notifyLostAgent()

	self.Log.Info("uid %v delayDisconnect: ping timeout", self.Uid)
	self.DelayDisconnect(0)
}

func (self *Agent) Frame() {
	self.FrameID++

	if self.FrameID >= self.NextCheckPingFrame {
		self.NextCheckPingFrame = self.FrameID + int(ClientPingInterval/AgentFrameInterval)

		self.CheckPing()
	}

	self.FlushCachedMsgs()
}

func (self *Agent) DelayDisconnect(delay time.Duration) {
	self.Log.Info("uid %v try delayDisconnect, %.02fs", self.Uid, delay.Seconds())

	self.IsDisconnected = true
	self.delayDisconnectTimer.Reset(delay)
}

func GetBinary(protoMsg proto.Message, log *lgames.Logger) (res []byte, err error) {
	protoName := proto.MessageName(protoMsg)
	if g_roomServer.Config.Agent.EnableLogSend && protoName != "pb.s2cHeart" && protoName != "pb.s2cStrike" {
		log.Debug("send ==== %s %+v", protoName, protoMsg)
	}
	ba := pb.CreateEmpyByteArray()
	ba.WriteUint8(uint8(len(protoName)))
	ba.WriteString(protoName)
	binarybody, err := proto.Marshal(protoMsg)
	if err != nil {
		log.Error("proto.Marshal() failed: %+v", err)
		return nil, err
	}
	ba.WriteBytes(binarybody)
	return ba.Bytes(), nil
}
