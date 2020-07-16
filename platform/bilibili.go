package platform

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
	"io/ioutil"
	"live/util"
	"time"
)

const (
	InitUrl    = "https://api.live.bilibili.com/room/v1/Room/room_init?id=%d"
	LinkUrl    = "https://api.live.bilibili.com/xlive/web-room/v1/index/getRoomPlayInfo?room_id=%d&play_url=1&mask=1&qn=%d&platform=web"
	DanmakuUrl = "wss://broadcastlv.chat.bilibili.com/sub"
)

const (
	WS_OP_HEARTBEAT           = 2
	WS_OP_HEARTBEAT_REPLY     = 3
	WS_OP_MESSAGE             = 5
	WS_OP_USER_AUTHENTICATION = 7
	WS_OP_CONNECT_SUCCESS     = 8
)

var logger = util.NewLogger()

type Bilibili struct {
	ctx     context.Context
	cancel  context.CancelFunc
	Closed  bool
	Clients map[*websocket.Conn]bool
	Dan     *websocket.Conn
	RoomID  uint
	Quality uint
}

func GetRoomId(id uint) uint {
	res := util.Request("GET", fmt.Sprintf(InitUrl, id))
	return uint(gjson.Get(res, "data.room_id").Uint())
}

func (b *Bilibili) GetLiveInfo() (*Platform, error) {
	if b.Quality == 0 {
		b.Quality = 10000
	}
	res := util.Request("GET", fmt.Sprintf(LinkUrl, b.RoomID, b.Quality))
	data := gjson.Parse(res)
	if data.Get("code").Uint() == 60004 {
		return nil, errors.New(fmt.Sprintf("room %d not found", b.RoomID))
	}
	// links
	var links []string
	data.Get("data.play_url.durl").ForEach(func(key, value gjson.Result) bool {
		links = append(links, value.Get("url").String())
		return true
	})
	// qualities
	var qualities []Quality
	data.Get("data.play_url.quality_description").ForEach(func(key, value gjson.Result) bool {
		qualities = append(qualities, Quality{
			Quality:     value.Get("qn").Uint(),
			Description: value.Get("desc").String(),
		})
		return true
	})
	return &Platform{
		Type:           BILIBILI,
		RoomID:         b.RoomID,
		Status:         data.Get("data.live_status").Uint(),
		CurrentQuality: data.Get("data.play_url.current_qn").Uint(),
		Links:          links,
		Qualities:      qualities,
	}, nil
}

func (b *Bilibili) AddClient(conn *websocket.Conn) {
	b.Clients[conn] = true
	logger.Info("qwer")
	logger.Infof("add client %+v", conn.RemoteAddr())
	// listen close event
	go func() {
		closed := false
		conn.SetCloseHandler(func(code int, text string) error {
			message := websocket.FormatCloseMessage(code, "close")
			_ = conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second*5))
			closed = true
			b.RemoveClient(conn)
			logger.Infof("client %+v closed", conn.RemoteAddr())
			return nil
		})
		// do noting here
		for {
			if closed {
				break
			}
			_, _, err := conn.ReadMessage()
			if err != nil {
				b.RemoveClient(conn)
			}
		}
	}()
}

func (b *Bilibili) RemoveClient(conn *websocket.Conn) {
	_ = conn.Close()
	if _, ok := b.Clients[conn]; ok {
		delete(b.Clients, conn)
	}
	// all clients exited
	if len(b.Clients) == 0 {
		b.Close()
	}
}

func (b *Bilibili) Send(danmaku Danmaku) {
	logger.Infof("danmaku %+v", danmaku)
	if len(b.Clients) == 0 {
		b.Close()
		return
	}
	for client := range b.Clients {
		err := client.WriteJSON(danmaku)
		if err != nil {
			b.RemoveClient(client)
			continue
		}
	}
	return
}

func (b *Bilibili) Close() {
	if b.Closed {
		return
	}
	b.Closed = true
	// stop heartbeat and listener, note listener will not exit until this func is done
	b.cancel()
	// close danmaku websocket (bilibili)
	message := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "close")
	_ = b.Dan.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second*5))
	for client := range b.Clients {
		_ = client.Close()
	}
	delete(rooms, fmt.Sprintf("%d:%d", BILIBILI, b.RoomID))
	logger.Infow("room closed",
		"room id", b.RoomID,
		"rooms", rooms)
	return

}

// danmaku data structure
// +-------------+-----------------------------------------+------------------+
// |             |                PACKAGE                  |                  |
// |   HEADLEN   +-------+-------+------------+------------+       DATA       |
// |             |  LEN  |  VER  |   OPTION   |  SEQUENCE  |                  |
// +-------------+-------+-------+------------+------------+------------------+
// |      4      |   2   |   2   |     4      |     4      |    HEADLEN - 16  |
// +-------------+-------+-------+------------+------------+------------------+
// source: https://github.com/metowolf/BilibiliHelper/wiki/%E5%BC%B9%E5%B9%95%E5%8D%8F%E8%AE%AE
// note: data in head are big endian
func encode(data []byte, op int) []byte {
	var header = []byte{0, 0, 0, 0, 0, 0x10, 0, 0x1, 0, 0, 0, byte(op), 0, 0, 0, 0x1}
	// set LEN
	binary.BigEndian.PutUint32(header[0:], uint32(len(header)+len(data)))
	return append(header, data...)
}

func decode(raw []byte) ([]string, error) {
	header := raw[:16]
	var res []string
	operation := binary.BigEndian.Uint32(header[8:])
	switch operation {
	case WS_OP_HEARTBEAT_REPLY:
		logger.Infof("popularity %d", binary.BigEndian.Uint32(raw[16:]))
	case WS_OP_MESSAGE:
		buffer := bytes.NewReader(raw[16:])
		r, err := zlib.NewReader(buffer)
		if err != nil {
			if errors.Is(err, zlib.ErrHeader) {
				break
			}
			return nil, err
		}
		data, err := ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}
		for offset, length := 0, 0; offset < len(data); offset += length {
			length = int(binary.BigEndian.Uint32(data[offset:]))
			res = append(res, string(data[offset+16:offset+length]))
		}
	case WS_OP_CONNECT_SUCCESS:
		logger.Infof("room init result %s", raw[16:])
	default:
		return nil, errors.New(fmt.Sprintf("unsupport operation %d", operation))
	}
	return res, nil
}

func (b *Bilibili) authenticate() error {
	m := map[string]interface{}{
		"clientver": "1.6.3",
		"platform":  "web",
		"protover":  2,
		"roomid":    b.RoomID,
		"uid":       0,
		"type":      2,
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	err = b.Dan.WriteMessage(websocket.BinaryMessage, encode(data, WS_OP_USER_AUTHENTICATION))
	if err != nil {
		return err
	}
	return nil
}

func (b *Bilibili) heartBeat() {
	defer logger.Infof("heartbeat of room %d exited", b.RoomID)
	data := encode(nil, WS_OP_HEARTBEAT)
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			err := b.Dan.WriteMessage(websocket.BinaryMessage, data)
			if err != nil {
				logger.Error(err)
				b.Close()
				return
			}
		case <-b.ctx.Done():
			return
		}
	}
}

func (b *Bilibili) listener() {
	defer logger.Infof("listener of room %d exited", b.RoomID)
	for {
		select {
		case <-b.ctx.Done():
			return
		default:
			_, raw, err := b.Dan.ReadMessage()
			if err != nil {
				if b.Closed {
					return
				}
				logger.Error(err)
				b.Close()
				return
			}
			res, err := decode(raw)
			if err != nil {
				logger.Error(err)
				b.Close()
				return
			}
			logger.Infof("room id %d clients %+v", b.RoomID, b.Clients)
			for _, dan := range res {
				_danmaku := gjson.Parse(dan)
				if _danmaku.Get("cmd").String() == "DANMU_MSG" {
					b.Send(Danmaku{
						Text:  _danmaku.Get("info.1").String(),
						Color: "#ffffff",
						Type:  0,
					})
				}
			}
		}
	}
}

func (b *Bilibili) Connect() {
	if b.Dan != nil {
		return
	}
	conn, _, err := websocket.DefaultDialer.Dial(DanmakuUrl, nil)
	if err != nil {
		logger.Error(err)
		return
	}
	b.Dan = conn
	ctx, cancel := context.WithCancel(context.Background())
	b.ctx = ctx
	b.cancel = cancel
	// start listener
	go b.listener()
	err = b.authenticate()
	if err != nil {
		logger.Error(err)
		b.Close()
		return
	}
	// start heartbeat
	go b.heartBeat()
}
