// Package service exposes the same application over a local Unix socket. It
// never owns business rules, and a stopped service is not needed for CLI use.
package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"rumsdlc/internal/app"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"sync"
	"time"
)

const MaxRequest = 8 * 1024 * 1024

func Endpoint(home string) (string, error) {
	path := filepath.Join(home, "service.sock")
	if len([]byte(path)) >= 100 {
		return "", fmt.Errorf("Device home is too long: the full service.sock path must be shorter than 100 bytes; choose a shorter --home")
	}
	return path, nil
}
func Request(ctx context.Context, home, method string, params app.Object) (any, error) {
	endpoint, e := Endpoint(home)
	if e != nil {
		return nil, e
	}
	dialer := net.Dialer{Timeout: 300 * time.Second}
	connection, e := dialer.DialContext(ctx, "unix", endpoint)
	if e != nil {
		return nil, e
	}
	defer connection.Close()
	stopCancel := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stopCancel()
	deadline := time.Now().Add(300 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	connection.SetDeadline(deadline)
	if e = json.NewEncoder(connection).Encode(app.Object{"method": method, "params": params}); e != nil {
		return nil, e
	}
	raw, e := readLine(connection)
	if e != nil {
		return nil, e
	}
	value, e := jsonvalue.Decode(raw)
	if e != nil {
		return nil, e
	}
	response, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Invalid device service response")
	}
	if response["ok"] != true {
		message, _ := response["error"].(string)
		if message == "" {
			message = "Device service failed"
		}
		return nil, errors.New(message)
	}
	return response["result"], nil
}
func readLine(reader io.Reader) ([]byte, error) {
	stream := bufio.NewReader(io.LimitReader(reader, MaxRequest+1))
	raw, e := stream.ReadBytes('\n')
	if len(raw) > MaxRequest || e != nil || len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return nil, fmt.Errorf("Request is too large or incomplete")
	}
	return raw, nil
}
func Serve(ctx context.Context, device *app.Device, interval, discoveryInterval time.Duration) error {
	return serve(ctx, device, interval, discoveryInterval, "")
}

// ServeRecovery starts a suspended daemon for explicit legacy rollback. It owns
// the socket and daemon lock, but waits for the installer to release its gate
// before invoking any application operation. The journal authenticates startup.
func ServeRecovery(ctx context.Context, device *app.Device, interval, discoveryInterval time.Duration, token string) error {
	journal, e := fsutil.ReadJSON(filepath.Join(device.Home, "install-transaction.json"))
	if e != nil {
		return e
	}
	if token == "" || journal["format"] != "rum-install-transaction/1" || journal["home"] != device.Home || journal["phase"] != "restored" || journal["native_restart_token"] != token {
		return fmt.Errorf("Invalid installation recovery service request")
	}
	return serve(ctx, device, interval, discoveryInterval, token)
}
func serve(ctx context.Context, device *app.Device, interval, discoveryInterval time.Duration, recoveryToken string) error {
	if interval < 5*time.Second || discoveryInterval < interval {
		return fmt.Errorf("Invalid synchronization intervals")
	}
	home := device.Home
	endpoint, e := Endpoint(home)
	if e != nil {
		return e
	}
	if recoveryToken == "" {
		lease, err := fsutil.Lease(home, false)
		if err != nil {
			return err
		}
		defer lease.Close()
	}
	lock, e := fsutil.Acquire(filepath.Join(home, "daemon.lock"), true, false)
	if e != nil {
		return e
	}
	defer lock.Close()
	// The daemon lock is held during both endpoint creation and cleanup.
	if e = os.Remove(endpoint); e != nil && !os.IsNotExist(e) {
		return e
	}
	listener, e := net.Listen("unix", endpoint)
	if e != nil {
		return e
	}
	defer listener.Close()
	if e = os.Chmod(endpoint, 0600); e != nil {
		return e
	}
	defer os.Remove(endpoint)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if e = fsutil.WriteJSON(filepath.Join(home, "service.json"), app.Object{"pid": os.Getpid(), "started_at": fsutil.Now(), "version": app.Version, "interval": int(interval.Seconds()), "discovery_interval": int(discoveryInterval.Seconds())}); e != nil {
		return e
	}
	defer os.Remove(filepath.Join(home, "service.json"))
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		if recoveryToken != "" {
			deadline := time.Now().Add(30 * time.Second)
			for {
				lease, err := fsutil.Lease(home, false)
				if err == nil {
					defer lease.Close()
					break
				}
				if time.Now().After(deadline) {
					cancel()
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(50 * time.Millisecond):
				}
			}
		}
		nextDiscovery := time.Time{}
		for ctx.Err() == nil {
			discover := !time.Now().Before(nextDiscovery)
			result, e := device.Call(ctx, "tick", app.Object{"discover": discover})
			if e != nil {
				result = app.Object{"error": e.Error(), "at": fsutil.Now()}
			}
			_ = fsutil.WriteJSON(filepath.Join(home, "last-cycle.json"), result)
			if discover {
				nextDiscovery = time.Now().Add(discoveryInterval)
			}
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	var mutex sync.Mutex
	connections := map[net.Conn]bool{}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		listener.Close()
		mutex.Lock()
		for connection := range connections {
			connection.Close()
		}
		mutex.Unlock()
	}()
	for {
		connection, e := listener.Accept()
		if e != nil {
			if ctx.Err() == nil {
				cancel()
				workers.Wait()
				<-stopped
				return e
			}
			break
		}
		mutex.Lock()
		if ctx.Err() != nil {
			mutex.Unlock()
			connection.Close()
			break
		}
		connections[connection] = true
		mutex.Unlock()
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer connection.Close()
			defer func() { mutex.Lock(); delete(connections, connection); mutex.Unlock() }()
			connection.SetReadDeadline(time.Now().Add(30 * time.Second))
			response, shutdown := handle(ctx, device, connection, recoveryToken)
			connection.SetWriteDeadline(time.Now().Add(30 * time.Second))
			_ = json.NewEncoder(connection).Encode(response)
			if shutdown {
				cancel()
			}
		}()
	}
	cancel()
	workers.Wait()
	<-stopped
	return nil
}
func handle(ctx context.Context, device *app.Device, reader io.Reader, recoveryToken string) (response app.Object, shutdown bool) {
	defer func() {
		if p := recover(); p != nil {
			response = app.Object{"ok": false, "error": fmt.Sprint(p)}
		}
	}()
	raw, e := readLine(reader)
	if e != nil {
		return app.Object{"ok": false, "error": e.Error()}, false
	}
	value, e := jsonvalue.Decode(raw)
	if e != nil {
		return app.Object{"ok": false, "error": e.Error()}, false
	}
	request, ok := value.(map[string]any)
	if !ok {
		return app.Object{"ok": false, "error": "Invalid request"}, false
	}
	method, ok := request["method"].(string)
	if !ok {
		return app.Object{"ok": false, "error": "Missing method"}, false
	}
	if method == "installation-ready" && recoveryToken != "" {
		return app.Object{"ok": true, "result": app.Object{"token": recoveryToken}}, false
	}
	if method == "shutdown" {
		return app.Object{"ok": true, "result": app.Object{"stopping": true}}, true
	}
	var params app.Object
	if request["params"] != nil {
		params, ok = request["params"].(map[string]any)
		if !ok {
			return app.Object{"ok": false, "error": "Invalid parameters"}, false
		}
	}
	result, e := device.Call(ctx, method, params)
	if e != nil {
		return app.Object{"ok": false, "error": e.Error()}, false
	}
	return app.Object{"ok": true, "result": result}, false
}
