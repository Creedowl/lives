package platform

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/markbates/pkger"
	"github.com/robertkrimen/otto"
	"github.com/tidwall/gjson"
	"io/ioutil"
	"live/util"
	"regexp"
	"strconv"
	"time"
)

const (
	DouyuBaseUrl      = "https://www.douyu.com/%d"
	DouyuRoomUrl      = "https://www.douyu.com/lapi/live/getH5Play/%d"
	DouyuDID          = "'10000000000000000000000000001501'"
	DouyuDanmakuUrl   = "wss://danmuproxy.douyu.com:8501/"
	DouyuLoginMsg     = "type@=loginreq/roomid@=%d/"
	DouyuJoinGroupMsg = "type@=joingroup/rid@=%d/gid@=-9999/"
)

var (
	DouyuRoomIDRe      = regexp.MustCompile(`\$ROOM\.room_id\s*=\s*(\d+)`)
	DouyuRoomStatusRe  = regexp.MustCompile(`\$ROOM\.show_status\s*=\s*(\d+)`)
	DouyuJsRe          = regexp.MustCompile(`<script type="text/javascript">(\s*var[\s\S]*?)</script>`)
	DouyuDanmakuTypeRe = regexp.MustCompile(`type@=(\w+)/?`)
	DouyuDanmakuTxtRe  = regexp.MustCompile(`txt@=([\s\S]+?)/`)
)

type Douyu struct {
	ctx     context.Context
	cancel  context.CancelFunc
	Closed  bool
	RoomID  uint
	Quality uint
	Status  int
	Dan     *websocket.Conn
	Clients map[*websocket.Conn]bool
}

func (d *Douyu) GetClients() map[*websocket.Conn]bool {
	return d.Clients
}

func (d *Douyu) IsClosed() bool {
	return d.Closed
}

func GetDouyuRoom(roomID, quality uint, client *websocket.Conn) (Room, error) {
	// get real room id
	html, err := util.Request("GET", fmt.Sprintf(DouyuBaseUrl, roomID), "", nil)
	if err != nil {
		return nil, err
	}
	r := DouyuRoomIDRe.FindSubmatch(html)
	if len(r) == 0 {
		return nil, errors.New(fmt.Sprintf("room %d not found", roomID))
	}
	_roomID, err := strconv.Atoi(string(r[1]))
	if err != nil {
		return nil, err
	}
	roomID = uint(_roomID)

	// get room status
	status, err := strconv.Atoi(string(DouyuRoomStatusRe.FindSubmatch(html)[1]))
	if err != nil {
		return nil, err
	}
	if client == nil {
		return &Douyu{
			RoomID:  roomID,
			Quality: quality,
			Status:  status,
		}, nil
	}
	index := fmt.Sprintf("%d:%d", DOUYU, roomID)
	if rooms[index] == nil {
		rooms[index] = &Douyu{
			Closed:  false,
			Clients: make(map[*websocket.Conn]bool),
			RoomID:  roomID,
		}
	}
	AddClient(rooms[index], client)
	return rooms[index], nil
}

func (d *Douyu) GetLiveInfo() (*Platform, error) {
	if d.Status != 1 {
		return &Platform{
			Type:           DOUYU,
			RoomID:         d.RoomID,
			Status:         0,
			CurrentQuality: d.Quality,
		}, nil
	}
	html, err := util.Request("GET", fmt.Sprintf(DouyuBaseUrl, d.RoomID), "", nil)
	if err != nil {
		return nil, err
	}
	matches := DouyuJsRe.FindAllSubmatch(html, -1)
	code := matches[len(matches)-1][1]
	js, err := pkger.Open("/util/CryptoJS.js")
	if err != nil {
		return nil, err
	}
	defer js.Close()
	cryptoJS, err := ioutil.ReadAll(js)
	if err != nil {
		return nil, err
	}
	// js vm
	vm := otto.New()
	// use js vm to get params
	v, err := vm.Run(append(append(cryptoJS, code...),
		[]byte(fmt.Sprintf("ub98484234(%d,%s,%d)", d.RoomID, DouyuDID, time.Now().Unix()))...))
	if err != nil {
		return nil, err
	}
	logger.Debugf("params %s", v.String())
	res, err := util.Request(
		"POST",
		fmt.Sprintf(DouyuRoomUrl, d.RoomID),
		fmt.Sprintf("%s&rate=%d", v.String(), d.Quality),
		map[string]string{
			"Content-type": "application/x-www-form-urlencoded",
		})
	if err != nil {
		return nil, err
	}
	logger.Debugf(string(res))
	data := gjson.ParseBytes(res)
	var qualities []Quality
	data.Get("data.multirates").ForEach(func(key, value gjson.Result) bool {
		qualities = append(qualities, Quality{
			Quality:     value.Get("rate").Uint(),
			Description: value.Get("name").String(),
		})
		return true
	})
	return &Platform{
		Type:           DOUYU,
		RoomID:         d.RoomID,
		Status:         uint(d.Status),
		CurrentQuality: d.Quality,
		Link:           data.Get("data.rtmp_url").String() + "/" + data.Get("data.rtmp_live").String(),
		Qualities:      qualities,
	}, nil
}

func (d *Douyu) Send(danmaku *Danmaku) {
	logger.Infof("danmaku %+v", danmaku)
	if len(d.Clients) == 0 {
		d.Close()
		return
	}
	for client := range d.Clients {
		err := client.WriteJSON(danmaku)
		if err != nil {
			RemoveClient(d, client)
			continue
		}
	}
}

func (d *Douyu) Close() {
	if d.Closed {
		return
	}
	d.Closed = true
	// stop heartbeat and listener, note listener will not exit until this func is done
	d.cancel()
	// close danmaku websocket (douyu)
	message := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "close")
	_ = d.Dan.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second*5))
	for client := range d.Clients {
		_ = client.Close()
	}
	delete(rooms, fmt.Sprintf("%d:%d", DOUYU, d.RoomID))
	logger.Infof("room %d closed, rooms %+v", d.RoomID, rooms)
}

// danmaku data structure
// source: https://open.cplusplus.me/DevelopmentDocs/%E6%96%97%E9%B1%BC%E5%BC%B9%E5%B9%95%E6%9C%8D%E5%8A%A1%E5%99%A8%E7%AC%AC%E4%B8%89%E6%96%B9%E6%8E%A5%E5%85%A5%E5%8D%8F%E8%AE%AEv1.6.2.pdf
// note: 1. date length in first 4 bytes doesn't contain the first 4 bytes
//       2. one message may contain multi danmaku and one message may be split into multi message
func (d *Douyu) encode(data []byte) []byte {
	header := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0xb1, 0x2, 0, 0}
	binary.LittleEndian.PutUint32(header[0:], uint32(len(data)+9))
	binary.LittleEndian.PutUint32(header[4:], uint32(len(data)+9))
	return append(append(header, data...), 0x00)
}

func (d *Douyu) decode(raw []byte) ([]string, error) {
	var res []string
	for offset, length := 0, 0; offset < len(raw); offset += length + 4 {
		length = int(binary.LittleEndian.Uint32(raw[offset:]))
		res = append(res, string(raw[offset+12:offset+length+4]))
	}
	return res, nil
}

func (d *Douyu) authenticate() error {
	// login req
	err := d.Dan.WriteMessage(websocket.BinaryMessage,
		d.encode([]byte(fmt.Sprintf(DouyuLoginMsg, d.RoomID))))
	if err != nil {
		return err
	}
	// join group req, the -9999 group has all danmaku
	err = d.Dan.WriteMessage(websocket.BinaryMessage,
		d.encode([]byte(fmt.Sprintf(DouyuJoinGroupMsg, d.RoomID))))
	if err != nil {
		return err
	}
	return nil
}

func (d *Douyu) heartBeat() {
	defer logger.Infof("heartbeat of room %d exited", d.RoomID)
	data := d.encode([]byte("type@=mrkl/"))
	ticker := time.NewTicker(time.Second * 45)
	defer ticker.Stop()
	for true {
		select {
		case <-ticker.C:
			err := d.Dan.WriteMessage(websocket.BinaryMessage, data)
			if err != nil {
				logger.Error(err)
				d.Close()
				return
			}
		case <-d.ctx.Done():
			return
		}
	}
}

func (d *Douyu) listener() {
	defer logger.Infof("listener of room %d exited", d.RoomID)
	var data []byte
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
			_, raw, err := d.Dan.ReadMessage()
			if d.Closed {
				return
			}
			if err != nil {
				logger.Error(err)
				d.Close()
				return
			}
			data = append(data, raw...)
			if raw[len(raw)-1] != 0x0 {
				logger.Debug("half")
				break
			}
			res, err := d.decode(data)
			if err != nil {
				logger.Error(err)
				d.Close()
				return
			}
			data = []byte{}
			logger.Debugf("room id %d clients %+v", d.RoomID, d.Clients)
			for _, dan := range res {
				danmakuType := DouyuDanmakuTypeRe.FindStringSubmatch(dan)[1]
				if danmakuType == "chatmsg" {
					d.Send(&Danmaku{
						Text:  DouyuDanmakuTxtRe.FindStringSubmatch(dan)[1],
						Color: "#fff",
						Type:  0,
					})
				}
			}
		}
	}
}

func (d *Douyu) Connect() {
	if d.Dan != nil {
		return
	}
	conn, _, err := websocket.DefaultDialer.Dial(DouyuDanmakuUrl, nil)
	if err != nil {
		logger.Error(err)
		return
	}
	logger.Infof("connect to danmaku %d", d.RoomID)
	d.Dan = conn
	d.ctx, d.cancel = context.WithCancel(context.Background())
	// start listener
	go d.listener()
	err = d.authenticate()
	if err != nil {
		logger.Error(err)
		return
	}
	// start heartbeat
	go d.heartBeat()
}
