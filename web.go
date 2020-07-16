package main

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"live/platform"
	"log"
	"net/http"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type room struct {
	Platform platform.Type `form:"platform"`
	RoomID   uint          `form:"roomID"`
	Quality  uint          `form:"quality"`
}

func NewServer() *gin.Engine {
	r := gin.Default()
	r.GET("/live", RoomInfo)
	r.GET("/danmaku", Danmaku)
	return r
}

func RoomInfo(ctx *gin.Context) {
	var r room
	err := ctx.BindQuery(&r)
	if err != nil {
		log.Println(err)
		ctx.JSON(http.StatusBadRequest, gin.H{
			"msg":  err,
			"data": nil,
		})
		return
	}
	info, err := platform.InitRoom(r.Platform, r.RoomID, r.Quality)
	if err != nil {
		log.Println(err)
		ctx.JSON(http.StatusBadRequest, gin.H{
			"msg":  err.Error(),
			"data": nil,
		})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"msg":  "success",
		"data": info,
	})
	log.Printf("%+v\n", r)
}

// websocket
func Danmaku(ctx *gin.Context) {
	var r room
	err := ctx.BindQuery(&r)
	if err != nil {
		log.Println(err)
		ctx.JSON(http.StatusBadRequest, gin.H{
			"msg":  err,
			"data": nil,
		})
		return
	}
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		log.Println(err)
		return
	}
	platform.InitDanmaku(r.Platform, r.RoomID, conn)
}
