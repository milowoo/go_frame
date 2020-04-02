package main

import (
	"log"
	"new_frame/src"
	"new_frame/src/lgames"
)

const (
	LOG_DIR = "/data/yy/llog/new_frame/"
)

func main() {

	logName := LOG_DIR + "frametest.log"
	//日志名 + 文件大小（M为单位） + 打印标志 + 线程数量 （未启动） + 工作协程长度（未启动） + 深度
	log := lgames.NewLogger2(logName, 1024*2, log.LstdFlags|log.Lshortfile, 8, 1024, 2)
	roomServer, err := new_frame.NewRoomServer(log)
	if err != nil {
		log.Warn("NewRoomServer failed, %+v", err)
	}
	roomServer.Run()
}
