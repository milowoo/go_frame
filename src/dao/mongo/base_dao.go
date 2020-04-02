package mongo

import (
	"new_frame/src/lgames"

	"gopkg.in/mgo.v2"
)

type Config struct {
	Address string `json:"address"`
	Dbname  string `json:"dbname"`
}

type DataSource struct {
	database string
	session  *mgo.Session
}

func NewDataSource(address string, database string, log *lgames.Logger) (*DataSource, error) {
	var GlobalMgoSession, err = mgo.Dial(address + "/" + database)
	if err != nil {
		log.Error("mon Dial err %v", err)
		return nil, err
	}

	GlobalMgoSession.SetMode(mgo.Monotonic, true)

	dataSource := &DataSource{
		database: database,
		session:  GlobalMgoSession,
	}

	return dataSource, err
}

func (s *DataSource) GetSession() *mgo.Session {
	return s.session.Clone()
}

type BaseDAO struct {
	dataSource *DataSource
}

// Constructor
func NewBaseDAO(dataSource *DataSource) *BaseDAO {
	dao := &BaseDAO{
		dataSource: dataSource,
	}

	return dao
}

func (dao *BaseDAO) GetDataSource() *DataSource {
	return dao.dataSource
}
