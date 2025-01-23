package gins

type ResultObject struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
	//	Token   string      `json:"token"`
}
