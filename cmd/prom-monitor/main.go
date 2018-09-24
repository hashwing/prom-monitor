package main

import (
	"os"
	"time"
	"bytes"
	"syscall"
	"context"
	"strings"
	"net/http"
	"os/signal"
	"encoding/base64"
	"github.com/hashwing/log"
	"github.com/hashwing/promxy"
)

func main(){
	ctx,cancel:=context.WithCancel(context.Background())
	defer cancel()
	opts:=promxy.CLIOpts{
		BindAddr:":8082",
		ConfigFile:"config.yml",
		LogLevel:"info",
		QueryTimeout:time.Second,
		QueryMaxConcurrency:1000,
		NotificationQueueCapacity:10000,
	}
	nf :=func(alerts ...*promxy.Alert)error{
		for _,a:=range alerts{
			log.Info(a.String())
		}
		return nil
	}
	auth := func(w http.ResponseWriter,r *http.Request)(*http.Request,bool){
		basicAuthPrefix := "Basic "
        // 获取 request header
        auth := r.Header.Get("Authorization")
        // 如果是 http basic auth
        if strings.HasPrefix(auth, basicAuthPrefix) {
            // 解码认证信息
            payload, err := base64.StdEncoding.DecodeString(
                auth[len(basicAuthPrefix):],
            )
            if err == nil {
                pair := bytes.SplitN(payload, []byte(":"), 2)
                if len(pair) == 2 && bytes.Equal(pair[0], []byte("admin")) && bytes.Equal(pair[1],[]byte("admin")) {
                    commonLabels:=map[string]string{
						"sg": "test1",
					}
					valueCtx :=context.WithValue(r.Context(),"common_labels",commonLabels)
					r=r.WithContext(valueCtx)
                    return r,true
                }
            }
        }

        // 认证失败，提示 401 Unauthorized
        w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
        // 401 状态码
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("401 Unauthorized"))
		return r,false
	}
	r,err:=promxy.Run(ctx,opts,nf,auth)
	if err!=nil{
		log.Error(err)
		return
	}
	r.HandlerFunc("GET", "/reload", func(w http.ResponseWriter,r *http.Request){
		err:=promxy.ReloadConfig()
		if err!=nil{
			log.Error(err)
		}
	})
	srv := &http.Server{
		Addr:    ":8082",
		Handler:r,
	}

	go func() {
		log.Info("promxy starting")
		if err := srv.ListenAndServe(); err != nil {
			log.Error("Error listening: %v", err)
		}
	}()
	sigs := make(chan os.Signal)
	defer close(sigs)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)

	// wait for signals etc.
	for {
		select {
		case sig := <-sigs:
			switch sig {
			case syscall.SIGHUP:
				log.Info("Reloading config")
				if err := promxy.ReloadConfig(); err != nil {
					log.Error("Error reloading config: %s", err)
				}
			case syscall.SIGTERM, syscall.SIGINT:
				log.Info("promxy exiting")
				cancel()
				srv.Shutdown(ctx)
				return
			default:
				log.Error("Uncaught signal: %v", sig)
			}

		}
	}
}