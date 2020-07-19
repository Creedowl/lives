package platform

import (
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
)

type Type uint32

const (
	BILIBILI Type = 1
)

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
	Status         uint64    `json:"status"`
	CurrentQuality uint64    `json:"current_quality"`
	Links          []string  `json:"links"`
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

var rooms = map[string]Room{}

func selectPlatform(platform Type, roomID, quality uint, client *websocket.Conn) Room {
	switch platform {
	case BILIBILI:
		roomID = GetRoomId(roomID)
		if client == nil {
			return &Bilibili{
				RoomID:  roomID,
				Quality: quality,
			}
		}
		index := fmt.Sprintf("%d:%d", platform, roomID)
		if rooms[index] == nil {
			rooms[index] = &Bilibili{
				Closed:  false,
				Clients: make(map[*websocket.Conn]bool),
				RoomID:  roomID,
			}
		}
		rooms[index].AddClient(client)
		return rooms[index]
	default:
		return nil
	}
}

func InitRoom(platform Type, roomID, quality uint) (*Platform, error) {
	room := selectPlatform(platform, roomID, quality, nil)
	if room == nil {
		return nil, errors.New(fmt.Sprintf("platform %d not found", platform))
	}
	return room.GetLiveInfo()
}

func InitDanmaku(platform Type, roomID uint, conn *websocket.Conn) {
	room := selectPlatform(platform, roomID, 0, conn)
	if room == nil {
		return
	}
	room.Connect()
}
