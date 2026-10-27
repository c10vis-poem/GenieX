// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/qualcomm/GenieX/cli/internal/config"
	"github.com/qualcomm/GenieX/cli/internal/render"
	"github.com/qualcomm/GenieX/cli/server/service"
)

// hostBindingHint suggests --host 0.0.0.0 when bound to loopback, else "".
// Malformed hosts are treated as non-loopback — better silent than misleading.
func hostBindingHint(host string) string {
	h, port, err := net.SplitHostPort(host)
	if err != nil {
		return ""
	}
	loopback := strings.EqualFold(h, "localhost")
	if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	if !loopback {
		return ""
	}
	return fmt.Sprintf("Bound to loopback only. To expose on your network, restart with --host 0.0.0.0:%s", port)
}

// @Title		GenieX Server
// @Version	0.0.0
// @BasePath	/v1
func Serve() {
	service.Init()
	defer service.DeInit()

	gin.SetMode(gin.ReleaseMode)
	engine := gin.Default()

	RegisterRoot(engine)
	RegisterAPIv1(engine)
	RegisterSwagger(engine)

	cfg := config.Get()
	srv := &http.Server{Addr: cfg.Host, Handler: engine}
	listen := srv.ListenAndServe
	scheme := "http"

	if cfg.HTTPS {
		for _, path := range []string{cfg.CertFile, cfg.KeyFile} {
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				fmt.Println(render.GetTheme().Error.Sprintf("HTTPS cert/key file not found: %s", path))
				return
			}
		}
		listen = func() error { return srv.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile) }
		scheme = "https"

		fmt.Println(render.GetTheme().Info.Sprintf("HTTPS enabled: cert=%s key=%s", cfg.CertFile, cfg.KeyFile))
	}

	fmt.Println(render.GetTheme().Info.Sprintf("Local hosting on %s://%s/", scheme, cfg.Host))
	if hint := hostBindingHint(cfg.Host); hint != "" {
		fmt.Println(render.GetTheme().Info.Sprint(hint))
	}

	// Unhandled, Ctrl+C skips the deferred DeInit and pins NPU buffers till reboot.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() { errCh <- listen() }()

	select {
	case err := <-errCh: // listen only returns on failure here
		fmt.Println(render.GetTheme().Error.Sprintf("HTTP/HTTPS Server Error: %v", err))
	case <-sigCh:
		fmt.Println(render.GetTheme().Info.Sprint("Shutting down, releasing model resources..."))
		// Escape hatch for a teardown a stuck request holds up; the buffers leak.
		go func() {
			<-sigCh
			fmt.Println(render.GetTheme().Warning.Sprint("Received second interrupt, terminating immediately. Model resources stay reserved until reboot."))
			os.Exit(1)
		}()
		srv.Shutdown(context.Background())
	}
}
