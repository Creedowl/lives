package platform

import (
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
	BaseUrl = "https://www.douyu.com/%d"
	RoomUrl = "https://www.douyu.com/lapi/live/getH5Play/%d"
	DID     = `'10000000000000000000000000001501'`
)

var (
	roomIDRe     = regexp.MustCompile(`\$ROOM\.room_id\s*=\s*(\d+)`)
	roomStatusRe = regexp.MustCompile(`\$ROOM\.show_status\s*=\s*(\d+)`)
	jsRe         = regexp.MustCompile(`<script type="text/javascript">(\s*var[\s\S]*?)</script>`)
)

type Douyu struct {
	RoomID  uint
	Quality uint
	Status  int
	Clients map[*websocket.Conn]bool
}

func GetDouyuRoom(roomID, quality uint, client *websocket.Conn) (Room, error) {
	// get real room id
	html, err := util.Request("GET", fmt.Sprintf(BaseUrl, roomID), "", nil)
	if err != nil {
		return nil, err
	}
	r := roomIDRe.FindSubmatch(html)
	if len(r) == 0 {
		return nil, errors.New(fmt.Sprintf("room %d not found", roomID))
	}
	_roomID, err := strconv.Atoi(string(r[1]))
	if err != nil {
		return nil, err
	}
	roomID = uint(_roomID)

	// get room status
	status, err := strconv.Atoi(string(roomStatusRe.FindSubmatch(html)[1]))
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
	return nil, nil
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
	html, err := util.Request("GET", fmt.Sprintf(BaseUrl, d.RoomID), "", nil)
	if err != nil {
		return nil, err
	}
	matches := jsRe.FindAllSubmatch(html, -1)
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
		[]byte(fmt.Sprintf("ub98484234(%d,%s,%d)", d.RoomID, DID, time.Now().Unix()))...))
	if err != nil {
		return nil, err
	}
	logger.Infof("params %s", v.String())
	res, err := util.Request(
		"POST",
		fmt.Sprintf(RoomUrl, d.RoomID),
		fmt.Sprintf("%s&rate=%d", v.String(), d.Quality),
		map[string]string{
			"Content-type": "application/x-www-form-urlencoded",
		})
	if err != nil {
		return nil, err
	}
	logger.Info(string(res))
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

func (d *Douyu) AddClient(conn *websocket.Conn) {
	panic("implement me")
}

func (d *Douyu) RemoveClient(conn *websocket.Conn) {
	panic("implement me")
}

func (d *Douyu) Send(danmaku Danmaku) {
	panic("implement me")
}

func (d *Douyu) Close() {
	panic("implement me")
}

func (d *Douyu) Connect() {
	panic("implement me")
}
