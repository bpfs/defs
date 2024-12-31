package api

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/bpfs/defs"
	"github.com/bpfs/defs/api/pkg/gins/middleware"
	"github.com/bpfs/defs/api/pkg/routers"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func recover400(c *gin.Context) {
	c.JSON(http.StatusNotFound, HandleResult(0, fmt.Errorf("接口地址不存在,请确认后再重试").Error()))
}

func recover500(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Printf("panic: %v\n", r)
			debug.PrintStack()
			c.JSON(http.StatusOK, HandleResult(0, fmt.Errorf("接口异常,请确认后再重试").Error()))

		}
	}()
	c.Next()
}

// 运行api服务
func Runapi(defs *defs.DeFS) {
	version := "v1"

	// api服务
	r := gin.Default()

	// 解决跨域问题
	r.Use(routers.Cors())

	// log中间件
	r.Use(middleware.Logger())

	// 500错误
	r.Use(recover500)

	// 设置安全的 JSON 前缀
	r.SetTrustedProxies([]string{"127.0.0.1"})

	//处理404 请求
	r.NoRoute(recover400)

	// 路由组1 ，处理POST请求
	v1 := r.Group(version)

	// {} 是书写规范
	{

		// 生产环境
		v1.POST("/exec", func(ctx *gin.Context) {
			query := &struct {
				FQL string `json:"fql"` // 创建sql
			}{}

			// 获取传参
			if err := ctx.ShouldBindJSON(query); err != nil {
				ctx.JSON(http.StatusOK, HandleResult(0, "参数解析错误"))
				return
			}

		})

		//////////////////////////////////////////////////////////////

	}

	r.Run(":8081")
}
