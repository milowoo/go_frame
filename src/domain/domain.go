package domain

import (
	"time"

	"labix.org/v2/mgo/bson"
)

/*
**	player 玩家记录
 */
type Player struct {
	Uid         bson.ObjectId `bson:"_id,omitempty"`
	Url         string        `bson:"url" json:"url"`
	Sex         int8          `bson:"sex" json:"sex"`
	Utime       time.Time     `bson:"utime" json:"utime"`
}


func NewPlayer(uid string) *Player {
	mongoData := &Player{Uid:bson.ObjectId(uid),
	}

	return  mongoData;
}