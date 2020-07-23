package platform

import (
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"live/util"
)

type Type uint32

const (
	BILIBILI Type = iota
	DOUYU
)

var logger = util.GetLogger()

type Danmaku struct {
	Text  string `json:"text"`
	Color string `json:"color"`
	Type  int    `json:"type"`
}

type Quality struct {
	Quality     uint64 `json:"quality"`
	Description string `json:"description"`
}

type Platform struct {
	Type           Type      `json:"type"`
	RoomID         uint      `json:"room_id"`
	Status         uint      `json:"status"`
	CurrentQuality uint      `json:"current_quality"`
	Link           string    `json:"link"`
	Qualities      []Quality `json:"qualities"`
}

type Room interface {
	GetLiveInfo() (*Platform, error)
	AddClient(conn *websocket.Conn)
	RemoveClient(conn *websocket.Conn)
	Send(danmaku Danmaku)
	Close()
	Connect()
}

// danmaku cache
var rooms = map[string]Room{}

func selectPlatform(platform Type, roomID, quality uint, client *websocket.Conn) (Room, error) {
	switch platform {
	case BILIBILI:
		return GetBilibiliRoom(roomID, quality, client)
	case DOUYU:
		return GetDouyuRoom(roomID, quality, client)
	default:
		return nil, errors.New(fmt.Sprintf("platform %d not found", platform))
	}
}

func InitRoom(platform Type, roomID, quality uint) (*Platform, error) {
	room, err := selectPlatform(platform, roomID, quality, nil)
	if err != nil {
		return nil, err
	}
	return room.GetLiveInfo()
}

func InitDanmaku(platform Type, roomID uint, conn *websocket.Conn) {
	room, err := selectPlatform(platform, roomID, 0, conn)
	if err != nil {
		logger.Error(err)
		return
	}
	room.Connect()
}
