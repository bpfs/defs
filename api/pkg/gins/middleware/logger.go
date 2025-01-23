package middleware

import (
	"bytes"
	"fmt"
	"io"

	"math"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func Logger() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		startTime := time.Now().UTC()

		// 请求报文
		var requestBody []byte
		if ctx.Request.Body != nil {
			var err error
			requestBody, err = ctx.GetRawData()
			if err != nil {
				logrus.Warn(map[string]interface{}{"err": err.Error()}, "get http request body error")
			}
			ctx.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// 处理请求
		ctx.Next()

		stopTime := time.Since(startTime)
		spendTime := fmt.Sprintf("%d ms", int(math.Ceil(float64(stopTime.Nanoseconds())/1000000.0)))
		statusCode := ctx.Writer.Status()
		clientIp := ctx.ClientIP()
		userAgent := ctx.Request.UserAgent()
		dataSize := ctx.Writer.Size()
		if dataSize < 0 {
			dataSize = 0
		}
		method := ctx.Request.Method
		path := ctx.Request.RequestURI

		logrus.Info(map[string]interface{}{
			"Status":    statusCode,
			"SpendTime": spendTime,
			"Ip":        clientIp,
			"Uri":       path,
			"Method":    method,
			"Agent":     userAgent,
		})
	}
}
