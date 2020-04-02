package new_frame

import (
	"crypto/sha1"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"math/rand"
	"time"

	"github.com/go-ini/ini"
)

const (
	RandomBase int = 65535
)

type GameConfig struct {
	GameId         string
	ListenIp       string
	ListenPort     int
	MaxClient      int
	MaxPlayerCount int
	ChannelId      string
	AppKey         string

	EnableLogSend          bool
	EnableLogRecv          bool
	EnableCompressMsg      bool
	EnableCheckPing        bool
	EnableCachedMsg        bool
	EnableCheckLoginParams bool

	CachedMsgMaxCount        int
	CompressMsgSizeThreshold int
}

type BasePlayerInfo struct {
	Uid       string `json:"uid,omitempty"`
	Name      string `json:"name,omitempty"`
	AvatarUrl string `json:"avatarurl,omitempty"`
	Opt       string `json:"opt,omitempty"`
	TeamId    string `json:"teamId,omitempty"`
	Sex       int    `json:"sex,omitempty"`
}

type PostData struct {
	ChannelId string          `json:"channelid,omitempty"`
	GameId    string          `json:"gameid,omitempty"`
	RoomId    string          `json:"roomid,omitempty"`
	Player    *BasePlayerInfo `json:"player,omitempty"`
}

type LoginFromAgent struct {
	RoomId     string
	PlayerInfo *BasePlayerInfo
}

type MsgApplyFunction struct {
	Func   string
	Params []interface{}
}

func GetSha1HexSign(timestamp int64, nonStr string, postDataStr string, appKey string) string {
	data := fmt.Sprintf("%d%s%s%s", timestamp, nonStr, postDataStr, appKey)
	sum := sha1.Sum([]byte(data))
	return fmt.Sprintf("%x", sum)
}

type InnerGameResult struct {
	GameId     string      `json:"gameid,omitempty"`
	RoomId     string      `json:"roomid,omitempty"`
	ChannelId  string      `json:"channelid,omitempty"`
	ResultType string      `json:"resulttype,omitempty"`
	Users      []string    `json:"users,omitempty"`
	Winners    []string    `json:"winners,omitempty"`
	Jifen      []string    `json:"jifen,omitempty"`
	GameSpec   interface{} `json:"gameSpec,omitempty"`
}

type GameResult struct {
	Timestamp     int64            `json:"timestamp,omitempty"`
	NonStr        string           `json:"nonstr,omitempty"`
	Sign          string           `json:"sign,omitempty"`
	ResultRawData string           `json:"resultrawdata,omitempty"`
	Result        *InnerGameResult `json:"result,omitempty"`
	GameType      string           `json:"gametype,omitemtpy`
}

// [minLen, maxLenPlus1)
const (
	RANDOM_CHAR_LIST     = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	RANDOM_CHAR_LIST_LEN = len(RANDOM_CHAR_LIST)
)

func RandomInt(rand *rand.Rand, min int, maxPlus1 int) int {
	return min + int(rand.Int63n(int64(maxPlus1-min)))
}

func RandomString(rand *rand.Rand, minLen int, maxLenPlus1 int) string {
	randLen := RandomInt(rand, minLen, maxLenPlus1)
	sections := make([]byte, 0, maxLenPlus1)
	var i int = 0
	for ; i < randLen; i++ {
		sections = append(sections, RANDOM_CHAR_LIST[RandomInt(rand, 0, RANDOM_CHAR_LIST_LEN)])
	}

	return string(sections)
}

func Clamp(v int, min int, max int) int {
	if v < min {
		return min
	}

	if v > max {
		return max
	}

	return v
}

func SafeRunClosure(v interface{}, c Closure) {
	defer func() {
		if err := recover(); err != nil {
			//log.Printf("%+v: %s", err, debug.Stack())

		}
	}()

	c()
}

type NetworkAsClient struct {
	Host         string
	Port         int
	PingInterval time.Duration
	HMACKey      string
}

type NetworkAsServer struct {
	ListenIp   string
	ListenPort int
	Timeout    time.Duration
	HMACKey    string
}

func NewNetworkAsServerFromIniFile(cfg *ini.File, sectionName string) (*NetworkAsServer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cfg is nil")
	}

	network := &NetworkAsServer{
		ListenIp:   "0.0.0.0",
		ListenPort: -1,
		Timeout:    0,
	}

	section, err := cfg.GetSection(sectionName)
	if err != nil {
		return nil, err
	}

	key, err := section.GetKey("listenIp")
	if err != nil {
		return nil, err
	}
	network.ListenIp = key.String()

	key, err = section.GetKey("listenPort")
	if err != nil {
		return nil, err
	}
	network.ListenPort = key.MustInt(-1)
	if network.ListenPort <= 0 {
		return nil, fmt.Errorf("unexpected listenPort=%d", network.ListenPort)
	}

	key, err = section.GetKey("timeout")
	if err != nil {
		return nil, err
	}

	seconds := key.MustFloat64(-1.0)
	if seconds <= 0 {
		return nil, fmt.Errorf("unexpected timeout=%.02f", seconds)
	}
	network.Timeout = time.Duration(seconds * float64(time.Second))

	// optional
	key, err = section.GetKey("hmacKey")
	if err == nil {
		network.HMACKey = key.MustString("")
	}

	return network, nil
}

func LoadPositiveIntFromIniSection(section *ini.Section, keyName string) (int, error) {
	key, err := section.GetKey(keyName)
	if err != nil {
		return -1, err
	}

	v := key.MustInt(-1)
	if v <= 0 {
		return -1, fmt.Errorf("unexpected %s.%s=%d", section.Name(), keyName, v)
	}

	return v, nil
}

func LoadPositiveInt64FromIniSection(section *ini.Section, keyName string) (int64, error) {
	key, err := section.GetKey(keyName)
	if err != nil {
		return -1, err
	}

	v := key.MustInt64(-1)
	if v <= 0 {
		return -1, fmt.Errorf("unexpected %s.%s=%d", section.Name(), keyName, v)
	}

	return v, nil
}

func LoadPositiveFloatFromIniSection(section *ini.Section, keyName string) (float64, error) {
	key, err := section.GetKey(keyName)
	if err != nil {
		return -1, err
	}

	v := key.MustFloat64(-1)
	if v <= 0 {
		return -1, fmt.Errorf("unexpected %s.%s=%f", section.Name(), keyName, v)
	}

	return v, nil
}

func LoadSecondFromIniSection(section *ini.Section, keyName string) (time.Duration, error) {
	key, err := section.GetKey(keyName)
	if err != nil {
		return -1, err
	}

	f := key.MustFloat64(-1)
	if f <= 0 {
		return -1, fmt.Errorf("unexpected %s.%s=%f", section.Name(), keyName, f)
	}

	return time.Duration(float64(time.Second) * f), nil
}

func LoadBoolFromIniSection(section *ini.Section, keyName string) (bool, error) {
	key, err := section.GetKey(keyName)
	if err != nil {
		return false, err
	}

	v := key.MustBool(false)

	return v, nil
}

func LoadStringFromIniSection(section *ini.Section, keyName string) (string, error) {
	key, err := section.GetKey(keyName)
	if err != nil {
		return "", err
	}

	v := key.MustString("")

	return v, nil
}

func RunOnAgent(c chan Closure, agent *Agent, cb func(agent *Agent)) {
	c <- func() {
		cb(agent)
	}
}

func RunOnRoomMgr(c chan Closure, mgr *RoomMgr, cb func(mgr *RoomMgr)) {
	c <- func() {
		cb(mgr)
	}
}

func RunOnRoom(c chan Closure, room *Room, cb func(room *Room)) {
	c <- func() {
		cb(room)
	}
}

func NewRandNum(baseRand int) int {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return r.Intn(baseRand)
}

//获取时间戳(毫秒）
func GetCurTimeMillis() int64 {
	return time.Now().UnixNano() / 1e6
}

func UUID()string {
	UID := uuid.NewV4()
	return  UID.String()
}

func B2S(bs []uint8) string {
	ba := []byte{}
	for _, b := range bs {
		ba = append(ba, byte(b))
	}
	return string(ba)
}