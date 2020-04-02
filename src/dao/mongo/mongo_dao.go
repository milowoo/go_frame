package mongo

import (
	"time"

	"new_frame/src/domain"
	"new_frame/src/lgames"

	"gopkg.in/mgo.v2/bson"
)

type MongoDAO struct {
	BaseDAO
	log *lgames.Logger
}

func NewMongoDao(dataSource *DataSource, log *lgames.Logger) *MongoDAO {
	dao := &MongoDAO{BaseDAO: *NewBaseDAO(dataSource),
		log: log}
	return dao
}

//获取设备信息
func (dao *MongoDAO) GetPlayer(uid string) (*domain.Player, error) {
	session := dao.dataSource.GetSession()
	defer session.Close()

	c := session.DB(dao.dataSource.database).C("player")
	m := domain.Player{}
	err := c.Find(&bson.M{"_id": uid}).One(&m)
	if err != nil {
		dao.log.Debug("GetPlayer err %v uid %v ", err, uid)
		return &m, err
	}
	return &m, nil
}

func (dao *MongoDAO) InsertPlayer(player *domain.Player) error {
	player.Utime = time.Now()

	session := dao.dataSource.GetSession()
	defer session.Close()
	c := session.DB(dao.dataSource.database).C("player")
	err := c.Insert(player)
	if err != nil {
		dao.log.Debug("InsertPlayer err %v uid %v", player.Uid)
		return err
	}

	return nil

}

func (dao *MongoDAO) SavaPlayer(uid string, player interface{}) error {
	session := dao.dataSource.GetSession()
	defer session.Close()
	c := session.DB(dao.dataSource.database).C("player")

	query := bson.M{"_id": uid}

	_, err := c.Upsert(query, player)
	if err != nil {
		dao.log.Debug("UpdatePlayer err %v, uid %v",err,  uid)
		return err
	}

	return nil
}

func (dao *MongoDAO) DeletePlayer(uid string) error {
	session := dao.dataSource.GetSession()
	defer session.Close()
	c := session.DB(dao.dataSource.database).C("player")

	err := c.Remove(bson.M{"_id": uid})

	if err != nil {
		dao.log.Debug("DeletePlayer err %v uid %v", err, uid)
		return err
	}

	return nil
}

//func (dao *MongoDAO) GetPlayerByTime(updateTime int64) ([]domain.Player, error) {
//	session := dao.dataSource.GetSession()
//	defer session.Close()
//	m := make([]domain.Player, 0)
//
//	c := session.DB(dao.dataSource.database).C("player")
//	var player domain.Player
//	iter := c.Find(bson.M{"updateTime": bson.M{"$gt": updateTime}}).Iter()
//	for iter.Next(&player) {
//		m = append(m, player)
//	}
//
//	return m, nil
//}
