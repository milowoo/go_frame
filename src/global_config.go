package new_frame

import (
	"fmt"
	"new_frame/src/lgames"

	"github.com/go-ini/ini"
)

type GlobalConfig struct {
	ReconnectTime     int
	TotalUseTime      int
	UploadUrl         string

}

func loadDomain(log *lgames.Logger) (string, error) {
	cfg, err := ini.Load(fmt.Sprintf("%s/domain.ini", CFG_DIR))
	if err != nil {
		return "", err
	}

	section, err := cfg.GetSection("domain")
	if err != nil {
		log.Error("NewGlobalConfig  GetSection domain  err")
		return "", err
	}

	domain, err := LoadStringFromIniSection(section, "domain")
	if err != nil {
		log.Error("NewGlobalConfig  GetKey domain  err")
		return "", err
	}

	return domain, nil
}

func NewGlobalConfig(log *lgames.Logger) (*GlobalConfig, error) {
	ret := &GlobalConfig{}
	//domain, err := loadDomain(log)
	//if err != nil {
	//	log.Error("NewGlobalConfig load Domain err")
	//	return nil, err
	//}
	//
	//ret.UploadUrl = "http://" + domain + "/gameGateway/resultReport"

	log.Info("NewGlobalConfig  success")

	return ret, nil
}
